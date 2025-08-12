package events

import (
	"time"

	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/persistence/utils"
	goevents "github.com/asaidimu/go-events"
)

// WithEventEmission is a higher-order function that wraps an operation
// with start, success, and failure events. It handles the timing of the operation
// and constructs the appropriate event for each stage.
func WithEventEmission(
	bus *goevents.TypedEventBus[base.PersistenceEvent],
	operationName string,
	collectionName string,
	startEventType base.PersistenceEventType,
	successEventType base.PersistenceEventType,
	failedEventType base.PersistenceEventType,
	input any,
	queryParam any,
	fn func() (any, error),
) (any, error) {
	startTime := time.Now()

	// Emit start event
	startEvent := utils.CreateEvent(
		startEventType,
		operationName,
		collectionName,
		input,
		nil, // No output yet
		queryParam,
		nil, // No error yet
		nil, // No issues yet
		startTime,
	)
	if bus != nil {
		bus.Emit(string(startEvent.Type), startEvent)
	}

	// Execute the operation
	result, err := fn()

	if err != nil {
		// Emit failure event
		errStr := err.Error()
		failEvent := utils.CreateEvent(
			failedEventType,
			operationName,
			collectionName,
			input,
			nil, // No output on failure
			queryParam,
			&errStr,
			nil, // Issues can be added here if available
			startTime,
		)
		if bus != nil {
			bus.Emit(string(failEvent.Type), failEvent)
		}
		return nil, err
	}

	// Emit success event
	successEvent := utils.CreateEvent(
		successEventType,
		operationName,
		collectionName,
		input,
		result,
		queryParam,
		nil, // No error on success
		nil, // No issues on success
		startTime,
	)
	if bus != nil {
		bus.Emit(string(successEvent.Type), successEvent)
	}

	return result, nil
}
