package base

import (
	"github.com/asaidimu/go-anansi/v7/core/common"
	"github.com/asaidimu/go-anansi/v7/core/data"
)

type CreateResultSet []CreateResult

func (crs CreateResultSet) Documents() data.DocumentSet {
	docs := make(data.DocumentSet, len(crs))
	for i, res := range crs {
		docs[i] = res.Data
	}
	return docs
}

func (crs CreateResultSet) Issues() []common.Issue {
	var all []common.Issue
	for i, res := range crs {
		// Handle both validation issues AND hard persistence errors
		if len(res.Issues) > 0 || res.Error != nil {
			// Helper to create int pointer
			index := i

			docIssue := common.Issue{
				Code:  "DOCUMENT_CREATION_FAILED",
				Index: &index, // Clean integer, no string parsing needed
			}
			docIssue = docIssue.WithMessagef("Document at index %d could not be created", i)

			// Collect all problems for THIS document
			var causes common.Issues

			// Add hard errors (e.g., DB unique constraint violation)
			if res.Error != nil {
				causes = append(causes, common.Issue{
					Code:    "PERSISTENCE_ERROR",
					Message: res.Error.Error(),
					Index:   &index, // Same index as parent
				})
			}

			// Add validation issues (e.g., missing fields)
			for _, iss := range res.Issues {
				causes = append(causes, iss)
			}

			docIssue.Cause = &causes
			all = append(all, docIssue)
		}
	}
	return all
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
