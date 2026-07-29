package collection_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/asaidimu/go-anansi/v8/core/cache"
	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/persistence/base"
	"github.com/asaidimu/go-anansi/v8/core/persistence/collection"
	"github.com/asaidimu/go-anansi/v8/core/query"
	"github.com/asaidimu/go-anansi/v8/tests/testutils"
)

func TestMain(m *testing.M) {
	testutils.ConfigureDocumentFactory()
	os.Exit(m.Run())
}

// ---------------------------------------------------------------------------
// Test doubles
// ---------------------------------------------------------------------------

// docStore is an in-memory, thread-safe document store that implements
// enough of base.Collection to exercise LiveRepository's cache interceptors.
type docStore struct {
	mu    sync.RWMutex
	docs  map[string]*data.Document
	byKey map[string]string
}

func newDocStore() *docStore {
	return &docStore{
		docs:  make(map[string]*data.Document),
		byKey: make(map[string]string),
	}
}

func (s *docStore) CreateOne(_ context.Context, doc *data.Document) (base.CreateResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := doc.ID()
	if id == "" {
		return base.CreateResult{}, nil
	}
	s.docs[id] = doc
	return base.CreateResult{Status: base.StatusCreated, Data: doc}, nil
}

func (s *docStore) CreateMany(_ context.Context, docs []*data.Document) ([]base.CreateResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	res := make([]base.CreateResult, len(docs))
	for i, doc := range docs {
		id := doc.ID()
		if id != "" {
			s.docs[id] = doc
		}
		res[i] = base.CreateResult{Status: base.StatusCreated, Data: doc}
	}
	return res, nil
}

func (s *docStore) Read(_ context.Context, q *query.Query) (*base.ReadResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if q != nil && q.Filters != nil {
		if val := extractFilterValue(q.Filters); val != "" {
			if id, ok := s.byKey[val]; ok {
				if doc, exists := s.docs[id]; exists {
					return &base.ReadResult{Data: data.DocumentSet{doc}, Count: 1}, nil
				}
				delete(s.byKey, val)
			}
			return &base.ReadResult{Data: data.DocumentSet{}, Count: 0}, nil
		}
	}

	out := make(data.DocumentSet, 0, len(s.docs))
	for _, doc := range s.docs {
		out = append(out, doc)
	}
	return &base.ReadResult{Data: out, Count: len(out)}, nil
}

func (s *docStore) Update(_ context.Context, params *base.CollectionUpdate) (*base.ReadResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if params.Set == nil {
		return &base.ReadResult{}, nil
	}
	newName := params.Set.Must().GetString("name")

	// Find the document matching the filter.
	var targetID string
	if params.Filter != nil {
		if val := extractFilterValue(params.Filter); val != "" {
			targetID = s.byKey[val]
			// Also handle index update: if name changed, remove old index.
			delete(s.byKey, val)
		}
	}
	if targetID == "" {
		for id := range s.docs {
			targetID = id
			break
		}
	}

	doc := data.MustNewDocument(map[string]any{"name": newName})
	if targetID != "" {
		delete(s.docs, targetID)
	}
	s.docs[doc.ID()] = doc
	s.byKey[newName] = doc.ID()

	out := &base.ReadResult{Data: data.DocumentSet{doc}, Count: 1}
	if params.ReturnDocument {
		return out, nil
	}
	out.Data = nil
	return out, nil
}

func (s *docStore) Delete(_ context.Context, filter *query.QueryFilter, _ bool) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if filter == nil {
		count := len(s.docs)
		s.docs = make(map[string]*data.Document)
		s.byKey = make(map[string]string)
		return count, nil
	}
	val := extractFilterValue(filter)
	if val == "" {
		return 0, nil
	}
	id, ok := s.byKey[val]
	if !ok {
		return 0, nil
	}
	if _, exists := s.docs[id]; exists {
		delete(s.docs, id)
		delete(s.byKey, val)
		return 1, nil
	}
	return 0, nil
}

func (s *docStore) Validate(_ context.Context, _ *data.Document, _ bool) ([]common.Issue, bool) {
	return nil, true
}

func (s *docStore) Metadata(_ context.Context, _ *base.MetadataFilter, _ bool) *base.CollectionMetadata {
	return &base.CollectionMetadata{Name: "test"}
}

func (s *docStore) Subscribe(_ context.Context, _ base.SubscriptionOptions) string { return "" }
func (s *docStore) Unsubscribe(_ context.Context, _ string)                        {}
func (s *docStore) Subscriptions(_ context.Context) ([]base.SubscriptionInfo, error) {
	return nil, nil
}

func (s *docStore) Capabilities(_ context.Context) *query.Capabilities {
	return &query.Capabilities{}
}

func (s *docStore) indexKey(keyField, keyValue, docID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byKey[keyValue] = docID
}

// extractFilterValue is a simple helper that extracts a string value from a
// QueryFilter containing a single equality condition.
func extractFilterValue(f *query.QueryFilter) string {
	if f == nil {
		return ""
	}
	if f.Condition != nil && f.Condition.Operator == query.ComparisonOperatorEq {
		if f.Condition.Value.StringVal != nil {
			return *f.Condition.Value.StringVal
		}
	}
	if f.Group != nil {
		for _, sub := range f.Group.Conditions {
			if v := extractFilterValue(&sub); v != "" {
				return v
			}
		}
	}
	return ""
}

// recordingProcessor records every Create / Destroy call for test assertions.
type recordingProcessor struct {
	mu         sync.Mutex
	created    []string
	destroyed  []string
	createErr  error
	destroyErr error
}

func (p *recordingProcessor) Create(_ context.Context, doc *data.Document) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.createErr != nil {
		return "", p.createErr
	}
	key, _ := doc.Get("name")
	keyStr := fmt.Sprintf("%v", key)
	p.created = append(p.created, keyStr)
	return keyStr, nil
}

func (p *recordingProcessor) Compile(ctx context.Context, doc *data.Document) (string, error) {
	return p.Create(ctx, doc)
}

func (p *recordingProcessor) Destroy(_ context.Context, state string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.destroyErr != nil {
		return p.destroyErr
	}
	p.destroyed = append(p.destroyed, state)
	return nil
}

func (p *recordingProcessor) CloneState(state string) (string, error) {
	return state, nil
}

// recordingProcessorInt is like recordingProcessor but for int artifacts.
type recordingProcessorInt struct {
	mu        sync.Mutex
	created   int
	destroyed []int
}

func (p *recordingProcessorInt) Create(_ context.Context, doc *data.Document) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.created++
	return p.created, nil
}

func (p *recordingProcessorInt) Compile(ctx context.Context, doc *data.Document) (int, error) {
	return p.Create(ctx, doc)
}

func (p *recordingProcessorInt) Destroy(_ context.Context, state int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.destroyed = append(p.destroyed, state)
	return nil
}

func (p *recordingProcessorInt) CloneState(state int) (int, error) {
	return state, nil
}

// --- liveRepository test helpers ---

type testHarness struct {
	t       *testing.T
	ctx     context.Context
	store   *docStore
	proc    *recordingProcessor
	repo    collection.LiveCollection[string]
	cleanup func()
}

func newHarness(t *testing.T, opts ...func(*collection.LiveRepositoryOptions[string])) *testHarness {
	t.Helper()
	ctx := context.Background()
	store := newDocStore()
	proc := &recordingProcessor{}

	repoOpts := collection.LiveRepositoryOptions[string]{
		Collection: store,
		Processor:  proc,
		QueryKey:   "name",
		CacheConfig: &cache.CacheConfig{
			MaxEntries:     100,
			JanitorInterval: 0,
		},
	}
	for _, o := range opts {
		o(&repoOpts)
	}

	repo, err := collection.NewLiveRepository[string](ctx, repoOpts)
	if err != nil {
		t.Fatalf("NewLiveRepository: %v", err)
	}

	return &testHarness{
		t:     t,
		ctx:   ctx,
		store: store,
		proc:  proc,
		repo:  repo,
		cleanup: func() {
			_ = repo.Close()
		},
	}
}

func (h *testHarness) insertDoc(name string) *data.Document {
	doc := data.MustNewDocument(map[string]any{"name": name})
	res, err := h.store.CreateOne(h.ctx, doc)
	if err != nil {
		h.t.Fatalf("store.CreateOne: %v", err)
	}
	h.store.indexKey("name", name, res.Data.ID())
	return res.Data
}

// --- Tests ---

func TestLiveRepository_CreateOne_CachesArtifact(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()

	doc := data.MustNewDocument(map[string]any{"name": "alice"})
	result, err := h.repo.CreateOne(h.ctx, doc)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != base.StatusCreated {
		t.Fatalf("expected CREATED, got %v", result.Status)
	}

	val, ok := h.repo.Get("alice")
	if !ok {
		t.Fatal("expected cached artifact after CreateOne, got miss")
	}
	if val != "alice" {
		t.Fatalf("expected artifact 'alice', got %q", val)
	}
}

func TestLiveRepository_CreateOne_ProcessorError(t *testing.T) {
	ctx := context.Background()
	store := newDocStore()
	proc := &recordingProcessor{createErr: errTest}

	repo, err := collection.NewLiveRepository[string](ctx, collection.LiveRepositoryOptions[string]{
		Collection: store,
		Processor:  proc,
		QueryKey:   "name",
		CacheConfig: &cache.CacheConfig{
			MaxEntries:     100,
			JanitorInterval: 0,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	doc := data.MustNewDocument(map[string]any{"name": "bad"})
	_, err = repo.CreateOne(ctx, doc)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := repo.Get("bad"); ok {
		t.Fatal("expected cache miss when processor returns error")
	}
}

func TestLiveRepository_Get_ReadThrough(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()

	h.insertDoc("bob")

	val, ok := h.repo.Get("bob")
	if !ok {
		t.Fatal("expected hit after read-through")
	}
	if val != "bob" {
		t.Fatalf("expected 'bob', got %q", val)
	}

	before := len(h.proc.created)
	h.repo.Get("bob")
	if len(h.proc.created) != before {
		t.Fatal("read-through should only call Create once")
	}
}

func TestLiveRepository_Get_ReadThroughMiss(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()

	val, ok := h.repo.Get("nonexistent")
	if ok {
		t.Fatal("expected miss for nonexistent key")
	}
	if val != "" {
		t.Fatalf("expected zero value, got %q", val)
	}
}

func TestLiveRepository_Update_RefreshesCache(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()

	doc := data.MustNewDocument(map[string]any{"name": "carol"})
	_, err := h.repo.CreateOne(h.ctx, doc)
	if err != nil {
		t.Fatal(err)
	}

	updateDoc := data.MustNewDocument(map[string]any{"name": "carol-updated"})
	q := query.NewQueryBuilder().Where("name").Eq("carol").Build()
	_, err = h.repo.Update(h.ctx, &base.CollectionUpdate{
		Set:    updateDoc,
		Filter: q.Filters,
	})
	if err != nil {
		t.Fatal(err)
	}

	val, ok := h.repo.Get("carol-updated")
	if !ok {
		t.Fatal("expected new artifact after Update")
	}
	if val != "carol-updated" {
		t.Fatalf("expected 'carol-updated', got %q", val)
	}
}

func TestLiveRepository_Delete_CallsDestroy(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()

	h.insertDoc("dave")
	_, ok := h.repo.Get("dave")
	if !ok {
		t.Fatal("expected hit after read-through")
	}

	q := query.NewQueryBuilder().Where("name").Eq("dave").Build()
	n, err := h.repo.Delete(h.ctx, q.Filters, false)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 deleted, got %d", n)
	}

	if len(h.proc.destroyed) != 1 || h.proc.destroyed[0] != "dave" {
		t.Fatalf("expected Destroy('dave'), got destroyed=%v", h.proc.destroyed)
	}
}

func TestLiveRepository_Delete_ClearFallback_CallsDestroy(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()

	h.insertDoc("eve")
	h.insertDoc("frank")

	h.repo.Get("eve")
	h.repo.Get("frank")

	n, err := h.repo.Delete(h.ctx, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("expected 2 deleted, got %d", n)
	}

	if len(h.proc.destroyed) != 2 {
		t.Fatalf("expected 2 Destroy calls, got %d: %v", len(h.proc.destroyed), h.proc.destroyed)
	}
}

func TestLiveRepository_Unset_CallsDestroy(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()

	h.repo.Set("grace", "grace")
	h.repo.Unset("grace")

	if len(h.proc.destroyed) != 1 || h.proc.destroyed[0] != "grace" {
		t.Fatalf("expected Destroy('grace'), got destroyed=%v", h.proc.destroyed)
	}
	if _, ok := h.repo.Get("grace"); ok {
		t.Fatal("expected miss after Unset")
	}
}

func TestLiveRepository_NullifyWithTTL_CallsDestroy(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()

	h.repo.Set("heidi", "heidi")
	h.repo.NullifyWithTTL("heidi", 5*time.Minute)

	if len(h.proc.destroyed) != 1 || h.proc.destroyed[0] != "heidi" {
		t.Fatalf("expected Destroy('heidi'), got destroyed=%v", h.proc.destroyed)
	}
	if _, ok := h.repo.Get("heidi"); ok {
		t.Fatal("expected negative hit after NullifyWithTTL")
	}
}

func TestLiveRepository_NullifyWithTTL_NoPreviousValue(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()

	h.repo.NullifyWithTTL("phantom", time.Minute)

	if len(h.proc.destroyed) != 0 {
		t.Fatalf("expected no Destroy calls for never-set key, got %v", h.proc.destroyed)
	}
}

func TestLiveRepository_CapacityEviction_CallsDestroy(t *testing.T) {
	ctx := context.Background()
	store := newDocStore()
	proc := &recordingProcessorInt{}

	repo, err := collection.NewLiveRepository[int](ctx, collection.LiveRepositoryOptions[int]{
		Collection: store,
		Processor:  proc,
		QueryKey:   "name",
		CacheConfig: &cache.CacheConfig{
			MaxEntries:     2,
			ShardCount:     1,
			JanitorInterval: 0,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	repo.SetWithTTL("a", 1, cache.NoExpiration)
	repo.SetWithTTL("b", 2, cache.NoExpiration)
	repo.SetWithTTL("c", 3, cache.NoExpiration)

	if len(proc.destroyed) == 0 {
		t.Fatal("expected at least one Destroy call due to capacity eviction")
	}
}

func TestLiveRepository_ReadThrough_EvictionCallsDestroy(t *testing.T) {
	ctx := context.Background()
	store := newDocStore()
	proc := &recordingProcessorInt{}

	repo, err := collection.NewLiveRepository[int](ctx, collection.LiveRepositoryOptions[int]{
		Collection: store,
		Processor:  proc,
		QueryKey:   "name",
		CacheConfig: &cache.CacheConfig{
			MaxEntries:     2,
			ShardCount:     1,
			JanitorInterval: 0,
		},
	})

	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	for _, name := range []string{"alpha", "beta", "gamma"} {
		doc := data.MustNewDocument(map[string]any{"name": name})
		r, _ := store.CreateOne(ctx, doc)
		store.indexKey("name", name, r.Data.ID())
	}

	// Read-through three keys with MaxEntries=2. At least one should be
	// evicted and destroyed. CLOCK is an approximation, so we can't
	// predict which one; we only assert that some Destroy happened.
	for _, name := range []string{"alpha", "beta", "gamma"} {
		repo.Get(name)
	}

	if len(proc.destroyed) == 0 {
		t.Fatal("expected at least one Destroy call due to read-through eviction")
	}
}

func TestLiveRepository_AutoLoad(t *testing.T) {
	ctx := context.Background()
	store := newDocStore()
	proc := &recordingProcessor{}

	doc1 := data.MustNewDocument(map[string]any{"name": "ivan"})
	r1, _ := store.CreateOne(ctx, doc1)
	store.indexKey("name", "ivan", r1.Data.ID())

	doc2 := data.MustNewDocument(map[string]any{"name": "judy"})
	r2, _ := store.CreateOne(ctx, doc2)
	store.indexKey("name", "judy", r2.Data.ID())

	repo, err := collection.NewLiveRepository[string](ctx, collection.LiveRepositoryOptions[string]{
		Collection: store,
		Processor:  proc,
		QueryKey:   "name",
		CacheConfig: &cache.CacheConfig{
			MaxEntries:     100,
			JanitorInterval: 0,
		},
		AutoLoad: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	if _, ok := repo.Get("ivan"); !ok {
		t.Fatal("expected 'ivan' cached after AutoLoad")
	}
	if _, ok := repo.Get("judy"); !ok {
		t.Fatal("expected 'judy' cached after AutoLoad")
	}
	if len(proc.created) != 2 {
		t.Fatalf("expected 2 Create calls from AutoLoad, got %d", len(proc.created))
	}
}

func TestLiveRepository_Clone(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()

	h.repo.Set("karl", "karl")
	h.repo.Set("lisa", "lisa")

	clone, err := h.repo.Clone()
	if err != nil {
		t.Fatal(err)
	}
	defer clone.Close()

	for _, key := range []string{"karl", "lisa"} {
		val, ok := clone.Get(key)
		if !ok {
			t.Fatalf("expected %q in clone", key)
		}
		if val != key {
			t.Fatalf("expected %q in clone, got %q", key, val)
		}
	}
}

func TestLiveRepository_SetWithTTL_And_Persist(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()

	h.repo.SetWithTTL("mallory", "mallory", 5*time.Minute)

	ttl, ok := h.repo.TTL("mallory")
	if !ok {
		t.Fatal("expected TTL to report ok for cached key")
	}
	if ttl <= 0 || ttl > 6*time.Minute {
		t.Fatalf("expected TTL ~5m, got %v", ttl)
	}

	if !h.repo.Persist("mallory") {
		t.Fatal("Persist should return true")
	}
	persistedTTL, ok := h.repo.TTL("mallory")
	if !ok {
		t.Fatal("expected TTL to report ok after Persist")
	}
	if persistedTTL != cache.NoExpiration {
		t.Fatalf("expected NoExpiration after Persist, got %v", persistedTTL)
	}
}

func TestLiveRepository_Keys(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()

	h.repo.Set("nancy", "nancy")
	h.repo.Set("oscar", "oscar")

	keys := h.repo.Keys()
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d: %v", len(keys), keys)
	}
}

func TestLiveRepository_Concurrency(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()

	var wg sync.WaitGroup
	n := 20

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := fmt.Sprintf("user-%d", i)
			doc := data.MustNewDocument(map[string]any{"name": name})
			h.repo.CreateOne(h.ctx, doc)
		}(i)
	}

	var hits int32
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, ok := h.repo.Get("user-0"); ok {
				atomic.AddInt32(&hits, 1)
			}
		}()
	}

	wg.Wait()
}

var errTest = &testError{}

type testError struct{}

func (e *testError) Error() string { return "test error" }
