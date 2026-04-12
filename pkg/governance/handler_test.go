package governance

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-github/v76/github"
	"github.com/stretchr/testify/require"

)

const testWebhookSecret = "test-secret"

func signPayload(payload []byte) string {
	mac := hmac.New(sha256.New, []byte(testWebhookSecret))
	mac.Write(payload) // nolint: errcheck
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func newTestHandler() http.Handler {
	return NewHandler(
		[]byte(testWebhookSecret),
		&fakeClientFactory{
			issuesClient: newFakeIssuesClient(),
			reposClient: &fakeRepositoriesClient{
				configYAML: "version: v1",
			},
		},
	)
}

func Test_handler_ServeHTTP(t *testing.T) {
	testCases := []struct {
		name       string
		request    func() *http.Request
		assertResp func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name: "missing signature",
			request: func() *http.Request {
				return httptest.NewRequest(
					http.MethodPost, "/", strings.NewReader("{}"),
				)
			},
			assertResp: func(t *testing.T, rr *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusUnauthorized, rr.Code)
			},
		},
		{
			name: "invalid signature",
			request: func() *http.Request {
				req := httptest.NewRequest(
					http.MethodPost, "/", strings.NewReader("{}"),
				)
				req.Header.Set(github.SHA256SignatureHeader, "sha256=invalid")
				return req
			},
			assertResp: func(t *testing.T, rr *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusUnauthorized, rr.Code)
			},
		},
		{
			name: "ping event",
			request: func() *http.Request {
				body := []byte(`{"zen":"something"}`)
				req := httptest.NewRequest(
					http.MethodPost, "/", strings.NewReader(string(body)),
				)
				req.Header.Set(github.EventTypeHeader, "ping")
				req.Header.Set(github.SHA256SignatureHeader, signPayload(body))
				return req
			},
			assertResp: func(t *testing.T, rr *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusOK, rr.Code)
			},
		},
		{
			name: "pull_request opened event",
			request: func() *http.Request {
				event := github.PullRequestEvent{
					Action: github.Ptr("opened"),
					PullRequest: &github.PullRequest{
						Number:            github.Ptr(1),
						AuthorAssociation: github.Ptr("NONE"),
					},
					Repo: &github.Repository{
						Name:  github.Ptr("repo"),
						Owner: &github.User{Login: github.Ptr("test")},
					},
					Sender:       &github.User{Login: github.Ptr("someone")},
					Installation: &github.Installation{ID: github.Ptr(int64(1))},
				}
				body, _ := json.Marshal(event)
				req := httptest.NewRequest(
					http.MethodPost, "/", strings.NewReader(string(body)),
				)
				req.Header.Set(github.EventTypeHeader, "pull_request")
				req.Header.Set(github.SHA256SignatureHeader, signPayload(body))
				return req
			},
			assertResp: func(t *testing.T, rr *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusOK, rr.Code)
			},
		},
		{
			name: "issues opened event",
			request: func() *http.Request {
				event := github.IssuesEvent{
					Action: github.Ptr("opened"),
					Issue:  &github.Issue{Number: github.Ptr(1)},
					Repo: &github.Repository{
						Name:  github.Ptr("repo"),
						Owner: &github.User{Login: github.Ptr("test")},
					},
					Installation: &github.Installation{ID: github.Ptr(int64(1))},
				}
				body, _ := json.Marshal(event)
				req := httptest.NewRequest(
					http.MethodPost, "/", strings.NewReader(string(body)),
				)
				req.Header.Set(github.EventTypeHeader, "issues")
				req.Header.Set(github.SHA256SignatureHeader, signPayload(body))
				return req
			},
			assertResp: func(t *testing.T, rr *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusOK, rr.Code)
			},
		},
		{
			name: "issue_comment created event",
			request: func() *http.Request {
				event := github.IssueCommentEvent{
					Action:       github.Ptr("created"),
					Issue:        &github.Issue{Number: github.Ptr(1)},
					Comment:      &github.IssueComment{Body: github.Ptr("/help")},
					Repo:         &github.Repository{FullName: github.Ptr("test/repo")},
					Installation: &github.Installation{ID: github.Ptr(int64(1))},
				}
				body, _ := json.Marshal(event)
				req := httptest.NewRequest(
					http.MethodPost, "/", strings.NewReader(string(body)),
				)
				req.Header.Set(github.EventTypeHeader, "issue_comment")
				req.Header.Set(github.SHA256SignatureHeader, signPayload(body))
				return req
			},
			assertResp: func(t *testing.T, rr *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusOK, rr.Code)
			},
		},
		{
			name: "unhandled event type returns 200",
			request: func() *http.Request {
				body := []byte(`{"action":"completed"}`)
				req := httptest.NewRequest(
					http.MethodPost, "/", strings.NewReader(string(body)),
				)
				req.Header.Set(github.EventTypeHeader, "check_run")
				req.Header.Set(github.SHA256SignatureHeader, signPayload(body))
				return req
			},
			assertResp: func(t *testing.T, rr *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusOK, rr.Code)
			},
		},
		{
			name: "pull_request non-opened action ignored",
			request: func() *http.Request {
				event := github.PullRequestEvent{
					Action:      github.Ptr("closed"),
					PullRequest: &github.PullRequest{Number: github.Ptr(1)},
					Repo:        &github.Repository{FullName: github.Ptr("test/repo")},
				}
				body, _ := json.Marshal(event)
				req := httptest.NewRequest(
					http.MethodPost, "/", strings.NewReader(string(body)),
				)
				req.Header.Set(github.EventTypeHeader, "pull_request")
				req.Header.Set(github.SHA256SignatureHeader, signPayload(body))
				return req
			},
			assertResp: func(t *testing.T, rr *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusOK, rr.Code)
			},
		},
	}
	h := newTestHandler()
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, testCase.request())
			testCase.assertResp(t, rr)
		})
	}
}
