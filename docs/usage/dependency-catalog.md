# Dependency Catalog

## External Dependencies

### github.com/mattn/go-sqlite3
- **Purpose**: SQLite database driver
- **Installation**: `go get github.com/mattn/go-sqlite3`
- **Version Compatibility**: `>=1.14.22`

### go.uber.org/zap
- **Purpose**: Structured logging library
- **Installation**: `go get go.uber.org/zap`
- **Version Compatibility**: `>=1.27.0`

### github.com/asaidimu/go-events
- **Purpose**: Generic typed event bus for internal event emission
  - **Required Interfaces**:
    - `TypedEventBus[T]`: Provides methods for subscribing to and emitting typed events.
      - **Methods**:
        - `Subscribe`
          - **Signature**: `Subscribe(eventName string, callback events.EventCallback[T]) func()`
          - **Parameters**: eventName: string - The name of the event to subscribe to; callback: events.EventCallback[T] - The function to call when the event is emitted.
          - **Returns**: func() - A function that can be called to unsubscribe.
        - `Emit`
          - **Signature**: `Emit(eventName string, payload T)`
          - **Parameters**: eventName: string - The name of the event to emit; payload: T - The event payload.
          - **Returns**: void
- **Installation**: `go get github.com/asaidimu/go-events`
- **Version Compatibility**: `>=0.0.0`

### github.com/google/uuid
- **Purpose**: Library for generating UUIDs
- **Installation**: `go get github.com/google/uuid`
- **Version Compatibility**: `>=1.6.0`



---
*Generated using Gemini AI on 6/28/2025, 10:32:05 PM. Review and refine as needed.*