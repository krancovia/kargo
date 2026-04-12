package governance

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/google/go-github/v76/github"

	"github.com/akuity/kargo/pkg/logging"
)

func (h *handler) handleComment(
	ctx context.Context,
	event *github.IssueCommentEvent,
) {
	logger := logging.LoggerFromContext(ctx)
	body := event.GetComment().GetBody()
	if !strings.HasPrefix(body, "/") {
		return
	}

	owner := event.GetRepo().GetOwner().GetLogin()
	repo := event.GetRepo().GetName()
	number := event.GetIssue().GetNumber()

	logger = logger.WithValues(
		"owner", owner,
		"repo", repo,
		"number", number,
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

	// Slash commands are maintainer-only.
	author := event.GetComment().GetAuthorAssociation()
	login := event.GetSender().GetLogin()
	if !isExempt(cfg, author, login) {
		logger.Debug("comment author is not a maintainer, ignoring")
		return
	}

	// Parse command and optional argument from the first line.
	firstLine := strings.SplitN(body, "\n", 2)[0]
	parts := strings.Fields(firstLine)
	cmdName := strings.TrimPrefix(parts[0], "/")
	var arg string
	if len(parts) > 1 {
		arg = strings.TrimPrefix(parts[1], "#")
	}

	logger = logger.WithValues("command", cmdName)
	ctx = logging.ContextWithLogger(ctx, logger)

	// Determine context: issue or PR.
	isPR := event.GetIssue().PullRequestLinks != nil
	var commands map[string]commandDef
	if isPR {
		commands = cfg.SlashCommands.PullRequest
	} else {
		commands = cfg.SlashCommands.Issue
	}

	issuesClient, err := h.clientFactory.NewIssuesClient(installationID)
	if err != nil {
		logger.Error(err, "error creating issues client")
		return
	}

	// /help is a built-in command that generates its response from the
	// command definitions.
	if cmdName == "help" {
		helpBody := buildHelpComment(commands)
		if _, _, err := issuesClient.CreateComment(
			ctx, owner, repo, number,
			&github.IssueComment{Body: github.Ptr(helpBody)},
		); err != nil {
			logger.Error(err, "error posting help comment")
		}
		return
	}

	cmd, ok := commands[cmdName]
	if !ok {
		logger.Debug("unknown slash command, ignoring")
		return
	}

	if cmd.RequiresArg && arg == "" {
		logger.Debug("slash command requires an argument, ignoring")
		return
	}

	var prsClient PullRequestsClient
	if cmd.Close && isPR {
		prsClient, err = h.clientFactory.NewPullRequestsClient(installationID)
		if err != nil {
			logger.Error(err, "error creating pull requests client")
			return
		}
	}

	templateData := map[string]string{
		"Arg":          arg,
		"RepoFullName": owner + "/" + repo,
	}

	logger.Info("executing slash command")
	executeAction(
		ctx, issuesClient, prsClient,
		owner, repo, number,
		cmd.action,
		templateData,
	)
}

func buildHelpComment(commands map[string]commandDef) string {
	var b strings.Builder
	b.WriteString("## Available Slash Commands\n\n")
	b.WriteString("| Command | Description |\n")
	b.WriteString("|---------|-------------|\n")

	names := make([]string, 0, len(commands))
	for name := range commands {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		cmd := commands[name]
		desc := cmd.Description
		if desc == "" {
			desc = "(no description)"
		}
		argHint := ""
		if cmd.RequiresArg {
			argHint = " #N"
		}
		fmt.Fprintf(&b, "| `/%s%s` | %s |\n", name, argHint, desc)
	}

	fmt.Fprintf(&b, "| `/help` | Show this list |\n")
	return b.String()
}
