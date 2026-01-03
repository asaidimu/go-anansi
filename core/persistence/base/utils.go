package base

import (
	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/data"
)

// In package data
type CreateResultSet []CreateResult

func (crs CreateResultSet) Documents() data.DocumentSet {
    docs := make(data.DocumentSet, len(crs))
    for i, res := range crs {
        docs[i] = res.Data
    }
    return docs
}

// CreateIssue represents a validation failure tied to a specific document index.
type CreateIssue struct {
	Index  int            `json:"index"`  // The original position in the request array
	Issues []common.Issue `json:"issues"` // Why it failed validation
}

func (crs CreateResultSet) Issues() []CreateIssue {
	var list []CreateIssue
	for i, res := range crs {
		if len(res.Issues) > 0 {
			list = append(list, CreateIssue{
				Index:  i,
				Issues: res.Issues,
			})
		}
	}
	return list
}

// HasFailures returns true if any document failed creation.
func (crs CreateResultSet) HasFailures() bool {
	for _, res := range crs {
		if res.Error != nil || len(res.Issues) > 0 {
			return true
		}
	}
	return false
}

// Errors() for hard persistence/system failures (Status: FAILED_PERSISTENCE)
func (crs CreateResultSet) Errors() []map[string]any {
	var list []map[string]any
	for i, res := range crs {
		if res.Error != nil {
			list = append(list, map[string]any{
				"index": i,
				"error": res.Error,
			})
		}
	}
	return list
}
