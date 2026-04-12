package governance

import (
	"context"
	"testing"

	"github.com/google/go-github/v76/github"
	"github.com/stretchr/testify/require"

)

type fakePullRequestsClient struct {
	closedPRs map[int]bool
}

func newFakePullRequestsClient() *fakePullRequestsClient {
	return &fakePullRequestsClient{closedPRs: make(map[int]bool)}
}

func (f *fakePullRequestsClient) Edit(
	_ context.Context, _, _ string, number int, pr *github.PullRequest,
) (*github.PullRequest, *github.Response, error) {
	if pr.GetState() == "closed" {
		f.closedPRs[number] = true
	}
	return pr, nil, nil
}

// fakeIssuesClientWithIssues extends fakeIssuesClient to return
// specific issues by number.
type fakeIssuesClientWithIssues struct {
	fakeIssuesClient
	issues       map[int]*github.Issue
	comments     map[int][]string
	closedIssues map[int]bool
}

func newFakeIssuesClientWithIssues() *fakeIssuesClientWithIssues {
	return &fakeIssuesClientWithIssues{
		fakeIssuesClient: *newFakeIssuesClient(),
		issues:           make(map[int]*github.Issue),
		comments:         make(map[int][]string),
		closedIssues:     make(map[int]bool),
	}
}

func (f *fakeIssuesClientWithIssues) Get(
	_ context.Context, _, _ string, number int,
) (*github.Issue, *github.Response, error) {
	if issue, ok := f.issues[number]; ok {
		return issue, nil, nil
	}
	return &github.Issue{}, nil, nil
}

func (f *fakeIssuesClientWithIssues) CreateComment(
	_ context.Context, _, _ string, number int, comment *github.IssueComment,
) (*github.IssueComment, *github.Response, error) {
	f.comments[number] = append(f.comments[number], comment.GetBody())
	return comment, nil, nil
}

func (f *fakeIssuesClientWithIssues) Edit(
	_ context.Context, _, _ string, number int, req *github.IssueRequest,
) (*github.Issue, *github.Response, error) {
	if req.GetState() == "closed" {
		f.closedIssues[number] = true
	}
	return &github.Issue{}, nil, nil
}

type fakeClientFactoryFull struct {
	issuesClient *fakeIssuesClientWithIssues
	prsClient    *fakePullRequestsClient
	reposClient  *fakeRepositoriesClient
}

func (f *fakeClientFactoryFull) NewIssuesClient(int64) (IssuesClient, error) {
	return f.issuesClient, nil
}

func (f *fakeClientFactoryFull) NewPullRequestsClient(int64) (PullRequestsClient, error) {
	return f.prsClient, nil
}

func (f *fakeClientFactoryFull) NewRepositoriesClient(int64) (RepositoriesClient, error) {
	return f.reposClient, nil
}

func Test_handlePROpened(t *testing.T) {
	testCases := []struct {
		name       string
		configYAML string
		prBody     string
		prLabels   []*github.Label
		prAuthor   string
		senderLogin string
		issues     map[int]*github.Issue
		assert     func(
			*testing.T,
			*fakeIssuesClientWithIssues,
			*fakePullRequestsClient,
		)
	}{
		{
			name: "maintainer is exempt from policy check",
			configYAML: `
version: v1
exemptAssociations: [MEMBER]
prPolicy:
  blockingLabels: [kind/proposal]
  noLinkedIssue:
    close: true
    comment: "No linked issue."
`,
			prBody:   "No issue reference here.",
			prAuthor: "MEMBER",
			assert: func(
				t *testing.T,
				ic *fakeIssuesClientWithIssues,
				pc *fakePullRequestsClient,
			) {
				require.Empty(t, pc.closedPRs)
				require.Empty(t, ic.comments)
			},
		},
		{
			name: "bot is exempt from policy check",
			configYAML: `
version: v1
exemptActorSuffixes: ["[bot]"]
prPolicy:
  noLinkedIssue:
    close: true
    comment: "No linked issue."
`,
			prBody:      "No issue reference.",
			prAuthor:    "NONE",
			senderLogin: "dependabot[bot]",
			assert: func(
				t *testing.T,
				ic *fakeIssuesClientWithIssues,
				pc *fakePullRequestsClient,
			) {
				require.Empty(t, pc.closedPRs)
			},
		},
		{
			name: "no linked issue closes PR",
			configYAML: `
version: v1
prPolicy:
  noLinkedIssue:
    addLabels: [policy/no-linked-issue]
    close: true
    comment: "No linked issue found."
`,
			prBody: "This PR has no issue link.",
			assert: func(
				t *testing.T,
				ic *fakeIssuesClientWithIssues,
				_ *fakePullRequestsClient,
			) {
				require.Contains(t, ic.labelsAdded[1], "policy/no-linked-issue")
				require.Len(t, ic.comments[1], 1)
				require.Contains(t, ic.comments[1][0], "No linked issue found.")
				require.True(t, ic.closedIssues[1])
			},
		},
		{
			name: "linked issue with blocking label closes PR",
			configYAML: `
version: v1
prPolicy:
  blockingLabels: [kind/proposal, needs discussion]
  blockedIssue:
    addLabels: [policy/blocked-issue]
    close: true
    comment: "Issue #{{.IssueNumber}} blocked by {{.BlockingLabels}}"
`,
			prBody: "Closes #99",
			issues: map[int]*github.Issue{
				99: {Labels: []*github.Label{
					{Name: github.Ptr("kind/proposal")},
					{Name: github.Ptr("area/cli")},
				}},
			},
			assert: func(
				t *testing.T,
				ic *fakeIssuesClientWithIssues,
				_ *fakePullRequestsClient,
			) {
				require.Contains(t, ic.labelsAdded[1], "policy/blocked-issue")
				require.Len(t, ic.comments[1], 1)
				require.Contains(t, ic.comments[1][0], "#99")
				require.Contains(t, ic.comments[1][0], "`kind/proposal`")
				require.True(t, ic.closedIssues[1])
			},
		},
		{
			name: "linked issue without blocking labels passes",
			configYAML: `
version: v1
prPolicy:
  blockingLabels: [kind/proposal]
  noLinkedIssue:
    close: true
`,
			prBody: "Closes #50",
			issues: map[int]*github.Issue{
				50: {Labels: []*github.Label{
					{Name: github.Ptr("kind/bug")},
				}},
			},
			assert: func(
				t *testing.T,
				ic *fakeIssuesClientWithIssues,
				pc *fakePullRequestsClient,
			) {
				require.Empty(t, pc.closedPRs)
				require.Empty(t, ic.closedIssues)
			},
		},
		{
			name: "label inheritance copies matching prefixes",
			configYAML: `
version: v1
prPolicy:
  blockingLabels: []
labelInheritance:
  prefixes: [kind/, area/]
`,
			prBody: "Closes #50",
			issues: map[int]*github.Issue{
				50: {Labels: []*github.Label{
					{Name: github.Ptr("kind/bug")},
					{Name: github.Ptr("area/cli")},
					{Name: github.Ptr("priority/high")},
				}},
			},
			assert: func(
				t *testing.T,
				ic *fakeIssuesClientWithIssues,
				_ *fakePullRequestsClient,
			) {
				added := ic.labelsAdded[1]
				require.Contains(t, added, "kind/bug")
				require.Contains(t, added, "area/cli")
				require.NotContains(t, added, "priority/high")
			},
		},
		{
			name: "inherited labels satisfy governance",
			configYAML: `
version: v1
prPolicy:
  blockingLabels: []
labelInheritance:
  prefixes: [kind/]
labelGovernance:
  pullRequest:
  - prefix: kind
    values: [bug, enhancement]
`,
			prBody: "Closes #50",
			issues: map[int]*github.Issue{
				50: {Labels: []*github.Label{
					{Name: github.Ptr("kind/bug")},
				}},
			},
			assert: func(
				t *testing.T,
				ic *fakeIssuesClientWithIssues,
				_ *fakePullRequestsClient,
			) {
				added := ic.labelsAdded[1]
				require.Contains(t, added, "kind/bug")
				require.NotContains(t, added, "needs kind")
			},
		},
		{
			name: "governance flags missing labels after inheritance",
			configYAML: `
version: v1
prPolicy:
  blockingLabels: []
labelInheritance:
  prefixes: [kind/]
labelGovernance:
  pullRequest:
  - prefix: kind
    values: [bug]
  - prefix: priority
    values: [high, low]
`,
			prBody: "Closes #50",
			issues: map[int]*github.Issue{
				50: {Labels: []*github.Label{
					{Name: github.Ptr("kind/bug")},
				}},
			},
			assert: func(
				t *testing.T,
				ic *fakeIssuesClientWithIssues,
				_ *fakePullRequestsClient,
			) {
				added := ic.labelsAdded[1]
				require.Contains(t, added, "kind/bug")
				require.Contains(t, added, "needs priority")
				require.NotContains(t, added, "needs kind")
			},
		},
		{
			name: "policy close skips inheritance and governance",
			configYAML: `
version: v1
prPolicy:
  noLinkedIssue:
    close: true
labelInheritance:
  prefixes: [kind/]
labelGovernance:
  pullRequest:
  - prefix: kind
    values: [bug]
`,
			prBody: "No issue link.",
			assert: func(
				t *testing.T,
				ic *fakeIssuesClientWithIssues,
				_ *fakePullRequestsClient,
			) {
				require.True(t, ic.closedIssues[1])
				require.NotContains(t, ic.labelsAdded[1], "needs kind")
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			issuesClient := newFakeIssuesClientWithIssues()
			if testCase.issues != nil {
				issuesClient.issues = testCase.issues
			}
			prsClient := newFakePullRequestsClient()
			h := &handler{
				clientFactory: &fakeClientFactoryFull{
					issuesClient: issuesClient,
					prsClient:    prsClient,
					reposClient: &fakeRepositoriesClient{
						configYAML: testCase.configYAML,
					},
				},
			}
			authorAssoc := testCase.prAuthor
			if authorAssoc == "" {
				authorAssoc = "NONE"
			}
			senderLogin := testCase.senderLogin
			if senderLogin == "" {
				senderLogin = "some-user"
			}
			event := &github.PullRequestEvent{
				Action: github.Ptr("opened"),
				PullRequest: &github.PullRequest{
					Number:            github.Ptr(1),
					Body:              github.Ptr(testCase.prBody),
					Labels:            testCase.prLabels,
					AuthorAssociation: github.Ptr(authorAssoc),
				},
				Repo: &github.Repository{
					Name:  github.Ptr("kargo"),
					Owner: &github.User{Login: github.Ptr("akuity")},
				},
				Sender:       &github.User{Login: github.Ptr(senderLogin)},
				Installation: &github.Installation{ID: github.Ptr(int64(1))},
			}
			h.handlePROpened(t.Context(), event)
			testCase.assert(t, issuesClient, prsClient)
		})
	}
}

func Test_isExempt(t *testing.T) {
	cfg := &config{
		ExemptAssociations:  []string{"MEMBER", "OWNER"},
		ExemptActorSuffixes: []string{"[bot]"},
	}
	testCases := []struct {
		name        string
		association string
		login       string
		expected    bool
	}{
		{
			name:        "MEMBER is exempt",
			association: "MEMBER",
			login:       "kent",
			expected:    true,
		},
		{
			name:        "OWNER is exempt",
			association: "OWNER",
			login:       "kent",
			expected:    true,
		},
		{
			name:        "case insensitive association",
			association: "member",
			login:       "kent",
			expected:    true,
		},
		{
			name:        "bot suffix is exempt",
			association: "NONE",
			login:       "dependabot[bot]",
			expected:    true,
		},
		{
			name:        "regular user is not exempt",
			association: "NONE",
			login:       "random-user",
			expected:    false,
		},
		{
			name:        "CONTRIBUTOR is not exempt",
			association: "CONTRIBUTOR",
			login:       "regular",
			expected:    false,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			result := isExempt(cfg, testCase.association, testCase.login)
			require.Equal(t, testCase.expected, result)
		})
	}
}

func Test_renderTemplate(t *testing.T) {
	testCases := []struct {
		name     string
		tmpl     string
		data     map[string]string
		expected string
		hasError bool
	}{
		{
			name:     "no data passthrough",
			tmpl:     "Hello world",
			data:     nil,
			expected: "Hello world",
		},
		{
			name:     "template with variables",
			tmpl:     "Issue #{{.IssueNumber}} blocked by {{.BlockingLabels}}",
			data:     map[string]string{"IssueNumber": "42", "BlockingLabels": "`kind/proposal`"},
			expected: "Issue #42 blocked by `kind/proposal`",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			result, err := renderTemplate(testCase.tmpl, testCase.data)
			if testCase.hasError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, testCase.expected, result)
			}
		})
	}
}

func Test_formatBlockers(t *testing.T) {
	require.Equal(t, "`kind/proposal`", formatBlockers([]string{"kind/proposal"}))
	require.Equal(
		t,
		"`kind/proposal`, `needs discussion`",
		formatBlockers([]string{"kind/proposal", "needs discussion"}),
	)
}
