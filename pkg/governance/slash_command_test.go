package governance

import (
	"strings"
	"testing"

	"github.com/google/go-github/v76/github"
	"github.com/stretchr/testify/require"

)

func Test_handleComment(t *testing.T) {
	baseConfig := `
version: v1
exemptAssociations: [MEMBER]
slashCommands:
  issue:
    discuss:
      description: "Add needs discussion label"
      addLabels: [needs discussion]
      comment: "Discussion needed."
    duplicate:
      description: "Close as duplicate"
      requiresArg: true
      addLabels: [duplicate]
      close: true
      comment: "Duplicate of #{{.Arg}}."
    enterprise:
      description: "Close as planned for enterprise"
      close: true
      comment: "Planned for enterprise."
    maintainer:
      description: "Add maintainer only label"
      addLabels: [maintainer only]
      comment: "Maintainer only."
    research:
      description: "Add needs research label"
      addLabels: [needs research]
      comment: "Research needed."
    unblock:
      description: "Remove blocking labels"
      removeLabels: [kind/proposal, needs discussion, needs research]
      comment: "Unblocked."
  pullRequest:
    discuss:
      description: "Add needs discussion label; blocks merge"
      addLabels: [needs discussion]
      comment: "Discussion needed before merge."
    quality:
      description: "Close; quality standards"
      close: true
      comment: "Quality standards not met."
    unsolicited:
      description: "Close; no linked issue"
      close: true
      comment: "No linked issue."
`

	testCases := []struct {
		name        string
		commentBody string
		authorAssoc string
		isPR        bool
		assert      func(
			*testing.T,
			*fakeIssuesClientWithIssues,
			*fakePullRequestsClient,
		)
	}{
		{
			name:        "non-slash comment ignored",
			commentBody: "This is a regular comment.",
			authorAssoc: "MEMBER",
			assert: func(
				t *testing.T,
				ic *fakeIssuesClientWithIssues,
				_ *fakePullRequestsClient,
			) {
				require.Empty(t, ic.labelsAdded)
				require.Empty(t, ic.comments)
			},
		},
		{
			name:        "non-maintainer ignored",
			commentBody: "/discuss",
			authorAssoc: "NONE",
			assert: func(
				t *testing.T,
				ic *fakeIssuesClientWithIssues,
				_ *fakePullRequestsClient,
			) {
				require.Empty(t, ic.labelsAdded)
				require.Empty(t, ic.comments)
			},
		},
		{
			name:        "unknown command ignored",
			commentBody: "/nonexistent",
			authorAssoc: "MEMBER",
			assert: func(
				t *testing.T,
				ic *fakeIssuesClientWithIssues,
				_ *fakePullRequestsClient,
			) {
				require.Empty(t, ic.labelsAdded)
				require.Empty(t, ic.comments)
			},
		},
		{
			name:        "/discuss on issue adds label and comments",
			commentBody: "/discuss",
			authorAssoc: "MEMBER",
			assert: func(
				t *testing.T,
				ic *fakeIssuesClientWithIssues,
				_ *fakePullRequestsClient,
			) {
				require.Contains(t, ic.labelsAdded[42], "needs discussion")
				require.Len(t, ic.comments[42], 1)
				require.Contains(t, ic.comments[42][0], "Discussion needed.")
			},
		},
		{
			name:        "/duplicate on issue with arg",
			commentBody: "/duplicate #99",
			authorAssoc: "MEMBER",
			assert: func(
				t *testing.T,
				ic *fakeIssuesClientWithIssues,
				_ *fakePullRequestsClient,
			) {
				require.Contains(t, ic.labelsAdded[42], "duplicate")
				require.Len(t, ic.comments[42], 1)
				require.Contains(t, ic.comments[42][0], "#99")
				require.True(t, ic.closedIssues[42])
			},
		},
		{
			name:        "/duplicate without arg is ignored",
			commentBody: "/duplicate",
			authorAssoc: "MEMBER",
			assert: func(
				t *testing.T,
				ic *fakeIssuesClientWithIssues,
				_ *fakePullRequestsClient,
			) {
				require.Empty(t, ic.labelsAdded)
				require.Empty(t, ic.comments)
			},
		},
		{
			name:        "/enterprise on issue closes",
			commentBody: "/enterprise",
			authorAssoc: "MEMBER",
			assert: func(
				t *testing.T,
				ic *fakeIssuesClientWithIssues,
				_ *fakePullRequestsClient,
			) {
				require.Len(t, ic.comments[42], 1)
				require.Contains(t, ic.comments[42][0], "enterprise")
				require.True(t, ic.closedIssues[42])
			},
		},
		{
			name:        "/maintainer on issue adds label",
			commentBody: "/maintainer",
			authorAssoc: "MEMBER",
			assert: func(
				t *testing.T,
				ic *fakeIssuesClientWithIssues,
				_ *fakePullRequestsClient,
			) {
				require.Contains(t, ic.labelsAdded[42], "maintainer only")
				require.Len(t, ic.comments[42], 1)
				require.False(t, ic.closedIssues[42])
			},
		},
		{
			name:        "/research on issue adds label",
			commentBody: "/research",
			authorAssoc: "MEMBER",
			assert: func(
				t *testing.T,
				ic *fakeIssuesClientWithIssues,
				_ *fakePullRequestsClient,
			) {
				require.Contains(t, ic.labelsAdded[42], "needs research")
				require.Len(t, ic.comments[42], 1)
			},
		},
		{
			name:        "/unblock on issue removes labels",
			commentBody: "/unblock",
			authorAssoc: "MEMBER",
			assert: func(
				t *testing.T,
				ic *fakeIssuesClientWithIssues,
				_ *fakePullRequestsClient,
			) {
				require.Len(t, ic.comments[42], 1)
				require.Contains(t, ic.comments[42][0], "Unblocked.")
			},
		},
		{
			name:        "/discuss on PR uses PR command set",
			commentBody: "/discuss",
			authorAssoc: "MEMBER",
			isPR:        true,
			assert: func(
				t *testing.T,
				ic *fakeIssuesClientWithIssues,
				_ *fakePullRequestsClient,
			) {
				require.Contains(t, ic.labelsAdded[42], "needs discussion")
				require.Len(t, ic.comments[42], 1)
				require.Contains(t, ic.comments[42][0], "before merge")
			},
		},
		{
			name:        "/quality on PR closes via PullRequestsClient",
			commentBody: "/quality",
			authorAssoc: "MEMBER",
			isPR:        true,
			assert: func(
				t *testing.T,
				ic *fakeIssuesClientWithIssues,
				pc *fakePullRequestsClient,
			) {
				require.Len(t, ic.comments[42], 1)
				require.Contains(t, ic.comments[42][0], "Quality standards")
				require.True(t, pc.closedPRs[42])
			},
		},
		{
			name:        "/unsolicited on PR closes",
			commentBody: "/unsolicited",
			authorAssoc: "MEMBER",
			isPR:        true,
			assert: func(
				t *testing.T,
				ic *fakeIssuesClientWithIssues,
				pc *fakePullRequestsClient,
			) {
				require.Len(t, ic.comments[42], 1)
				require.True(t, pc.closedPRs[42])
			},
		},
		{
			name:        "PR-only command on issue is unknown",
			commentBody: "/quality",
			authorAssoc: "MEMBER",
			isPR:        false,
			assert: func(
				t *testing.T,
				ic *fakeIssuesClientWithIssues,
				_ *fakePullRequestsClient,
			) {
				require.Empty(t, ic.labelsAdded)
				require.Empty(t, ic.comments)
			},
		},
		{
			name:        "/help on issue generates dynamic help",
			commentBody: "/help",
			authorAssoc: "MEMBER",
			assert: func(
				t *testing.T,
				ic *fakeIssuesClientWithIssues,
				_ *fakePullRequestsClient,
			) {
				require.Len(t, ic.comments[42], 1)
				comment := ic.comments[42][0]
				require.Contains(t, comment, "Available Slash Commands")
				require.Contains(t, comment, "/discuss")
				require.Contains(t, comment, "/duplicate")
				require.Contains(t, comment, "/unblock")
				require.Contains(t, comment, "/help")
				// PR-only commands should NOT appear
				require.NotContains(t, comment, "/quality")
				require.NotContains(t, comment, "/unsolicited")
			},
		},
		{
			name:        "/help on PR generates PR-specific help",
			commentBody: "/help",
			authorAssoc: "MEMBER",
			isPR:        true,
			assert: func(
				t *testing.T,
				ic *fakeIssuesClientWithIssues,
				_ *fakePullRequestsClient,
			) {
				require.Len(t, ic.comments[42], 1)
				comment := ic.comments[42][0]
				require.Contains(t, comment, "Available Slash Commands")
				require.Contains(t, comment, "/quality")
				require.Contains(t, comment, "/unsolicited")
				// Issue-only commands should NOT appear
				require.NotContains(t, comment, "/maintainer")
			},
		},
		{
			name:        "command with extra text on same line",
			commentBody: "/duplicate #55 some extra explanation",
			authorAssoc: "MEMBER",
			assert: func(
				t *testing.T,
				ic *fakeIssuesClientWithIssues,
				_ *fakePullRequestsClient,
			) {
				require.Contains(t, ic.labelsAdded[42], "duplicate")
				require.Contains(t, ic.comments[42][0], "#55")
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			issuesClient := newFakeIssuesClientWithIssues()
			prsClient := newFakePullRequestsClient()
			h := &handler{
				clientFactory: &fakeClientFactoryFull{
					issuesClient: issuesClient,
					prsClient:    prsClient,
					reposClient: &fakeRepositoriesClient{
						configYAML: baseConfig,
					},
				},
			}

			issue := &github.Issue{Number: github.Ptr(42)}
			if testCase.isPR {
				issue.PullRequestLinks = &github.PullRequestLinks{
					URL: github.Ptr("https://api.github.com/repos/test/repo/pulls/42"),
				}
			}

			event := &github.IssueCommentEvent{
				Action: github.Ptr("created"),
				Issue:  issue,
				Comment: &github.IssueComment{
					Body:              github.Ptr(testCase.commentBody),
					AuthorAssociation: github.Ptr(testCase.authorAssoc),
				},
				Repo: &github.Repository{
					Name:  github.Ptr("kargo"),
					Owner: &github.User{Login: github.Ptr("akuity")},
				},
				Sender:       &github.User{Login: github.Ptr("maintainer")},
				Installation: &github.Installation{ID: github.Ptr(int64(1))},
			}
			h.handleComment(t.Context(), event)
			testCase.assert(t, issuesClient, prsClient)
		})
	}
}

func Test_buildHelpComment(t *testing.T) {
	commands := map[string]commandDef{
		"discuss": {
			Description: "Add needs discussion label",
		},
		"duplicate": {
			Description: "Close as duplicate",
			RequiresArg: true,
		},
		"unblock": {
			Description: "Remove blocking labels",
		},
	}
	result := buildHelpComment(commands)

	require.Contains(t, result, "Available Slash Commands")
	require.Contains(t, result, "| `/discuss` | Add needs discussion label |")
	require.Contains(t, result, "| `/duplicate #N` | Close as duplicate |")
	require.Contains(t, result, "| `/unblock` | Remove blocking labels |")
	require.Contains(t, result, "| `/help` | Show this list |")

	// Verify alphabetical order: discuss before duplicate before unblock
	discussIdx := strings.Index(result, "/discuss")
	duplicateIdx := strings.Index(result, "/duplicate")
	unblockIdx := strings.Index(result, "/unblock")
	helpIdx := strings.Index(result, "/help")
	require.Less(t, discussIdx, duplicateIdx)
	require.Less(t, duplicateIdx, unblockIdx)
	require.Less(t, unblockIdx, helpIdx)
}
