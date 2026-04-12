package governance

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_parseLinkedIssue(t *testing.T) {
	testCases := []struct {
		name     string
		body     string
		expected int
	}{
		{
			name:     "Closes #123",
			body:     "Some description.\n\nCloses #123",
			expected: 123,
		},
		{
			name:     "Fixes #456",
			body:     "Fixes #456",
			expected: 456,
		},
		{
			name:     "Resolves #789",
			body:     "Resolves #789\n\nMore text here.",
			expected: 789,
		},
		{
			name:     "close #1 (singular)",
			body:     "close #1",
			expected: 1,
		},
		{
			name:     "closed #2 (past tense)",
			body:     "closed #2",
			expected: 2,
		},
		{
			name:     "fix #3 (singular)",
			body:     "fix #3",
			expected: 3,
		},
		{
			name:     "fixed #4 (past tense)",
			body:     "fixed #4",
			expected: 4,
		},
		{
			name:     "resolve #5 (singular)",
			body:     "resolve #5",
			expected: 5,
		},
		{
			name:     "resolved #6 (past tense)",
			body:     "resolved #6",
			expected: 6,
		},
		{
			name:     "case insensitive",
			body:     "CLOSES #99",
			expected: 99,
		},
		{
			name:     "full URL",
			body:     "Closes https://github.com/akuity/kargo/issues/42",
			expected: 42,
		},
		{
			name:     "full URL with hash",
			body:     "Fixes https://github.com/akuity/kargo/issues/#42",
			expected: 42,
		},
		{
			name:     "first match wins",
			body:     "Closes #10\nAlso fixes #20",
			expected: 10,
		},
		{
			name:     "no match returns 0",
			body:     "This PR does some stuff.",
			expected: 0,
		},
		{
			name:     "empty body returns 0",
			body:     "",
			expected: 0,
		},
		{
			name:     "hash without keyword returns 0",
			body:     "Related to #123",
			expected: 0,
		},
		{
			name:     "keyword without number returns 0",
			body:     "Closes the loop",
			expected: 0,
		},
		{
			name:     "embedded in PR template",
			body:     "**Policy statement**\n\nCloses #55\n\n## Description\n\nSome work.",
			expected: 55,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			result := parseLinkedIssue(testCase.body)
			require.Equal(t, testCase.expected, result)
		})
	}
}
