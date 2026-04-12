package governance

import (
	"context"
	"strings"

	"github.com/google/go-github/v76/github"

	"github.com/akuity/kargo/pkg/logging"
)

func (h *handler) handleIssueOpened(
	ctx context.Context,
	event *github.IssuesEvent,
) {
	logger := logging.LoggerFromContext(ctx)
	owner := event.GetRepo().GetOwner().GetLogin()
	repo := event.GetRepo().GetName()
	number := event.GetIssue().GetNumber()

	logger = logger.WithValues(
		"owner", owner,
		"repo", repo,
		"issue", number,
	)
	ctx = logging.ContextWithLogger(ctx, logger)

	installationID := event.GetInstallation().GetID()
	reposClient, err := h.clientFactory.NewRepositoriesClient(installationID)
	if err != nil {
		logger.Error(err, "error creating repositories client")
		return
	}

	cfg, err := h.loadConfig(ctx, reposClient, owner, repo)
	if err != nil {
		logger.Error(err, "error loading config")
		return
	}

	issuesClient, err := h.clientFactory.NewIssuesClient(installationID)
	if err != nil {
		logger.Error(err, "error creating issues client")
		return
	}

	existingLabels := make(map[string]bool)
	for _, l := range event.GetIssue().Labels {
		existingLabels[l.GetName()] = true
	}

	enforceRequiredLabels(
		ctx, issuesClient, owner, repo, number,
		existingLabels, cfg.LabelGovernance.Issue,
	)
}

func enforceRequiredLabels(
	ctx context.Context,
	issuesClient IssuesClient,
	owner, repo string,
	number int,
	existingLabels map[string]bool,
	groups []labelGroup,
) {
	logger := logging.LoggerFromContext(ctx)
	for _, group := range groups {
		if !needsLabel(group.Prefix, existingLabels) {
			continue
		}
		label := "needs " + group.Prefix
		logger.Info("adding missing label", "label", label)
		if _, _, err := issuesClient.AddLabelsToIssue(
			ctx, owner, repo, number, []string{label},
		); err != nil {
			logger.Error(err, "error adding label", "label", label)
		}
	}
}

// needsLabel returns true if no label with the given prefix is present
// in the existing labels.
func needsLabel(prefix string, existingLabels map[string]bool) bool {
	prefixSlash := prefix + "/"
	for label := range existingLabels {
		if strings.HasPrefix(label, prefixSlash) {
			return false
		}
	}
	return true
}
