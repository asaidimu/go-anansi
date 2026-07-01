// Package orchestrator provides a facade over multiple base.Persistence
// backends, routing every collection-scoped operation to the backend
// explicitly registered for that collection. It exists for deployments that
// split data across more than one physical store — for example, a SQLite
// file tuned for high-volume access/audit logs, and a separate SQLite file
// for application state — while presenting a single, uniform
// base.Persistence surface to the rest of the application.
//
// ROUTING MODEL
//
// Routing is explicit and static, by design. There is no name-based
// convention (e.g. an "audit_" prefix) and no implicit fallback backend,
// because either of those would make it possible for a collection to end up
// in the wrong physical store without anyone deciding that on purpose — and
// for audit/forensic data in particular, "which database is this actually
// in" should always be an answer you can point to, not an inference.
//
// Usage is a two-step registration:
//
//	o := orchestrator.New(logger)
//	_ = o.RegisterBackend("app", appPersistence)
//	_ = o.RegisterBackend("logs", logsPersistence)
//	_ = o.RouteCollection("orders", "app")
//	_ = o.RouteCollection("access_log", "logs")
//
// Collections must be routed *before* they are created through the
// orchestrator: CreateCollection / CreateCollections require an existing
// route and will return an error telling the caller to call RouteCollection
// first, rather than guessing a default backend.
//
// LIMITATIONS — READ BEFORE USE
//
//   - Transact is NOT supported across backends, and never will be. SQLite
//     (and most embedded engines) cannot provide a single atomic commit
//     across two independent database files/connections without a real
//     two-phase-commit protocol, which this package does not implement.
//     Calling Transact on an Orchestrator always returns
//     ErrCrossBackendTransaction. A caller that needs a real transaction
//     must fetch the specific backend via Backend(label) or BackendFor(name)
//     and call Transact directly on that backend, accepting that the
//     transaction is scoped to that single backend's collections only.
//
//   - Raw queries (Query) are only supported when every collection
//     referenced in the RawQuery's Collections map resolves to the same
//     backend. A raw SQL template and its placeholders are compiled by one
//     backend's query factory; there is no meaningful way to run a single
//     raw query template across two separate database connections. Mixed-
//     backend raw queries return ErrCrossBackendRawQuery.
//
//   - Joins across collections that live in different backends are NOT
//     resolved by this package. Collection(ctx, name) returns the
//     underlying backend's collection unmodified; if a query.Query passed to
//     that collection's Read contains a join targeting a collection routed
//     to a different backend, the underlying backend will fail trying to
//     resolve it, because it has no knowledge of the other database.
//     Cross-backend joins are a separate, higher-level concern (an
//     in-memory join executor sitting in front of Read) that this package
//     intentionally does not implement.
//
//   - Subscribe/Unsubscribe/Subscriptions only track subscriptions made
//     through this Orchestrator. A subscription registered directly against
//     an underlying backend (bypassing the facade) will not appear in
//     Subscriptions() and cannot be removed via Unsubscribe.
package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/asaidimu/go-anansi/v7/core/persistence/base"
	"github.com/asaidimu/go-anansi/v7/core/query"
	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
	"go.uber.org/zap"
)

// Compile-time assertion that Orchestrator satisfies base.Persistence.
var _ base.Persistence = (*Orchestrator)(nil)

// ---------------------------------------------------------------------
//  Sentinel errors
// ---------------------------------------------------------------------

var (
	// ErrNoRouteRegistered is returned when an operation references a
	// collection name that has no backend registered for it via
	// RouteCollection.
	ErrNoRouteRegistered = errors.New("orchestrator: no backend registered for collection")

	// ErrCrossBackendTransaction is always returned by Transact. See the
	// package documentation for why cross-backend transactions are not
	// supported.
	ErrCrossBackendTransaction = errors.New("orchestrator: cross-backend transactions are not supported; obtain a specific backend via Backend(label) or BackendFor(collection) and transact on it directly")

	// ErrCrossBackendRawQuery is returned by Query when the supplied
	// RawQuery references collections that resolve to more than one
	// backend.
	ErrCrossBackendRawQuery = errors.New("orchestrator: raw query references collections across more than one backend")

	// ErrBackendNotFound is returned by Backend and RouteCollection when no
	// backend has been registered under the given label.
	ErrBackendNotFound = errors.New("orchestrator: no backend registered under that label")

	// ErrAlreadyClosed is returned by any operation attempted after Close
	// has been called on the Orchestrator.
	ErrAlreadyClosed = errors.New("orchestrator: orchestrator instance is closed")
)

// ---------------------------------------------------------------------
//  Future (backend-independent Async support)
// ---------------------------------------------------------------------

// future is a minimal base.Future implementation backed by a channel. It is
// intentionally independent of any specific backend: base.BasePersistence.Async
// is not collection-scoped, so there is no backend to delegate to — the
// orchestrator just provides the goroutine/Future plumbing itself.
type future struct {
	done   chan struct{}
	result any
	err    error
}

func newFuture(ctx context.Context, f func(ctx context.Context) (any, error)) *future {
	fut := &future{done: make(chan struct{})}
	go func() {
		defer close(fut.done)
		defer func() {
			if r := recover(); r != nil {
				fut.err = fmt.Errorf("orchestrator: async function panicked: %v", r)
			}
		}()
		fut.result, fut.err = f(ctx)
	}()
	return fut
}

// Await blocks until the underlying function has completed and returns its
// result and error. Await may be called more than once; subsequent calls
// return the same result.
func (f *future) Await() (any, error) {
	<-f.done
	return f.result, f.err
}

// ---------------------------------------------------------------------
//  Backend registry
// ---------------------------------------------------------------------

// backendEntry pairs a registered backend with the human-readable label it
// was registered under. Using a pointer as the map value for routes lets us
// compare backend identity by pointer equality (e.g. when grouping schemas
// by backend in CreateCollections) without relying on base.Persistence being
// comparable.
type backendEntry struct {
	label       string
	persistence base.Persistence
}

// facadeSubscription tracks a single subscription registered through the
// Orchestrator: the original options the caller subscribed with, and the
// per-backend registrations that resulted from fanning that subscription out
// to every backend.
type facadeSubscription struct {
	id            string
	options       base.SubscriptionOptions
	registrations []subscriptionRegistration
}

type subscriptionRegistration struct {
	entry        *backendEntry
	backendSubID string
}

// Orchestrator is a base.Persistence facade that routes every collection-
// scoped operation to whichever backend was explicitly registered for that
// collection name via RouteCollection. It implements base.Persistence in
// full, but Transact and cross-backend Query are intentionally unsupported —
// see the package documentation for why, and for the recommended
// alternative.
type Orchestrator struct {
	mu sync.RWMutex

	// routes maps a collection name to the backend responsible for it.
	routes map[string]*backendEntry

	// backendsByLabel allows callers (and RouteCollection) to resolve a
	// human-readable label to its backend.
	backendsByLabel map[string]*backendEntry

	// allBackends is the deduplicated, insertion-ordered set of every
	// backend that has been registered. Used for fan-out operations
	// (ListCollections, Metadata, Subscribe, Close, ...).
	allBackends []*backendEntry

	// subscriptions tracks every subscription registered through this
	// Orchestrator, keyed by the facade-level subscription ID returned from
	// Subscribe.
	subscriptions map[string]*facadeSubscription
	nextSubID     atomic.Int64

	logger *zap.Logger
	closed bool
}

// New creates an empty Orchestrator. At least one backend must be attached
// via RegisterBackend, and at least one collection routed via
// RouteCollection, before any collection-scoped operation will succeed.
func New(logger *zap.Logger) *Orchestrator {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Orchestrator{
		routes:          make(map[string]*backendEntry),
		backendsByLabel: make(map[string]*backendEntry),
		subscriptions:   make(map[string]*facadeSubscription),
		logger:          logger,
	}
}

// RegisterBackend attaches a backend under a human-readable label (e.g.
// "app", "logs"), without routing any collections to it yet. The label is
// used by Backend, BackendFor, RouteCollection, and in error messages.
//
// RegisterBackend is idempotent when called twice with the same label and
// the same backend instance. Registering a different instance under a label
// that is already in use returns an error rather than silently replacing it,
// since silently re-pointing a label could route future collections to the
// wrong physical store without anyone noticing.
func (o *Orchestrator) RegisterBackend(label string, p base.Persistence) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.closed {
		return ErrAlreadyClosed
	}
	if label == "" {
		return fmt.Errorf("orchestrator: backend label must not be empty")
	}
	if p == nil {
		return fmt.Errorf("orchestrator: backend for label %q must not be nil", label)
	}

	if existing, ok := o.backendsByLabel[label]; ok {
		if existing.persistence != p {
			return fmt.Errorf("orchestrator: label %q is already registered to a different backend", label)
		}
		return nil
	}

	entry := &backendEntry{label: label, persistence: p}
	o.backendsByLabel[label] = entry
	o.allBackends = append(o.allBackends, entry)
	return nil
}

// RouteCollection declares that the named collection is owned by the backend
// registered under the given label. RegisterBackend must have been called
// for that label first.
//
// Calling RouteCollection again for a collection name that is already routed
// re-points it at the (possibly different) backend under the new label. This
// is allowed so routing tables can be rebuilt during startup, but doing it
// while the collection is in active use will make in-flight callers see an
// inconsistent picture of which backend owns the collection — it is a
// startup-time configuration operation, not a runtime one.
func (o *Orchestrator) RouteCollection(name string, label string) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.closed {
		return ErrAlreadyClosed
	}
	if name == "" {
		return fmt.Errorf("orchestrator: collection name must not be empty")
	}

	entry, ok := o.backendsByLabel[label]
	if !ok {
		return fmt.Errorf("%w: %q", ErrBackendNotFound, label)
	}

	o.routes[name] = entry
	return nil
}

// Backend returns the backend registered under the given label. This is the
// supported way to run a real, single-backend Transact: fetch the backend
// you need and call Transact on it directly, rather than through the
// Orchestrator.
func (o *Orchestrator) Backend(label string) (base.Persistence, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if o.closed {
		return nil, ErrAlreadyClosed
	}
	entry, ok := o.backendsByLabel[label]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrBackendNotFound, label)
	}
	return entry.persistence, nil
}

// BackendFor returns the backend that owns the given collection name,
// resolved via the routing table. Like Backend, this exists so a caller that
// needs a real ACID transaction can fetch the right backend and call
// Transact on it directly.
func (o *Orchestrator) BackendFor(collectionName string) (base.Persistence, error) {
	entry, err := o.resolve(collectionName)
	if err != nil {
		return nil, err
	}
	return entry.persistence, nil
}

// Labels returns the labels of every backend currently registered, in
// registration order.
func (o *Orchestrator) Labels() []string {
	o.mu.RLock()
	defer o.mu.RUnlock()

	labels := make([]string, len(o.allBackends))
	for i, entry := range o.allBackends {
		labels[i] = entry.label
	}
	return labels
}

// Routes returns a snapshot of the current collection-name-to-backend-label
// routing table, primarily for debugging and operational introspection.
func (o *Orchestrator) Routes() map[string]string {
	o.mu.RLock()
	defer o.mu.RUnlock()

	routes := make(map[string]string, len(o.routes))
	for name, entry := range o.routes {
		routes[name] = entry.label
	}
	return routes
}

// resolve looks up the backend responsible for a collection name. It is the
// single choke point every collection-scoped method goes through, so the
// "closed" and "not registered" checks only need to live in one place.
func (o *Orchestrator) resolve(name string) (*backendEntry, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if o.closed {
		return nil, ErrAlreadyClosed
	}
	entry, ok := o.routes[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrNoRouteRegistered, name)
	}
	return entry, nil
}

// snapshotBackends returns a copy of the current backend list, safe to range
// over without holding the lock for the duration of fan-out calls into
// backends (which may themselves take time and must not be made while
// holding o.mu, to avoid blocking unrelated RouteCollection/RegisterBackend
// calls for the duration of a slow fan-out).
func (o *Orchestrator) snapshotBackends() ([]*backendEntry, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if o.closed {
		return nil, ErrAlreadyClosed
	}
	backends := make([]*backendEntry, len(o.allBackends))
	copy(backends, o.allBackends)
	return backends, nil
}

// ---------------------------------------------------------------------
//  base.BasePersistence
// ---------------------------------------------------------------------

// Collection returns a handle to the named collection from whichever backend
// it is routed to.
func (o *Orchestrator) Collection(ctx context.Context, name string) (base.Collection, error) {
	entry, err := o.resolve(name)
	if err != nil {
		return nil, err
	}
	return entry.persistence.Collection(ctx, name)
}

// ListCollections returns the names of every collection known to every
// registered backend, deduplicated and sorted for deterministic output.
// Under normal operation there should be no duplicates, since a collection
// is only ever routed to one backend — but a backend could in principle
// contain collections nobody has routed through this Orchestrator yet (e.g.
// created directly against the backend), so we still de-duplicate
// defensively rather than assume the routing table is exhaustive.
func (o *Orchestrator) ListCollections(ctx context.Context) ([]string, error) {
	backends, err := o.snapshotBackends()
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	var names []string

	for _, entry := range backends {
		backendNames, err := entry.persistence.ListCollections(ctx)
		if err != nil {
			return nil, fmt.Errorf("orchestrator: listing collections on backend %q: %w", entry.label, err)
		}
		for _, n := range backendNames {
			if _, dup := seen[n]; dup {
				continue
			}
			seen[n] = struct{}{}
			names = append(names, n)
		}
	}

	sort.Strings(names)
	return names, nil
}

// Delete removes the named collection from its backend, and — on success —
// removes it from the routing table as well, so a subsequently re-created
// collection of the same name must be explicitly re-routed rather than
// silently inheriting a stale route.
func (o *Orchestrator) Delete(ctx context.Context, name string) (bool, error) {
	entry, err := o.resolve(name)
	if err != nil {
		return false, err
	}

	deleted, err := entry.persistence.Delete(ctx, name)
	if err != nil {
		return deleted, err
	}

	if deleted {
		o.mu.Lock()
		delete(o.routes, name)
		o.mu.Unlock()
	}

	return deleted, nil
}

// Schema retrieves a schema definition from the backend that owns the named
// collection.
func (o *Orchestrator) Schema(ctx context.Context, name string, version ...string) (*definition.Schema, error) {
	entry, err := o.resolve(name)
	if err != nil {
		return nil, err
	}
	return entry.persistence.Schema(ctx, name, version...)
}

// Metadata fans out to every registered backend and merges the results: counts
// and storage usage are summed, and the schema/collection/subscription lists
// are concatenated. ConnectionStatus surfaces the least-healthy status seen
// across any backend, and ConnectionError concatenates every backend's error
// (prefixed with its label) rather than discarding all but one, since a
// caller debugging a multi-backend deployment needs to know which backend is
// unhealthy, not just that one of them is.
func (o *Orchestrator) Metadata(ctx context.Context, filter *base.MetadataFilter) (base.Metadata, error) {
	backends, err := o.snapshotBackends()
	if err != nil {
		return base.Metadata{}, err
	}

	var merged base.Metadata
	var totalCollections int64
	var totalStorage int64
	storageKnown := false

	for _, entry := range backends {
		meta, err := entry.persistence.Metadata(ctx, filter)
		if err != nil {
			return base.Metadata{}, fmt.Errorf("orchestrator: fetching metadata from backend %q: %w", entry.label, err)
		}

		if meta.CollectionCount != nil {
			totalCollections += *meta.CollectionCount
		}
		if meta.StorageUsageBytes != nil {
			totalStorage += *meta.StorageUsageBytes
			storageKnown = true
		}

		merged.Schemas = append(merged.Schemas, meta.Schemas...)
		merged.Collections = append(merged.Collections, meta.Collections...)
		merged.Subscriptions = append(merged.Subscriptions, meta.Subscriptions...)

		if meta.ConnectionStatus != nil && *meta.ConnectionStatus != "healthy" {
			status := fmt.Sprintf("%s (%s)", *meta.ConnectionStatus, entry.label)
			merged.ConnectionStatus = &status
		}

		if meta.ConnectionError != nil {
			labeled := fmt.Sprintf("[%s] %s", entry.label, *meta.ConnectionError)
			if merged.ConnectionError == nil {
				merged.ConnectionError = &labeled
			} else {
				joined := *merged.ConnectionError + "; " + labeled
				merged.ConnectionError = &joined
			}
		}
	}

	merged.CollectionCount = &totalCollections
	if storageKnown {
		merged.StorageUsageBytes = &totalStorage
	}
	if merged.ConnectionStatus == nil {
		healthy := "healthy"
		merged.ConnectionStatus = &healthy
	}

	return merged, nil
}

// Async spawns the given function in a goroutine and returns a Future for
// it. This is not collection-scoped (per the base.BasePersistence contract)
// so there is no backend to delegate to — the Orchestrator provides this
// directly rather than arbitrarily picking one backend's implementation.
func (o *Orchestrator) Async(ctx context.Context, f func(ctx context.Context) (any, error)) base.Future {
	return newFuture(ctx, f)
}

// Query executes a raw, templated query. Every collection referenced in
// rawQuery.Collections must resolve to the same backend — a raw SQL template
// is compiled and executed by a single backend's query factory, so there is
// no way to honor a template that spans two separate database connections.
// If rawQuery.Collections is empty, the Orchestrator has no way to determine
// which backend should run the query and returns an error rather than
// guessing.
func (o *Orchestrator) Query(ctx context.Context, rawQuery *query.RawQuery) (*query.RawQueryResult, error) {
	if rawQuery == nil {
		return nil, fmt.Errorf("orchestrator: raw query must not be nil")
	}
	if len(rawQuery.Collections) == 0 {
		return nil, fmt.Errorf("orchestrator: raw query must specify at least one entry in Collections so the orchestrator can determine which backend to run it against")
	}

	var resolved *backendEntry
	var resolvedPlaceholder string

	for placeholder, target := range rawQuery.Collections {
		entry, err := o.resolve(target.Collection)
		if err != nil {
			return nil, fmt.Errorf("orchestrator: resolving raw query placeholder %q: %w", placeholder, err)
		}
		if resolved == nil {
			resolved = entry
			resolvedPlaceholder = placeholder
			continue
		}
		if resolved != entry {
			return nil, fmt.Errorf("%w: placeholder %q resolves to backend %q but placeholder %q resolves to backend %q",
				ErrCrossBackendRawQuery, placeholder, entry.label, resolvedPlaceholder, resolved.label)
		}
	}

	return resolved.persistence.Query(ctx, rawQuery)
}

// ---------------------------------------------------------------------
//  base.Persistence (extends BasePersistence)
// ---------------------------------------------------------------------

// CreateCollection creates a new collection on the backend that was
// previously registered for sc.Name via RouteCollection. If no route exists
// yet, this returns an error instructing the caller to register one first —
// the Orchestrator never guesses a default backend for an unrouted
// collection, since that would defeat the point of explicit routing.
func (o *Orchestrator) CreateCollection(ctx context.Context, sc *definition.Schema) (base.Collection, error) {
	if sc == nil {
		return nil, fmt.Errorf("orchestrator: schema must not be nil")
	}

	entry, err := o.resolve(sc.Name)
	if err != nil {
		return nil, fmt.Errorf("%w; call RouteCollection(%q, <backend label>) before creating it", err, sc.Name)
	}
	return entry.persistence.CreateCollection(ctx, sc)
}

// CreateCollections creates multiple collections, grouping them by their
// registered backend and issuing one CreateCollections call per backend.
//
// This is NOT atomic across backends: if creation succeeds against one
// backend and then fails against another, the collections already created
// remain created, and there is no rollback. Every schema's route is
// validated up front, before any backend is touched, so a missing route
// fails the whole call before any collection is created anywhere — but once
// backend calls start, partial failure across backends is possible and must
// be handled by the caller (e.g. by checking which collections now exist via
// HasCollection and deciding how to proceed).
func (o *Orchestrator) CreateCollections(ctx context.Context, schemas []*definition.Schema) error {
	if len(schemas) == 0 {
		return nil
	}

	grouped := make(map[*backendEntry][]*definition.Schema)
	var order []*backendEntry

	for _, sc := range schemas {
		if sc == nil {
			return fmt.Errorf("orchestrator: schema list contains a nil schema")
		}
		entry, err := o.resolve(sc.Name)
		if err != nil {
			return fmt.Errorf("%w; call RouteCollection(%q, <backend label>) before creating it", err, sc.Name)
		}
		if _, ok := grouped[entry]; !ok {
			order = append(order, entry)
		}
		grouped[entry] = append(grouped[entry], sc)
	}

	for _, entry := range order {
		if err := entry.persistence.CreateCollections(ctx, grouped[entry]); err != nil {
			return fmt.Errorf("orchestrator: creating collections on backend %q: %w", entry.label, err)
		}
	}

	return nil
}

// HasCollection reports whether the named collection is both routed and
// present on its backend. A collection name with no route registered is
// treated as simply not existing (false, nil) rather than as an error,
// matching the intuitive meaning of "has collection" — callers checking
// existence shouldn't have to distinguish "doesn't exist" from "exists but
// nobody told me where to look for it".
func (o *Orchestrator) HasCollection(ctx context.Context, name string) (bool, error) {
	entry, err := o.resolve(name)
	if err != nil {
		if errors.Is(err, ErrNoRouteRegistered) {
			return false, nil
		}
		return false, err
	}
	return entry.persistence.HasCollection(ctx, name)
}

// Transact always returns ErrCrossBackendTransaction. The Orchestrator
// cannot know, ahead of calling the callback, which backend(s) it will touch
// — and even if it could, SQLite cannot commit atomically across two
// independent database files without a two-phase-commit protocol this
// package does not implement. Use Backend(label) or BackendFor(collection)
// to get a specific backend and call Transact on it directly; that
// transaction will be scoped to that backend's collections only.
func (o *Orchestrator) Transact(ctx context.Context, callback func(ctx context.Context, p base.BasePersistence) (any, error)) (any, error) {
	return nil, ErrCrossBackendTransaction
}

// Subscribe registers the given subscription against every currently
// registered backend, since a persistence-level event (e.g. a document
// create/update) could originate from any of them. It returns a single
// facade-level subscription ID that Unsubscribe and Subscriptions
// understand; this ID does not correspond to any single backend's internal
// subscription ID.
//
// Backends registered *after* a call to Subscribe will not receive that
// subscription. Register all backends before subscribing if you need
// guaranteed coverage.
func (o *Orchestrator) Subscribe(ctx context.Context, options base.SubscriptionOptions) string {
	backends, err := o.snapshotBackends()
	if err != nil {
		// Orchestrator is closed; base.Persistence.Subscribe has no error
		// return, so we signal failure the only way the interface allows:
		// an empty ID that will never match a real subscription.
		o.logger.Warn("subscribe called on closed orchestrator")
		return ""
	}

	facadeID := fmt.Sprintf("orch-sub-%d", o.nextSubID.Add(1))
	sub := &facadeSubscription{id: facadeID, options: options}

	for _, entry := range backends {
		backendID := entry.persistence.Subscribe(ctx, options)
		sub.registrations = append(sub.registrations, subscriptionRegistration{
			entry:        entry,
			backendSubID: backendID,
		})
	}

	o.mu.Lock()
	o.subscriptions[facadeID] = sub
	o.mu.Unlock()

	return facadeID
}

// Unsubscribe removes a facade-level subscription previously returned by
// Subscribe, unsubscribing it from every backend it was registered against.
// Unsubscribing an unknown or already-removed ID is a no-op.
func (o *Orchestrator) Unsubscribe(ctx context.Context, id string) {
	o.mu.Lock()
	sub, ok := o.subscriptions[id]
	if ok {
		delete(o.subscriptions, id)
	}
	o.mu.Unlock()

	if !ok {
		return
	}

	for _, reg := range sub.registrations {
		reg.entry.persistence.Unsubscribe(ctx, reg.backendSubID)
	}
}

// Subscriptions returns every subscription currently registered through this
// Orchestrator. Each entry's Id is the facade-level ID returned by Subscribe
// (not any backend's internal ID), and its Unsubscribe closure removes the
// subscription from every backend it was fanned out to.
//
// Subscriptions made directly against an underlying backend, bypassing this
// Orchestrator, are not visible here.
func (o *Orchestrator) Subscriptions(ctx context.Context) ([]base.SubscriptionInfo, error) {
	o.mu.RLock()
	if o.closed {
		o.mu.RUnlock()
		return nil, ErrAlreadyClosed
	}
	subs := make([]*facadeSubscription, 0, len(o.subscriptions))
	for _, sub := range o.subscriptions {
		subs = append(subs, sub)
	}
	o.mu.RUnlock()

	infos := make([]base.SubscriptionInfo, 0, len(subs))
	for _, sub := range subs {
		id := sub.id
		infos = append(infos, base.SubscriptionInfo{
			Id:          &id,
			Event:       sub.options.Event,
			Label:       sub.options.Label,
			Description: sub.options.Description,
			Unsubscribe: func() { o.Unsubscribe(context.Background(), id) },
		})
	}

	sort.Slice(infos, func(i, j int) bool {
		return *infos[i].Id < *infos[j].Id
	})

	return infos, nil
}

// Rollback reverts a schema migration on the backend that owns the named
// collection.
func (o *Orchestrator) Rollback(ctx context.Context, name string, version *string, dryRun *bool) (base.Collection, error) {
	entry, err := o.resolve(name)
	if err != nil {
		return nil, err
	}
	return entry.persistence.Rollback(ctx, name, version, dryRun)
}

// Migrate applies a schema migration on the backend that owns the named
// collection.
func (o *Orchestrator) Migrate(ctx context.Context, name string, migration any, dryRun *bool) (base.Collection, error) {
	entry, err := o.resolve(name)
	if err != nil {
		return nil, err
	}
	return entry.persistence.Migrate(ctx, name, migration, dryRun)
}

// Close terminates every registered backend and marks the Orchestrator
// closed; subsequent calls to any method return ErrAlreadyClosed (or, for
// Subscribe, an empty string, since that method has no error return). Close
// is idempotent — calling it more than once is a no-op after the first call.
func (o *Orchestrator) Close(ctx context.Context) {
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		return
	}
	o.closed = true
	backends := make([]*backendEntry, len(o.allBackends))
	copy(backends, o.allBackends)
	o.mu.Unlock()

	for _, entry := range backends {
		entry.persistence.Close(ctx)
	}
}
