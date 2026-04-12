package governance

import (
	"context"
	"fmt"

	"github.com/google/go-github/v76/github"
	"github.com/jferrl/go-githubauth"
	"golang.org/x/oauth2"
)

// IssuesClient is the subset of github.IssuesService methods needed by
// the governance bot. *github.IssuesService satisfies this interface.
type IssuesClient interface {
	Get(
		ctx context.Context,
		owner, repo string,
		number int,
	) (*github.Issue, *github.Response, error)
	Edit(
		ctx context.Context,
		owner, repo string,
		number int,
		issue *github.IssueRequest,
	) (*github.Issue, *github.Response, error)
	CreateComment(
		ctx context.Context,
		owner, repo string,
		number int,
		comment *github.IssueComment,
	) (*github.IssueComment, *github.Response, error)
	AddLabelsToIssue(
		ctx context.Context,
		owner, repo string,
		number int,
		labels []string,
	) ([]*github.Label, *github.Response, error)
	RemoveLabelForIssue(
		ctx context.Context,
		owner, repo string,
		number int,
		label string,
	) (*github.Response, error)
	AddAssignees(
		ctx context.Context,
		owner, repo string,
		number int,
		assignees []string,
	) (*github.Issue, *github.Response, error)
}

// PullRequestsClient is the subset of github.PullRequestsService
// methods needed by the governance bot.
// *github.PullRequestsService satisfies this interface.
type PullRequestsClient interface {
	Edit(
		ctx context.Context,
		owner, repo string,
		number int,
		pull *github.PullRequest,
	) (*github.PullRequest, *github.Response, error)
}

// RepositoriesClient is the subset of github.RepositoriesService
// methods needed by the governance bot.
// *github.RepositoriesService satisfies this interface.
type RepositoriesClient interface {
	GetContents(
		ctx context.Context,
		owner, repo, path string,
		opts *github.RepositoryContentGetOptions,
	) (
		*github.RepositoryContent,
		[]*github.RepositoryContent,
		*github.Response,
		error,
	)
}

// GitHubClientFactory creates authenticated GitHub service clients for
// a given installation.
type GitHubClientFactory interface {
	NewIssuesClient(installationID int64) (IssuesClient, error)
	NewPullRequestsClient(installationID int64) (PullRequestsClient, error)
	NewRepositoriesClient(installationID int64) (RepositoriesClient, error)
}

type githubClientFactory struct {
	appTokenSource oauth2.TokenSource
}

// NewGitHubClientFactory returns a GitHubClientFactory that
// authenticates as a GitHub App installation.
func NewGitHubClientFactory(
	clientID string,
	privateKey []byte,
) (GitHubClientFactory, error) {
	appTokenSource, err :=
		githubauth.NewApplicationTokenSource(clientID, privateKey)
	if err != nil {
		return nil, fmt.Errorf(
			"error creating application token source: %w", err,
		)
	}
	return &githubClientFactory{appTokenSource: appTokenSource}, nil
}

func (f *githubClientFactory) NewIssuesClient(
	installationID int64,
) (IssuesClient, error) {
	client, err := f.newClient(installationID)
	if err != nil {
		return nil, err
	}
	return client.Issues, nil
}

func (f *githubClientFactory) NewPullRequestsClient(
	installationID int64,
) (PullRequestsClient, error) {
	client, err := f.newClient(installationID)
	if err != nil {
		return nil, err
	}
	return client.PullRequests, nil
}

func (f *githubClientFactory) NewRepositoriesClient(
	installationID int64,
) (RepositoriesClient, error) {
	client, err := f.newClient(installationID)
	if err != nil {
		return nil, err
	}
	return client.Repositories, nil
}

func (f *githubClientFactory) newClient(
	installationID int64,
) (*github.Client, error) {
	installationTokenSource := githubauth.NewInstallationTokenSource(
		installationID,
		f.appTokenSource,
	)
	token, err := installationTokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf(
			"error getting installation access token: %w", err,
		)
	}
	return github.NewClient(nil).WithAuthToken(token.AccessToken), nil
}
