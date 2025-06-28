package persistence

import (
	"time"
)

func createEvent(
	eventType PersistenceEventType,
	operation string,
	collectionName string,
	input any,
	output any,
	query any,
	err *string,
	issues []Issue,
	startTime time.Time,
) PersistenceEvent {
	var duration *int64
	if !startTime.IsZero() {
		d := time.Since(startTime).Milliseconds()
		duration = &d
	}

	collectionNamePtr := &collectionName

	return PersistenceEvent{
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
