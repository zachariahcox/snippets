package jira

import (
	"fmt"
	"time"
)

// ReportConfig holds options for report generation.
type ReportConfig struct {
	Title           string
	UpdatedAfter    *time.Time
	NoCommentAfter  *time.Time
	OutputFile      string
	JSONOutput      bool
	CSVOutput       bool
	SlackOutput     bool
	URLOutput       bool
	SimpleOutput    bool
	SummaryOutput   bool
	JQLQuery        string
	IncludeChildren bool

	DueDateFieldName        string
	TrendingStatusFieldName string
	CustomFieldNameToID     map[string]string
}

func (c *ReportConfig) String() string {
	if c == nil {
		return "nil"
	}
	since := ""
	if c.UpdatedAfter != nil {
		since = c.UpdatedAfter.Format("2006-01-02")
	}
	noComment := ""
	if c.NoCommentAfter != nil {
		noComment = c.NoCommentAfter.Format("2006-01-02")
	}
	return fmt.Sprintf("title=%q jql=%q since=%q noCommentAfter=%q out=%q json=%t csv=%t slack=%t url=%t simple=%t summary=%t children=%t dueField=%q trendField=%q fieldIDs=%d",
		c.Title, c.JQLQuery, since, noComment, c.OutputFile,
		c.JSONOutput, c.CSVOutput, c.SlackOutput, c.URLOutput,
		c.SimpleOutput, c.SummaryOutput, c.IncludeChildren,
		c.DueDateFieldName, c.TrendingStatusFieldName, len(c.CustomFieldNameToID))
}

// IssueComment holds the latest comment metadata for an issue.
type IssueComment struct {
	Url     string `json:"url"`
	Created string `json:"created"`
}

// IssueData represents extracted issue data.
type IssueData struct {
	Key             string       `json:"key"`
	URL             string       `json:"url"`
	Summary         string       `json:"summary"`
	Status          string       `json:"status"`
	StatusEmoji     string       `json:"status_emoji"`
	Assignee        string       `json:"assignee"`
	Priority        string       `json:"priority"`
	Created         string       `json:"created"`
	Updated         string       `json:"updated"`
	Due             string       `json:"target_end"`
	Trending        string       `json:"trending"`
	TrendingEmoji   string       `json:"Emoji"` // cache compat: historical key name
	TrendingComment string       `json:"trending_comment"`
	Comment         IssueComment `json:"comment"`
	Type            string       `json:"type"`
	Children        []*IssueData `json:"children"`
}
