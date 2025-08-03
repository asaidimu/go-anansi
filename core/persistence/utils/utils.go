// Package utils provides utility functions for the persistence layer.
package utils

import (
	"time"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
)

// createEvent is a helper function that constructs a PersistenceEvent. It populates
// the event with details about the operation, such as its type, the collection it
// belongs to, input and output data, and timing information. This function is used
// by the event-emitting wrappers to ensure that all events are created consistently.
func CreateEvent(
	eventType base.PersistenceEventType,
	operation string,
	collectionName string,
	input any,
	output any,
	query any,
	err *string,
	issues []common.Issue,
	startTime time.Time,
) base.PersistenceEvent {
	var duration *int64
	if !startTime.IsZero() {
		d := time.Since(startTime).Milliseconds()
		duration = &d
	}

	return base.PersistenceEvent{
		Type:       eventType,
		Timestamp:  time.Now().UnixMilli(),
		Operation:  operation,
		Collection: &collectionName,
		Input:      input,
		Output:     output,
		Error:      err,
		Issues:     issues,
		Query:      query,
		Duration:   duration,
	}
}

