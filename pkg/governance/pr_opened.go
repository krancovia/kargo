package governance

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"text/template"

	"github.com/google/go-github/v76/github"

	"github.com/akuity/kargo/pkg/logging"
)

func (h *handler) handlePROpened(
	ctx context.Context,
	event *github.PullRequestEvent,
) {
	logger := logging.LoggerFromContext(ctx)
	owner := event.GetRepo().GetOwner().GetLogin()
	repo := event.GetRepo().GetName()
	pr := event.GetPullRequest()
	number := pr.GetNumber()

	logger = logger.WithValues(
		"owner", owner,
		"repo", repo,
		"pr", number,
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

	// Auto-assign the PR to its author.
	login := event.GetSender().GetLogin()
	if _, _, err := issuesClient.AddAssignees(
		ctx, owner, repo, number, []string{login},
	); err != nil {
		logger.Error(err, "error assigning PR to author")
	}

	// Policy check: exempt maintainers and bots.
	author := pr.GetAuthorAssociation()
	if isExempt(cfg, author, login) {
		logger.Debug("author is exempt from policy check, skipping")
	} else {
		closed := h.checkPRPolicy(
			ctx, cfg, issuesClient, owner, repo, number, pr,
		)
		if closed {
			return
		}
	}

	// Label inheritance: copy labels from linked issue to PR.
	issueNumber := parseLinkedIssue(pr.GetBody())
	inheritedLabels := h.inheritLabels(
		ctx, cfg, issuesClient, owner, repo, number, issueNumber,
	)

	// Label governance: flag missing required labels, accounting for
	// both the PR's own labels and any we just inherited.
	existingLabels := make(map[string]bool)
	for _, l := range pr.Labels {
		existingLabels[l.GetName()] = true
	}
	for _, l := range inheritedLabels {
		existingLabels[l] = true
	}
	enforceRequiredLabels(
		ctx, issuesClient, owner, repo, number,
		existingLabels, cfg.LabelGovernance.PullRequest,
	)
}

// checkPRPolicy checks whether a PR has a linked issue and whether
// that issue has blocking labels. Returns true if the PR was closed.
func (h *handler) checkPRPolicy(
	ctx context.Context,
	cfg *config,
	issuesClient IssuesClient,
	owner, repo string,
	number int,
	pr *github.PullRequest,
) bool {
	logger := logging.LoggerFromContext(ctx)
	issueNumber := parseLinkedIssue(pr.GetBody())

	if issueNumber == 0 {
		logger.Info("PR has no linked issue, closing")
		executeAction(
			ctx, issuesClient, nil,
			owner, repo, number,
			cfg.PRPolicy.NoLinkedIssue, nil,
		)
		return true
	}

	logger = logger.WithValues("linkedIssue", issueNumber)
	ctx = logging.ContextWithLogger(ctx, logger)

	issue, _, err := issuesClient.Get(ctx, owner, repo, issueNumber)
	if err != nil {
		logger.Error(err, "error fetching linked issue")
		return false
	}

	issueLabels := make(map[string]bool)
	for _, l := range issue.Labels {
		issueLabels[l.GetName()] = true
	}

	var blockers []string
	for _, blocking := range cfg.PRPolicy.BlockingLabels {
		if issueLabels[blocking] {
			blockers = append(blockers, blocking)
		}
	}

	if len(blockers) > 0 {
		logger.Info("linked issue has blocking labels, closing PR",
			"blockers", blockers,
		)
		executeAction(
			ctx, issuesClient, nil,
			owner, repo, number,
			cfg.PRPolicy.BlockedIssue,
			map[string]string{
				"IssueNumber":    fmt.Sprintf("%d", issueNumber),
				"BlockingLabels": formatBlockers(blockers),
			},
		)
		return true
	}

	return false
}

// inheritLabels copies labels with configured prefixes from the linked
// issue to the PR. Returns the list of labels that were added.
func (h *handler) inheritLabels(
	ctx context.Context,
	cfg *config,
	issuesClient IssuesClient,
	owner, repo string,
	prNumber, issueNumber int,
) []string {
	if issueNumber == 0 || len(cfg.LabelInheritance.Prefixes) == 0 {
		return nil
	}

	logger := logging.LoggerFromContext(ctx)

	issue, _, err := issuesClient.Get(ctx, owner, repo, issueNumber)
	if err != nil {
		logger.Error(err, "error fetching linked issue for label inheritance")
		return nil
	}

	var toAdd []string
	for _, l := range issue.Labels {
		name := l.GetName()
		for _, prefix := range cfg.LabelInheritance.Prefixes {
			if strings.HasPrefix(name, prefix) {
				toAdd = append(toAdd, name)
				break
			}
		}
	}

	if len(toAdd) == 0 {
		return nil
	}

	logger.Info("inheriting labels from linked issue",
		"labels", toAdd,
		"linkedIssue", issueNumber,
	)
	if _, _, err := issuesClient.AddLabelsToIssue(
		ctx, owner, repo, prNumber, toAdd,
	); err != nil {
		logger.Error(err, "error adding inherited labels")
		return nil
	}
	return toAdd
}

// executeAction performs the actions defined in an action: add labels,
// remove labels, post a comment (with template rendering), and/or
// close the issue/PR.
func executeAction(
	ctx context.Context,
	issuesClient IssuesClient,
	prsClient PullRequestsClient,
	owner, repo string,
	number int,
	action action,
	templateData map[string]string,
) {
	logger := logging.LoggerFromContext(ctx)

	if len(action.AddLabels) > 0 {
		if _, _, err := issuesClient.AddLabelsToIssue(
			ctx, owner, repo, number, action.AddLabels,
		); err != nil {
			logger.Error(err, "error adding labels")
		}
	}

	for _, label := range action.RemoveLabels {
		if _, err := issuesClient.RemoveLabelForIssue(
			ctx, owner, repo, number, label,
		); err != nil {
			logger.Error(err, "error removing label", "label", label)
		}
	}

	if action.Comment != "" {
		body, err := renderTemplate(action.Comment, templateData)
		if err != nil {
			logger.Error(err, "error rendering comment template")
		} else {
			if _, _, err := issuesClient.CreateComment(
				ctx, owner, repo, number,
				&github.IssueComment{Body: github.Ptr(body)},
			); err != nil {
				logger.Error(err, "error posting comment")
			}
		}
	}

	if action.Close {
		if prsClient != nil {
			state := "closed"
			if _, _, err := prsClient.Edit(
				ctx, owner, repo, number,
				&github.PullRequest{State: &state},
			); err != nil {
				logger.Error(err, "error closing PR")
			}
		} else {
			stateReason := "not_planned"
			state := "closed"
			if _, _, err := issuesClient.Edit(
				ctx, owner, repo, number,
				&github.IssueRequest{
					State:       &state,
					StateReason: &stateReason,
				},
			); err != nil {
				logger.Error(err, "error closing issue")
			}
		}
	}
}

func renderTemplate(
	tmpl string,
	data map[string]string,
) (string, error) {
	if data == nil {
		return tmpl, nil
	}
	t, err := template.New("").Parse(tmpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err = t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func formatBlockers(blockers []string) string {
	formatted := make([]string, len(blockers))
	for i, b := range blockers {
		formatted[i] = "`" + b + "`"
	}
	return strings.Join(formatted, ", ")
}

func isExempt(cfg *config, authorAssociation, login string) bool {
	for _, exempt := range cfg.ExemptAssociations {
		if strings.EqualFold(authorAssociation, exempt) {
			return true
		}
	}
	for _, suffix := range cfg.ExemptActorSuffixes {
		if strings.HasSuffix(login, suffix) {
			return true
		}
	}
	return false
}
