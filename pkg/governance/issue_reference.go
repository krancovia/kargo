package governance

import (
	"regexp"
	"strconv"
)

// issueRefPattern matches GitHub closing keyword syntax:
//   - Closes #123
//   - Fixes #123
//   - Resolves #123
//   - Close #123, Fix #123, Resolve #123
//   - Closed #123, Fixed #123, Resolved #123
//   - Full URL variants: Closes https://github.com/owner/repo/issues/123
var issueRefPattern = regexp.MustCompile(
	`(?i)(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?)\s+` +
		`(?:https://github\.com/[^/]+/[^/]+/issues/)?#?(\d+)`,
)

// parseLinkedIssue extracts the first linked issue number from a PR
// body. Returns 0 if no linked issue is found.
func parseLinkedIssue(body string) int {
	match := issueRefPattern.FindStringSubmatch(body)
	if len(match) < 2 {
		return 0
	}
	n, err := strconv.Atoi(match[1])
	if err != nil {
		return 0
	}
	return n
}
