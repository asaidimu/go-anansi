// Package ecommerce demonstrates why document.Document outperforms map[string]any
// for a realistic e-commerce sale record and its linked transaction.
//
// Schema being modelled:
//
//   Sale {
//     id            bytes    (UUID)
//     order_ref     string
//     customer_id   bytes    (UUID)
//     status        int64    (enum ordinal: 0=pending 1=confirmed 2=shipped 3=cancelled)
//     line_items    []*Document (TypeArrayObject) → LineItem
//     subtotal_kes  int64    (minor units, i.e. cents × 100)
//     tax_kes       int64
//     total_kes     int64
//     discount_kes  int64    (nullable — not every sale has a discount)
//     coupon_code   string   (nullable)
//     created_at    int64    (unix nano)
//     transaction   *Document (TypeArrayObject, single element) → Transaction
//   }
//
//   LineItem {
//     sku           string
//     name          string
//     qty           int64
//     unit_price_kes int64
//     total_kes     int64
//   }
//
//   Transaction {
//     id            bytes    (UUID)
//     provider      string   (e.g. "mpesa", "card", "bank_transfer")
//     reference     string   (provider's own reference, nullable until confirmed)
//     amount_kes    int64
//     status        int64    (enum: 0=initiated 1=pending 2=settled 3=failed 4=reversed)
//     initiated_at  int64    (unix nano)
//     settled_at    int64    (unix nano, nullable)
//   }

package document_test

import (
	"testing"
	"unsafe"

	"github.com/asaidimu/go-anansi/v6/core/document"
)

// ═══════════════════════════════════════════════════════════════════════════════
// SECTION 1 — Key registry
//
// In a real system these come from schema.DocumentKey("field.path"). Here we
// construct them by hand so the file is self-contained. Each key encodes:
//   - DataType  (4 bits inside the DataPoint half)
//   - Ordinal   (27-bit stable identity, unique per field across schemas)
//   - Descriptor (upper 32 bits — carries required/unique/deprecated flags)
//
// The descriptor values below use a minimal encoding:
//   bit 5 (FDMaskRequired) = 0x20
//   bit 6 (FDMaskUnique)   = 0x40
// ═══════════════════════════════════════════════════════════════════════════════

func mustKey(typ document.DataType, ordinal int32, descriptor uint32) document.DocumentKey {
	dp, err := document.NewDataPoint(typ, ordinal)
	if err != nil {
		panic(err)
	}
	return document.NewDocumentKey(dp, descriptor)
}

// Sale field keys. Ordinals are arbitrary but must be unique across the schema.
var (
	// required + unique
	keySaleID = mustKey(document.TypeBytes, 1, 0x60)
	// required
	keySaleOrderRef    = mustKey(document.TypeString, 2, 0x20)
	keySaleCustomerID  = mustKey(document.TypeBytes, 3, 0x20)
	keySaleStatus      = mustKey(document.TypeInt, 4, 0x20)
	keySaleLineItems   = mustKey(document.TypeArrayObject, 5, 0x20)
	keySaleSubtotal    = mustKey(document.TypeInt, 6, 0x20)
	keySaleTax         = mustKey(document.TypeInt, 7, 0x20)
	keySaleTotal       = mustKey(document.TypeInt, 8, 0x20)
	keySaleTransaction = mustKey(document.TypeArrayObject, 9, 0x20)
	keySaleCreatedAt   = mustKey(document.TypeInt, 10, 0x20)
	// nullable — no required bit
	keySaleDiscount   = mustKey(document.TypeInt, 11, 0x00)
	keySaleCouponCode = mustKey(document.TypeString, 12, 0x00)
)

// LineItem field keys
var (
	keyItemSKU      = mustKey(document.TypeString, 20, 0x20)
	keyItemName     = mustKey(document.TypeString, 21, 0x20)
	keyItemQty      = mustKey(document.TypeInt, 22, 0x20)
	keyItemUnitPrice = mustKey(document.TypeInt, 23, 0x20)
	keyItemTotal    = mustKey(document.TypeInt, 24, 0x20)
)

// Transaction field keys
var (
	keyTxID          = mustKey(document.TypeBytes, 30, 0x60) // required + unique
	keyTxProvider    = mustKey(document.TypeString, 31, 0x20)
	keyTxAmount      = mustKey(document.TypeInt, 32, 0x20)
	keyTxStatus      = mustKey(document.TypeInt, 33, 0x20)
	keyTxInitiatedAt = mustKey(document.TypeInt, 34, 0x20)
	// nullable
	keyTxReference = mustKey(document.TypeString, 35, 0x00)
	keyTxSettledAt = mustKey(document.TypeInt, 36, 0x00)
)

// ═══════════════════════════════════════════════════════════════════════════════
// SECTION 2 — map[string]any approach
//
// This is what most Go services reach for first. It feels convenient because
// you can write it quickly — until you try to read any of it back.
// ═══════════════════════════════════════════════════════════════════════════════

type mapLineItem = map[string]any
type mapTransaction = map[string]any
type mapSale = map[string]any

func buildMapSale() mapSale {
	lineItems := []mapLineItem{
		{
			"sku":           "SKU-001",
			"name":          "Laptop Stand",
			"qty":           int64(2),
			"unit_price_kes": int64(350000), // KES 3,500.00 in minor units
			"total_kes":     int64(700000),
		},
		{
			"sku":           "SKU-002",
			"name":          "USB-C Hub",
			"qty":           int64(1),
			"unit_price_kes": int64(150000),
			"total_kes":     int64(150000),
		},
	}

	tx := mapTransaction{
		"id":           []byte("txn-uuid-bytes-here"),
		"provider":     "mpesa",
		"reference":    nil, // nullable: payment not yet confirmed
		"amount_kes":   int64(935000),
		"status":       int64(1), // pending
		"initiated_at": int64(1_700_000_000_000_000_000),
		"settled_at":   nil, // nullable
	}

	return mapSale{
		"id":           []byte("sale-uuid-bytes-here"),
		"order_ref":    "ORD-20240315-001",
		"customer_id":  []byte("cust-uuid-bytes-here"),
		"status":       int64(1), // confirmed
		"line_items":   lineItems,
		"subtotal_kes": int64(850000),
		"tax_kes":      int64(85000),
		"total_kes":    int64(935000),
		"discount_kes": nil,     // nullable: no discount on this order
		"coupon_code":  nil,     // nullable
		"created_at":   int64(1_700_000_000_000_000_000),
		"transaction":  tx,
	}
}

// readMapSaleTotals simulates a typical read: sum all line item totals and
// compare against sale.total_kes to detect drift. Watch how many type
// assertions are needed and how many can silently return zero instead of panicking.
func readMapSaleTotals(sale mapSale) (lineSum int64, saleTotal int64, discountApplied bool) {
	// PROBLEM 1: every read is a two-step: map lookup + type assertion.
	// The type assertion is unchecked — if someone stored float64 instead of
	// int64 (which json.Unmarshal does by default), this silently returns 0.
	saleTotal, _ = sale["total_kes"].(int64)

	// Add tax to line sum to match grand total
	if v, ok := sale["tax_kes"]; ok && v != nil {
		lineSum += v.(int64)
	}

	// PROBLEM 2: discount is nullable, but the map gives you the same `nil`
	// whether the field was never set or explicitly set to null. You cannot
	// distinguish "discount not applicable" from "discount field missing".
	if v, ok := sale["discount_kes"]; ok && v != nil {
		discountApplied = true
		lineSum -= v.(int64)
	}

	// PROBLEM 3: line_items is []mapLineItem but stored as `any`.
	// You must assert to the concrete slice type. If the producer used
	// []any instead of []mapLineItem (common from JSON decode), this panics.
	items, _ := sale["line_items"].([]mapLineItem)
	for _, item := range items {
		// Another unchecked assertion. A float64 slips through silently.
		t, _ := item["total_kes"].(int64)
		lineSum += t
	}

	return lineSum, saleTotal, discountApplied
}

// validateMapRequired checks that all required fields are present.
// PROBLEM 4: the schema lives outside the data. You need a separate, manually
// maintained list of required field names. It can drift from the actual schema.
func validateMapRequired(sale mapSale) []string {
	required := []string{
		"id", "order_ref", "customer_id", "status",
		"line_items", "subtotal_kes", "tax_kes", "total_kes",
		"created_at", "transaction",
	}
	var missing []string
	for _, field := range required {
		v, ok := sale[field]
		if !ok || v == nil {
			missing = append(missing, field)
		}
	}
	return missing
}

// ═══════════════════════════════════════════════════════════════════════════════
// SECTION 3 — document.Document approach
//
// The Document stores each type in its own contiguous slice. Reading a float
// never touches the string slice. Null and absent are distinct states. Required
// is encoded in the key's descriptor — it travels with the data, not beside it.
// ═══════════════════════════════════════════════════════════════════════════════

func buildDocumentSale() *document.Document {
	// --- Build line items ---
	item1 := document.NewDocument()
	item1.SetString(keyItemSKU, "SKU-001")
	item1.SetString(keyItemName, "Laptop Stand")
	item1.SetInt(keyItemQty, 2)
	item1.SetInt(keyItemUnitPrice, 350_000)
	item1.SetInt(keyItemTotal, 700_000)

	item2 := document.NewDocument()
	item2.SetString(keyItemSKU, "SKU-002")
	item2.SetString(keyItemName, "USB-C Hub")
	item2.SetInt(keyItemQty, 1)
	item2.SetInt(keyItemUnitPrice, 150_000)
	item2.SetInt(keyItemTotal, 150_000)

	// --- Build transaction ---
	tx := document.NewDocument()
	tx.SetBytes(keyTxID, []byte("txn-uuid-bytes-here"))
	tx.SetString(keyTxProvider, "mpesa")
	tx.SetNull(keyTxReference) // EXPLICIT null: payment not yet confirmed.
	tx.SetInt(keyTxAmount, 935_000)
	tx.SetInt(keyTxStatus, 1) // pending
	tx.SetInt(keyTxInitiatedAt, 1_700_000_000_000_000_000)
	tx.SetNull(keyTxSettledAt) // EXPLICIT null: not yet settled.

	// --- Build sale ---
	sale := document.NewDocument()
	sale.SetBytes(keySaleID, []byte("sale-uuid-bytes-here"))
	sale.SetString(keySaleOrderRef, "ORD-20240315-001")
	sale.SetBytes(keySaleCustomerID, []byte("cust-uuid-bytes-here"))
	sale.SetInt(keySaleStatus, 1) // confirmed
	sale.SetArrayObject(keySaleLineItems, []*document.Document{item1, item2})
	sale.SetInt(keySaleSubtotal, 850_000)
	sale.SetInt(keySaleTax, 85_000)
	sale.SetInt(keySaleTotal, 935_000)

	// Discount and coupon are not set at all — IsSet() returns false.
	// This is meaningfully different from SetNull(), which means "we checked
	// and there is no discount". In the map version both look identical.
	// sale.SetNull(keySaleDiscount) would mean: discount field present, value null.
	// Not calling SetNull at all means: discount field was never evaluated.

	sale.SetInt(keySaleCreatedAt, 1_700_000_000_000_000_000)
	sale.SetArrayObject(keySaleTransaction, []*document.Document{tx})

	return sale
}

// readDocumentSaleTotals does the same line-sum check as the map version.
// Notice: no type assertions, no interface boxing, three distinct null states.
func readDocumentSaleTotals(sale *document.Document) (lineSum int64, saleTotal int64, discountApplied bool, err error) {
	// Clean read: type is part of the key. ErrTypeMismatch is caught immediately
	// if a producer wrote the wrong type — not silently zeroed.
	saleTotal, _, err = sale.GetInt(keySaleTotal)
	if err != nil {
		return
	}

	// Add tax to line sum
	tax, _, err := sale.GetInt(keySaleTax)
	if err == nil {
		lineSum += tax
	}

	// THREE STATES:
	//   !IsSet(key)  → field was never written (not applicable in this context)
	//   IsNull(key)  → field was explicitly nulled ("discount evaluated: none")
	//   HasValue(key) → there is a concrete discount amount
	//
	// The map version collapses all three into nil vs. non-nil on a single key.
	if sale.HasValue(keySaleDiscount) {
		discountApplied = true
		discount, _, _ := sale.GetInt(keySaleDiscount)
		lineSum -= discount
	}

	items, _, err := sale.GetArrayObject(keySaleLineItems)
	if err != nil {
		return
	}
	for _, item := range items {
		// No assertion. TypeInt is part of keyItemTotal.
		// If a producer somehow called SetFloat on keyItemTotal, that call
		// itself would have returned ErrTypeMismatch at write time.
		t, _, e := item.GetInt(keyItemTotal)
		if e != nil {
			err = e
			return
		}
		lineSum += t
	}

	return lineSum, saleTotal, discountApplied, nil
}

// validateDocumentRequired checks required fields using the descriptor bits
// that are already encoded in each DocumentKey.
//
// ADVANTAGE: the schema is not a separate data structure. The required flag
// travels with the key. You cannot have a key that says "required" while the
// validation list says "optional" — they are the same bit.
func validateDocumentRequired(sale *document.Document, keys []document.DocumentKey) []document.DocumentKey {
	var missing []document.DocumentKey
	for _, key := range keys {
		// bit 5 of the descriptor is the required flag (FDMaskRequired = 0x20).
		isRequired := key.Descriptor()&0x20 != 0
		if isRequired && !sale.IsSet(key) {
			missing = append(missing, key)
		}
	}
	return missing
}

// allSaleKeys is the complete field set for a sale. In production this comes
// from ir.TerminalWalk over the compiled schema — no manual list needed.
var allSaleKeys = []document.DocumentKey{
	keySaleID, keySaleOrderRef, keySaleCustomerID, keySaleStatus,
	keySaleLineItems, keySaleSubtotal, keySaleTax, keySaleTotal,
	keySaleTransaction, keySaleCreatedAt,
	keySaleDiscount, keySaleCouponCode, // nullable, not required
}

// ═══════════════════════════════════════════════════════════════════════════════
// SECTION 4 — Correctness tests: prove the behaviour differences
// ═══════════════════════════════════════════════════════════════════════════════

func TestMapNullAmbiguity(t *testing.T) {
	sale := buildMapSale()

	// In the map, discount_kes was set to nil. coupon_code was also set to nil.
	// Are they absent or null? Both look identical.
	_, discountOk := sale["discount_kes"]
	_, couponOk := sale["coupon_code"]
	_, neverSetOk := sale["nonexistent_field"]

	// All three return `ok = true` because all three keys exist in the map —
	// the map does not distinguish "never set" from "explicitly null".
	t.Logf("discount_kes present in map: %v (value: %v)", discountOk, sale["discount_kes"])
	t.Logf("coupon_code present in map:  %v (value: %v)", couponOk, sale["coupon_code"])
	t.Logf("nonexistent present in map:  %v", neverSetOk)

	// Consequence: validateMapRequired treats nil fields as missing.
	// But nil was the producer's way of saying "no discount" — which is valid.
	// The validator cannot tell the difference without additional conventions.
	missing := validateMapRequired(sale)
	if len(missing) > 0 {
		t.Logf("map validator incorrectly flags as missing: %v", missing)
	}
}

func TestDocumentThreeStates(t *testing.T) {
	sale := buildDocumentSale()

	// discount_kes was never Set or SetNull — the field is absent entirely.
	if sale.IsSet(keySaleDiscount) {
		t.Error("discount should not be set")
	}

	// SetNull on the transaction's reference: present but valueless.
	txDocs, _, _ := sale.GetArrayObject(keySaleTransaction)
	tx := txDocs[0]
	if !tx.IsNull(keyTxReference) {
		t.Error("tx.reference should be explicitly null")
	}
	if tx.HasValue(keyTxReference) {
		t.Error("tx.reference should not have a value")
	}

	// provider is set and has a value.
	if !tx.HasValue(keyTxProvider) {
		t.Error("tx.provider should have a value")
	}

	// Validation: required fields all present, nullable ones correctly absent.
	missing := validateDocumentRequired(sale, allSaleKeys)
	if len(missing) > 0 {
		t.Errorf("required field validation incorrectly flagged %d keys", len(missing))
	}
}

func TestTotalConsistency(t *testing.T) {
	// Map version
	mapSale := buildMapSale()
	mapLineSum, mapTotal, _ := readMapSaleTotals(mapSale) //nolint
	if mapLineSum != mapTotal {
		t.Errorf("map: line sum %d != sale total %d", mapLineSum, mapTotal)
	}

	// Document version
	docSale := buildDocumentSale()
	docLineSum, docTotal, _, err := readDocumentSaleTotals(docSale)
	if err != nil {
		t.Fatalf("document read error: %v", err)
	}
	if docLineSum != docTotal {
		t.Errorf("document: line sum %d != sale total %d", docLineSum, docTotal)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// SECTION 5 — Benchmarks: memory layout is the key story
//
// map[string]any scatters every value on the heap as a separate interface{}.
// An interface is two words: a type pointer + a data pointer (or value if ≤ 1 word).
// For int64 on a 64-bit system, the value fits in the data word — no extra alloc.
// But the map bucket still allocates, and iteration touches random heap addresses.
//
// Document keeps all int64s in a single contiguous []int64. Iterating all
// integer fields is a sequential scan of one cache line block. For a sale with
// 7 int64 fields (subtotal, tax, total, status, created_at, discount, item totals)
// those all live in d.data[TypeInt] — one slice, one cache line.
// ═══════════════════════════════════════════════════════════════════════════════

func BenchmarkBuildMap(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		_ = buildMapSale()
	}
}

func BenchmarkBuildDocument(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		_ = buildDocumentSale()
	}
}

func BenchmarkReadMapTotals(b *testing.B) {
	sale := buildMapSale()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, _, _ = readMapSaleTotals(sale)
	}
}

func BenchmarkReadDocumentTotals(b *testing.B) {
	sale := buildDocumentSale()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, _, _, _ = readDocumentSaleTotals(sale)
	}
}

// BenchmarkDocumentWalkInts demonstrates the cache-line advantage.
// All int64 fields in the document live in a single slice. The Walk callback
// receives a direct pointer to that slice — no indirection, no boxing.
func BenchmarkDocumentWalkInts(b *testing.B) {
	sale := buildDocumentSale()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		sale.Walk(func(positions map[int64]int32, slot func(document.DataType, ...int) unsafe.Pointer) (any, error) {
			ints := *(*[]int64)(slot(document.TypeInt))
			var sum int64
			for _, v := range ints {
				sum += v
			}
			return sum, nil
		})
	}
}

// BenchmarkMapWalkInts does the equivalent: iterate the map and extract int64s.
// Every iteration touches a random bucket pointer and performs a type assertion.
func BenchmarkMapWalkInts(b *testing.B) {
	sale := buildMapSale()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		var sum int64
		for _, v := range sale {
			if i, ok := v.(int64); ok {
				sum += i
			}
		}
	}
}

// BenchmarkDocumentPooledRound simulates the hot path for a pooled document:
// get from pool → fill → read → clear → return. This is the allocation story
// for high-throughput endpoints: after warmup, zero allocations per request.
func BenchmarkDocumentPooledRound(b *testing.B) {
	// Warmup: allocate one Document and let it reach steady-state capacity.
	sale := buildDocumentSale()
	sale.Clear()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		// Simulate fill from deserialized wire data.
		sale.SetBytes(keySaleID, []byte("sale-uuid-bytes-here"))
		sale.SetString(keySaleOrderRef, "ORD-20240315-001")
		sale.SetBytes(keySaleCustomerID, []byte("cust-uuid-bytes-here"))
		sale.SetInt(keySaleStatus, 1)
		sale.SetInt(keySaleSubtotal, 850_000)
		sale.SetInt(keySaleTax, 85_000)
		sale.SetInt(keySaleTotal, 935_000)
		sale.SetInt(keySaleCreatedAt, 1_700_000_000_000_000_000)

		// Simulate read.
		sale.GetInt(keySaleTotal)
		sale.GetString(keySaleOrderRef)

		// Return to pool.
		sale.Clear()
	}
}
