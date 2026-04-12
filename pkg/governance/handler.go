package governance

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/google/go-github/v76/github"

	"github.com/akuity/kargo/pkg/logging"
)

const (
	maxBodySize = 1 << 20 // 1 MB
	configPath  = ".github/governance.yaml"
)

type handler struct {
	webhookSecret []byte
	clientFactory GitHubClientFactory
}

// NewHandler returns an http.Handler that processes GitHub webhooks for
// governance policies. The webhookSecret is used to validate webhook
// signatures. The clientFactory is used to create authenticated GitHub
// clients per installation.
func NewHandler(
	webhookSecret []byte,
	clientFactory GitHubClientFactory,
) http.Handler {
	return &handler{
		webhookSecret: webhookSecret,
		clientFactory: clientFactory,
	}
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	logger := logging.LoggerFromContext(r.Context())

	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodySize))
	if err != nil {
		logger.Error(err, "error reading request body")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	sig := r.Header.Get(github.SHA256SignatureHeader)
	if sig == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if err = github.ValidateSignature(sig, body, h.webhookSecret); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	eventType := r.Header.Get(github.EventTypeHeader)
	logger = logger.WithValues("eventType", eventType)

	event, err := github.ParseWebHook(eventType, body)
	if err != nil {
		logger.Error(err, "error parsing webhook payload")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	ctx := logging.ContextWithLogger(r.Context(), logger)

	switch e := event.(type) {
	case *github.PingEvent:
		logger.Debug("received ping event")
	case *github.PullRequestEvent:
		if e.GetAction() == "opened" {
			h.handlePROpened(ctx, e)
		}
	case *github.IssuesEvent:
		if e.GetAction() == "opened" {
			h.handleIssueOpened(ctx, e)
		}
	case *github.IssueCommentEvent:
		if e.GetAction() == "created" {
			h.handleComment(ctx, e)
		}
	default:
		logger.Debug("ignoring unhandled event type")
	}

	w.WriteHeader(http.StatusOK)
}

func (h *handler) loadConfig(
	ctx context.Context,
	reposClient RepositoriesClient,
	owner, repo string,
) (*config, error) {
	content, _, _, err := reposClient.GetContents(
		ctx, owner, repo, configPath,
		&github.RepositoryContentGetOptions{Ref: "HEAD"},
	)
	if err != nil {
		return nil, fmt.Errorf("error fetching governance config: %w", err)
	}
	if content == nil {
		return nil, fmt.Errorf("governance config not found at %s", configPath)
	}
	raw, err := content.GetContent()
	if err != nil {
		return nil, fmt.Errorf("error decoding governance config: %w", err)
	}
	return parseConfig([]byte(raw))
}

// Stubs for handlers to be implemented in subsequent steps.
