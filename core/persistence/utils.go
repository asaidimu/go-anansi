package persistence

import (
	"time"

	"github.com/asaidimu/go-anansi/core"
)

func createEvent(
	eventType core.PersistenceEventType,
	operation string,
	collectionName string,
	input any,
	output any,
	query any,
	err *string,
	issues []core.Issue,
	startTime time.Time,
) core.PersistenceEvent {
	var duration *int64
	if !startTime.IsZero() {
		d := time.Since(startTime).Milliseconds()
		duration = &d
	}

	collectionNamePtr := &collectionName

	return core.PersistenceEvent{
		Type:       eventType,
		Timestamp:  time.Now().UnixMilli(),
		Operation:  operation,
		Collection: collectionNamePtr,
		Input:      input,
		Output:     output,
		Error:      err,
		Issues:     issues,
		Query:      query,
		Duration:   duration,
	}
}
