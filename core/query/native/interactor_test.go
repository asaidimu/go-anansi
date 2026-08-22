package native

import (
	"context"
	"runtime"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/document"
	"github.com/asaidimu/go-anansi/v8/core/query"
	"go.uber.org/zap"
)

// stubExecutor is a minimal QueryExecutor whose only interesting behaviour is
// handing back itself from BeginTransaction.
type stubExecutor struct{ began int }

func (s *stubExecutor) Query(ctx context.Context, q NativeQuery[any]) ([]*document.Document, int64, error) {
	return nil, 0, nil
}
func (s *stubExecutor) ExecuteQuery(ctx context.Context, q NativeQuery[any]) (*query.RawQueryResult, error) {
	return nil, nil
}
func (s *stubExecutor) Exec(ctx context.Context, q NativeQuery[any]) (int64, error) {
	return 0, nil
}
func (s *stubExecutor) QueryStream(ctx context.Context, q NativeQuery[any]) (<-chan map[string]any, <-chan error, error) {
	return nil, nil, nil
}
func (s *stubExecutor) BeginTransaction(ctx context.Context) (QueryExecutor[any], error) {
	s.began++
	return s, nil
}
func (s *stubExecutor) Commit(ctx context.Context) error   { return nil }
func (s *stubExecutor) Rollback(ctx context.Context) error { return nil }
func (s *stubExecutor) Close() error                       { return nil }

type stubFactory struct{}

func (f *stubFactory) Build(q *query.Query, stmtType StatementType, extra any) (Query[any], error) {
	var zero Query[any]
	return zero, nil
}
func (f *stubFactory) Capabilities() query.Capabilities { return query.Capabilities{} }

// TestStartTransactionNoWatcherForUncancelableContext guards against the
// goroutine leak where StartTransaction spawned a cancellation watcher even
// for contexts without a Done channel (e.g. context.Background()). Such a
// watcher parks forever on a nil-channel receive — one leaked goroutine per
// transaction on background/scheduler/boot write paths.
func TestStartTransactionNoWatcherForUncancelableContext(t *testing.T) {
	ix, err := NewNativeInteractor[any](&stubExecutor{}, &stubFactory{}, zap.NewNop())
	if err != nil {
		t.Fatalf("new interactor: %v", err)
	}

	runtime.GC()
	before := runtime.NumGoroutine()

	for i := 0; i < 25; i++ {
		tx, err := ix.StartTransaction(context.Background())
		if err != nil {
			t.Fatalf("start transaction %d: %v", i, err)
		}
		if err := tx.Commit(context.Background()); err != nil {
			t.Fatalf("commit %d: %v", i, err)
		}
	}

	// Watchers park on receive; give any (buggy) spawner a moment to pile up.
	runtime.GC()
	if after := runtime.NumGoroutine(); after > before+2 {
		t.Fatalf("goroutines before=%d after=%d: StartTransaction leaks watchers for uncancelable contexts", before, after)
	}
}

// TestStartTransactionWatcherRollsBackCancelableContext verifies the watcher
// still exists and does its job when the context CAN be canceled: canceling an
// open transaction's context must roll it back.
func TestStartTransactionWatcherRollsBackCancelableContext(t *testing.T) {
	exec := &stubExecutor{}
	ix, err := NewNativeInteractor[any](exec, &stubFactory{}, zap.NewNop())
	if err != nil {
		t.Fatalf("new interactor: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	tx, err := ix.StartTransaction(ctx)
	if err != nil {
		t.Fatalf("start transaction: %v", err)
	}
	txInteractor := tx.(*NativeInteractor[any])
	if txInteractor.done.Load() {
		t.Fatal("fresh transaction must not be marked done")
	}

	cancel()
	waitFor(t, func() bool { return txInteractor.done.Load() })
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if cond() {
			return
		}
		runtime.Gosched()
	}
	t.Fatal("condition not met")
}
