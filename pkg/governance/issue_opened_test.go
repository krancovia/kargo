package governance

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/google/go-github/v76/github"
	"github.com/stretchr/testify/require"

)

type fakeIssuesClient struct {
	labelsAdded map[int][]string
}

func newFakeIssuesClient() *fakeIssuesClient {
	return &fakeIssuesClient{labelsAdded: make(map[int][]string)}
}

func (f *fakeIssuesClient) Get(
	context.Context, string, string, int,
) (*github.Issue, *github.Response, error) {
	return nil, nil, nil
}

func (f *fakeIssuesClient) Edit(
	context.Context, string, string, int, *github.IssueRequest,
) (*github.Issue, *github.Response, error) {
	return nil, nil, nil
}

func (f *fakeIssuesClient) CreateComment(
	context.Context, string, string, int, *github.IssueComment,
) (*github.IssueComment, *github.Response, error) {
	return nil, nil, nil
}

func (f *fakeIssuesClient) AddLabelsToIssue(
	_ context.Context, _, _ string, number int, labels []string,
) ([]*github.Label, *github.Response, error) {
	f.labelsAdded[number] = append(f.labelsAdded[number], labels...)
	return nil, nil, nil
}

func (f *fakeIssuesClient) RemoveLabelForIssue(
	context.Context, string, string, int, string,
) (*github.Response, error) {
	return nil, nil
}

func (f *fakeIssuesClient) AddAssignees(
	context.Context, string, string, int, []string,
) (*github.Issue, *github.Response, error) {
	return nil, nil, nil
}

type fakeRepositoriesClient struct {
	configYAML string
}

func (f *fakeRepositoriesClient) GetContents(
	_ context.Context, _, _, _ string, _ *github.RepositoryContentGetOptions,
) (*github.RepositoryContent, []*github.RepositoryContent, *github.Response, error) {
	encoded := base64.StdEncoding.EncodeToString([]byte(f.configYAML))
	return &github.RepositoryContent{
		Content:  github.Ptr(encoded),
		Encoding: github.Ptr("base64"),
	}, nil, nil, nil
}

type fakeClientFactory struct {
	issuesClient *fakeIssuesClient
	reposClient  *fakeRepositoriesClient
}

func (f *fakeClientFactory) NewIssuesClient(int64) (IssuesClient, error) {
	return f.issuesClient, nil
}

func (f *fakeClientFactory) NewPullRequestsClient(int64) (PullRequestsClient, error) {
	return nil, nil
}

func (f *fakeClientFactory) NewRepositoriesClient(int64) (RepositoriesClient, error) {
	return f.reposClient, nil
}

func Test_handleIssueOpened(t *testing.T) {
	testCases := []struct {
		name         string
		configYAML   string
		issueLabels  []*github.Label
		assert       func(*testing.T, *fakeIssuesClient)
	}{
		{
			name: "all required labels present",
			configYAML: `
version: v1
labelGovernance:
  issue:
  - prefix: kind
    values: [bug, enhancement]
  - prefix: priority
    values: [high, low]
`,
			issueLabels: []*github.Label{
				{Name: github.Ptr("kind/bug")},
				{Name: github.Ptr("priority/high")},
			},
			assert: func(t *testing.T, fc *fakeIssuesClient) {
				require.Empty(t, fc.labelsAdded)
			},
		},
		{
			name: "missing kind label",
			configYAML: `
version: v1
labelGovernance:
  issue:
  - prefix: kind
    values: [bug, enhancement]
  - prefix: priority
    values: [high, low]
`,
			issueLabels: []*github.Label{
				{Name: github.Ptr("priority/high")},
			},
			assert: func(t *testing.T, fc *fakeIssuesClient) {
				require.Contains(t, fc.labelsAdded[42], "needs kind")
				require.Len(t, fc.labelsAdded[42], 1)
			},
		},
		{
			name: "missing all required labels",
			configYAML: `
version: v1
labelGovernance:
  issue:
  - prefix: kind
    values: [bug, enhancement]
  - prefix: priority
    values: [high, low]
  - prefix: area
    values: [docs, cli]
`,
			issueLabels: []*github.Label{},
			assert: func(t *testing.T, fc *fakeIssuesClient) {
				added := fc.labelsAdded[42]
				require.Len(t, added, 3)
				require.Contains(t, added, "needs kind")
				require.Contains(t, added, "needs priority")
				require.Contains(t, added, "needs area")
			},
		},
		{
			name: "no label governance configured",
			configYAML: `
version: v1
`,
			issueLabels: []*github.Label{},
			assert: func(t *testing.T, fc *fakeIssuesClient) {
				require.Empty(t, fc.labelsAdded)
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			issuesClient := newFakeIssuesClient()
			h := &handler{
				clientFactory: &fakeClientFactory{
					issuesClient: issuesClient,
					reposClient:  &fakeRepositoriesClient{configYAML: testCase.configYAML},
				},
			}
			event := &github.IssuesEvent{
				Action: github.Ptr("opened"),
				Issue: &github.Issue{
					Number: github.Ptr(42),
					Labels: testCase.issueLabels,
				},
				Repo: &github.Repository{
					Name: github.Ptr("kargo"),
					Owner: &github.User{Login: github.Ptr("akuity")},
				},
				Installation: &github.Installation{ID: github.Ptr(int64(1))},
			}
			h.handleIssueOpened(t.Context(), event)
			testCase.assert(t, issuesClient)
		})
	}
}

func Test_needsLabel(t *testing.T) {
	testCases := []struct {
		name           string
		prefix         string
		existingLabels map[string]bool
		expected       bool
	}{
		{
			name:           "label present",
			prefix:         "kind",
			existingLabels: map[string]bool{"kind/bug": true},
			expected:       false,
		},
		{
			name:           "label missing",
			prefix:         "kind",
			existingLabels: map[string]bool{"priority/high": true},
			expected:       true,
		},
		{
			name:           "no labels at all",
			prefix:         "kind",
			existingLabels: map[string]bool{},
			expected:       true,
		},
		{
			name:           "prefix without slash does not match",
			prefix:         "kind",
			existingLabels: map[string]bool{"kinder": true},
			expected:       true,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			result := needsLabel(testCase.prefix, testCase.existingLabels)
			require.Equal(t, testCase.expected, result)
		})
	}
}
