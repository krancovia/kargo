package governance

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_parseConfig(t *testing.T) {
	testCases := []struct {
		name   string
		data   []byte
		assert func(*testing.T, *config, error)
	}{
		{
			name: "valid full config",
			data: []byte(`
version: v1
exemptAssociations:
- MEMBER
- OWNER
exemptActorSuffixes:
- "[bot]"
prPolicy:
  blockingLabels:
  - kind/proposal
  - needs discussion
  noLinkedIssue:
    addLabels:
    - policy/no-linked-issue
    comment: "No linked issue found."
    close: true
  blockedIssue:
    addLabels:
    - policy/blocked-issue
    comment: "Issue #{{.IssueNumber}} is blocked by: {{.BlockingLabels}}"
    close: true
labelGovernance:
  issue:
  - prefix: kind
    multiple: true
    values: [bug, enhancement]
  - prefix: priority
    multiple: false
    values: [high, low]
  pullRequest:
  - prefix: kind
    multiple: true
    values: [bug, enhancement]
labelInheritance:
  prefixes:
  - kind/
  - area/
slashCommands:
  issue:
    discuss:
      addLabels: [needs discussion]
      comment: "Discussion needed."
    duplicate:
      requiresArg: true
      addLabels: [duplicate]
      close: true
      comment: "Duplicate of #{{.Arg}}."
  pullRequest:
    quality:
      close: true
      comment: "Does not meet quality standards."
    unblock:
      removeLabels: [needs discussion, needs research]
      comment: "Unblocked for merge."
`),
			assert: func(t *testing.T, cfg *config, err error) {
				require.NoError(t, err)
				require.Equal(t, "v1", cfg.Version)

				// Exempt associations
				require.Equal(
					t,
					[]string{"MEMBER", "OWNER"},
					cfg.ExemptAssociations,
				)

				// Exempt actor suffixes
				require.Equal(
					t,
					[]string{"[bot]"},
					cfg.ExemptActorSuffixes,
				)

				// PR policy
				require.Equal(
					t,
					[]string{"kind/proposal", "needs discussion"},
					cfg.PRPolicy.BlockingLabels,
				)
				require.Equal(
					t,
					[]string{"policy/no-linked-issue"},
					cfg.PRPolicy.NoLinkedIssue.AddLabels,
				)
				require.True(t, cfg.PRPolicy.NoLinkedIssue.Close)
				require.Equal(
					t,
					[]string{"policy/blocked-issue"},
					cfg.PRPolicy.BlockedIssue.AddLabels,
				)
				require.True(t, cfg.PRPolicy.BlockedIssue.Close)

				// Label governance
				require.Len(t, cfg.LabelGovernance.Issue, 2)
				require.Equal(t, "kind", cfg.LabelGovernance.Issue[0].Prefix)
				require.True(t, cfg.LabelGovernance.Issue[0].Multiple)
				require.Equal(
					t,
					[]string{"bug", "enhancement"},
					cfg.LabelGovernance.Issue[0].Values,
				)
				require.Equal(t, "priority", cfg.LabelGovernance.Issue[1].Prefix)
				require.False(t, cfg.LabelGovernance.Issue[1].Multiple)
				require.Len(t, cfg.LabelGovernance.PullRequest, 1)

				// Label inheritance
				require.Equal(
					t,
					[]string{"kind/", "area/"},
					cfg.LabelInheritance.Prefixes,
				)

				// Slash commands - issue
				require.Len(t, cfg.SlashCommands.Issue, 2)
				discuss := cfg.SlashCommands.Issue["discuss"]
				require.Equal(
					t,
					[]string{"needs discussion"},
					discuss.AddLabels,
				)
				require.False(t, discuss.Close)
				duplicate := cfg.SlashCommands.Issue["duplicate"]
				require.True(t, duplicate.RequiresArg)
				require.True(t, duplicate.Close)

				// Slash commands - pull request
				require.Len(t, cfg.SlashCommands.PullRequest, 2)
				quality := cfg.SlashCommands.PullRequest["quality"]
				require.True(t, quality.Close)
				unblock := cfg.SlashCommands.PullRequest["unblock"]
				require.Equal(
					t,
					[]string{"needs discussion", "needs research"},
					unblock.RemoveLabels,
				)
				require.False(t, unblock.Close)
			},
		},
		{
			name: "minimal config",
			data: []byte(`version: v1`),
			assert: func(t *testing.T, cfg *config, err error) {
				require.NoError(t, err)
				require.Equal(t, "v1", cfg.Version)
				require.Empty(t, cfg.ExemptAssociations)
				require.Empty(t, cfg.PRPolicy.BlockingLabels)
				require.Nil(t, cfg.SlashCommands.Issue)
				require.Nil(t, cfg.SlashCommands.PullRequest)
			},
		},
		{
			name: "invalid YAML",
			data: []byte(`{{{`),
			assert: func(t *testing.T, _ *config, err error) {
				require.Error(t, err)
				require.Contains(t, err.Error(), "error parsing governance config")
			},
		},
		{
			name: "invalid template in PR policy",
			data: []byte(`
version: v1
prPolicy:
  noLinkedIssue:
    comment: "{{.Unclosed"
`),
			assert: func(t *testing.T, _ *config, err error) {
				require.Error(t, err)
				require.Contains(
					t, err.Error(), "error validating governance config",
				)
				require.Contains(
					t, err.Error(), "prPolicy.noLinkedIssue.comment",
				)
			},
		},
		{
			name: "invalid template in slash command",
			data: []byte(`
version: v1
slashCommands:
  issue:
    broken:
      comment: "{{range}}"
`),
			assert: func(t *testing.T, _ *config, err error) {
				require.Error(t, err)
				require.Contains(
					t, err.Error(), "slashCommands.issue.broken.comment",
				)
			},
		},
		{
			name: "valid templates with variables",
			data: []byte(`
version: v1
prPolicy:
  blockedIssue:
    comment: "Issue #{{.IssueNumber}} blocked by {{.BlockingLabels}}"
slashCommands:
  issue:
    duplicate:
      requiresArg: true
      comment: "Duplicate of #{{.Arg}}."
  pullRequest:
    premature:
      comment: "Repo: {{.RepoFullName}}"
`),
			assert: func(t *testing.T, cfg *config, err error) {
				require.NoError(t, err)
				require.Contains(
					t,
					cfg.PRPolicy.BlockedIssue.Comment,
					"{{.IssueNumber}}",
				)
				require.Contains(
					t,
					cfg.SlashCommands.Issue["duplicate"].Comment,
					"{{.Arg}}",
				)
			},
		},
		{
			name: "empty comment templates are valid",
			data: []byte(`
version: v1
slashCommands:
  issue:
    unblock:
      removeLabels: [kind/proposal]
`),
			assert: func(t *testing.T, cfg *config, err error) {
				require.NoError(t, err)
				require.Empty(
					t, cfg.SlashCommands.Issue["unblock"].Comment,
				)
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			cfg, err := parseConfig(testCase.data)
			testCase.assert(t, cfg, err)
		})
	}
}
