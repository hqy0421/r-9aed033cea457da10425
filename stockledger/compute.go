package stockledger

import (
	"sort"
)

const (
	directionConsume = "consume"
	directionReverse = "reverse"
)

// Allocation is the cost of one sale (or one return reversal) attributed to a
// specific receipt batch. Quantity and CostCents are always positive; the
// Direction and the line type determine the sign of the movement.
type Allocation struct {
	Seq        int64
	EventID    string
	EventType  EventType
	Warehouse  string
	SKU        string
	Product    string
	Batch      string
	ReceiptID  string
	ReceiptSeq int64
	Quantity   int64
	CostCents  int64
	Direction  string
	Reference  string
}

// SignedQuantity returns the quantity as an inventory movement: positive for a
// sale consume, negative for a return reversal.
func (a Allocation) SignedQuantity() int64 {
	if a.Direction == directionReverse {
		return -a.Quantity
	}
	return a.Quantity
}

// SignedCostCents returns the cost as a COGS movement: positive for a sale,
// negative for a return reversal.
func (a Allocation) SignedCostCents() int64 {
	if a.Direction == directionReverse {
		return -a.CostCents
	}
	return a.CostCents
}

// Line is one sale or return with its revenue, COGS, and gross profit.
// Receipts are not lines; they appear in Batches.
type Line struct {
	Seq          int64
	ID           string
	Type         EventType
	Warehouse    string
	SKU          string
	Product      string
	Customer     string
	Reference    string
	Quantity     int64
	RevenueCents int64
	CostCents    int64
	GrossCents   int64
}

// Batch is the current valuation of one receipt batch.
type Batch struct {
	ID                 string
	ReceiptID          string
	ReceiptSeq         int64
	Warehouse          string
	SKU                string
	Product            string
	ReceiptQuantity    int64
	ReceiptCostCents   int64
	RemainingQuantity  int64
	RemainingCostCents int64
	ConsumedQuantity   int64
	ConsumedCostCents  int64
	ReturnedQuantity   int64
	ReturnedCostCents  int64
}

// CustomerProductSummary aggregates sales and returns for one customer and one
// product. Rows are retained even when every amount is zero.
type CustomerProductSummary struct {
	Customer             string
	SKU                  string
	Product              string
	SoldQuantity         int64
	ReturnedQuantity     int64
	NetQuantity          int64
	RevenueCents         int64
	ReturnedRevenueCents int64
	NetRevenueCents      int64
	COGSCents            int64
	ReversedCostCents    int64
	NetCOGSCents         int64
	GrossCents           int64
}

// CustomerSummary aggregates one customer across products.
type CustomerSummary struct {
	Customer        string
	NetQuantity     int64
	NetRevenueCents int64
	NetCOGSCents    int64
	GrossCents      int64
}

// ProductSummary aggregates one product across customers.
type ProductSummary struct {
	SKU             string
	Product         string
	NetQuantity     int64
	NetRevenueCents int64
	NetCOGSCents    int64
	GrossCents      int64
}

// Result is the complete cost-accounting result for one ledger version.
type Result struct {
	Version          string
	Lines            []Line
	Allocations      []Allocation
	Batches          []Batch
	CustomerProducts []CustomerProductSummary
	Customers        []CustomerSummary
	Products         []ProductSummary
	NetRevenueCents  int64
	COGSCents        int64
	GrossCents       int64
	SoldQuantity     int64
	ReturnedQuantity int64
}

// saleState tracks a sale so that later returns can reverse its allocations.
type saleState struct {
	line          *Line
	allocations   []*allocationState
	returnedCents int64
}

// allocationState tracks how much of one sale-to-batch allocation has not yet
// been returned.
type allocationState struct {
	allocation    Allocation
	batch         *Batch
	remainingQty  int64
	remainingCost int64
}

// Compute replays the ledger in sequence order and returns the FIFO cost
// accounting result. It fails on insufficient stock, excess or out-of-order
// returns, duplicate batches, and any inconsistent reference between a return
// and its original sale.
func (l *Ledger) Compute() (*Result, error) {
	if l == nil {
		return nil, errorf(ErrReadFailed, "nil ledger")
	}
	result := &Result{Version: l.Version}
	batches := map[string]*Batch{}
	sales := map[string]*saleState{}
	eventTypes := map[string]EventType{}
	for _, row := range l.Rows {
		eventTypes[row.ID] = row.Type
	}
	// Every total below is a sum of a subset of the row amounts and
	// quantities, so bounding the sum of the absolute values bounds all of
	// them. This keeps the cent arithmetic exact instead of wrapping silently.
	if err := checkTotalsBound(l.Rows); err != nil {
		return nil, err
	}

	for _, row := range l.Rows {
		var err error
		switch row.Type {
		case EventReceipt:
			err = applyReceipt(result, batches, row)
		case EventSale:
			err = applySale(result, batches, sales, row)
		case EventReturn:
			err = applyReturn(result, sales, eventTypes, row)
		default:
			err = errorf(ErrUnknownEventType, "unsupported event type %q", row.Type)
		}
		if err != nil {
			return nil, err
		}
	}
	buildSummaries(result, batches)
	return result, nil
}

// checkTotalsBound rejects a ledger whose summed amounts or quantities could
// overflow int64. Ledgers this large are corrupt or hostile rather than real,
// and failing loudly is better than wrapping a total.
func checkTotalsBound(rows []Row) error {
	amounts := int64(0)
	quantities := int64(0)
	for _, row := range rows {
		if row.Cents > maxAmount-amounts {
			return errorf(ErrOverflow, "the ledger amounts sum beyond the supported maximum")
		}
		amounts += row.Cents
		if row.Quantity > maxQuantity-quantities {
			return errorf(ErrOverflow, "the ledger quantities sum beyond the supported maximum")
		}
		quantities += row.Quantity
	}
	return nil
}

func applyReceipt(result *Result, batches map[string]*Batch, row Row) error {
	if existing, ok := batches[row.Batch]; ok {
		return errorf(ErrDuplicateBatch,
			"batch %q is already owned by receipt %s; receipt %s cannot reuse it",
			row.Batch, existing.ReceiptID, row.ID)
	}
	batches[row.Batch] = &Batch{
		ID:                 row.Batch,
		ReceiptID:          row.ID,
		ReceiptSeq:         row.Seq,
		Warehouse:          row.Warehouse,
		SKU:                row.SKU,
		Product:            row.Product,
		ReceiptQuantity:    row.Quantity,
		ReceiptCostCents:   row.Cents,
		RemainingQuantity:  row.Quantity,
		RemainingCostCents: row.Cents,
	}
	return nil
}

func applySale(result *Result, batches map[string]*Batch, sales map[string]*saleState, row Row) error {
	if _, ok := sales[row.ID]; ok {
		return errorf(ErrConflictingEvent, "sale id %s appears twice", row.ID)
	}
	line := &Line{
		Seq:          row.Seq,
		ID:           row.ID,
		Type:         EventSale,
		Warehouse:    row.Warehouse,
		SKU:          row.SKU,
		Product:      row.Product,
		Customer:     row.Customer,
		Quantity:     row.Quantity,
		RevenueCents: row.Cents,
	}
	state := &saleState{line: line}
	if err := consumeFIFO(result, state, batches, row); err != nil {
		return err
	}
	result.Lines = append(result.Lines, *line)
	sales[row.ID] = state
	return nil
}

// consumeFIFO allocates a sale across matching receipt batches in receipt
// sequence order. Batches that are fully consumed stay in the queue; a later
// return puts quantity back into its original batch, which restores that
// batch's place in the FIFO order.
func consumeFIFO(result *Result, state *saleState, batches map[string]*Batch, row Row) error {
	available := int64(0)
	candidates := make([]*Batch, 0, len(batches))
	for _, batch := range batches {
		if batch.Warehouse == row.Warehouse && batch.SKU == row.SKU {
			candidates = append(candidates, batch)
			available += batch.RemainingQuantity
		}
	}
	if available < row.Quantity {
		return errorf(ErrInsufficientStock,
			"sale %s (seq %d) needs %d units of %s in %s, but only %d units are available",
			row.ID, row.Seq, row.Quantity, row.SKU, row.Warehouse, available)
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].ReceiptSeq != candidates[j].ReceiptSeq {
			return candidates[i].ReceiptSeq < candidates[j].ReceiptSeq
		}
		return candidates[i].ID < candidates[j].ID
	})

	remaining := row.Quantity
	for _, batch := range candidates {
		if remaining == 0 {
			break
		}
		if batch.RemainingQuantity == 0 {
			continue
		}
		taken := remaining
		if taken > batch.RemainingQuantity {
			taken = batch.RemainingQuantity
		}
		cost := consumeCost(batch.RemainingCostCents, batch.RemainingQuantity, taken)
		batch.RemainingQuantity -= taken
		batch.RemainingCostCents -= cost
		batch.ConsumedQuantity += taken
		batch.ConsumedCostCents += cost
		remaining -= taken

		state.allocations = append(state.allocations, &allocationState{
			allocation: Allocation{
				Seq:        row.Seq,
				EventID:    row.ID,
				EventType:  EventSale,
				Warehouse:  row.Warehouse,
				SKU:        row.SKU,
				Product:    row.Product,
				Batch:      batch.ID,
				ReceiptID:  batch.ReceiptID,
				ReceiptSeq: batch.ReceiptSeq,
				Quantity:   taken,
				CostCents:  cost,
				Direction:  directionConsume,
			},
			batch:         batch,
			remainingQty:  taken,
			remainingCost: cost,
		})
		result.Allocations = append(result.Allocations, state.allocations[len(state.allocations)-1].allocation)
		state.line.CostCents += cost
	}
	state.line.GrossCents = state.line.RevenueCents - state.line.CostCents
	return nil
}

func applyReturn(result *Result, sales map[string]*saleState, eventTypes map[string]EventType, row Row) error {
	original, ok := sales[row.Reference]
	if !ok {
		if eventType, known := eventTypes[row.Reference]; known {
			if eventType != EventSale {
				return errorf(ErrReferenceNotSale,
					"return %s references %s, which is a %s, not a sale",
					row.ID, row.Reference, eventType)
			}
			return errorf(ErrOutOfOrderReference,
				"return %s (seq %d) references sale %s, which appears later in the ledger",
				row.ID, row.Seq, row.Reference)
		}
		return errorf(ErrUnknownReference,
			"return %s references sale %s, which does not exist in this ledger",
			row.ID, row.Reference)
	}
	sale := original.line
	if row.Warehouse != sale.Warehouse {
		return errorf(ErrWarehouseMismatch,
			"return %s is in warehouse %s but sale %s is in %s",
			row.ID, row.Warehouse, sale.ID, sale.Warehouse)
	}
	if row.SKU != sale.SKU {
		return errorf(ErrSKUMismatch,
			"return %s is for SKU %s but sale %s is for SKU %s",
			row.ID, row.SKU, sale.ID, sale.SKU)
	}
	if row.Customer != "" && sale.Customer != "" && row.Customer != sale.Customer {
		return errorf(ErrCustomerMismatch,
			"return %s is for customer %s but sale %s is for customer %s",
			row.ID, row.Customer, sale.ID, sale.Customer)
	}

	unreturned := int64(0)
	for _, allocation := range original.allocations {
		unreturned += allocation.remainingQty
	}
	if row.Quantity > unreturned {
		return errorf(ErrExcessReturn,
			"return %s returns %d units but sale %s has only %d unreturned",
			row.ID, row.Quantity, sale.ID, unreturned)
	}
	if original.returnedCents+row.Cents > sale.RevenueCents {
		return errorf(ErrExcessReturn,
			"return %s of %d cents would exceed sale %s revenue (%d of %d cents already returned)",
			row.ID, row.Cents, sale.ID, original.returnedCents, sale.RevenueCents)
	}

	customer := row.Customer
	if customer == "" {
		customer = sale.Customer
	}
	product := row.Product
	if product == "" {
		product = sale.Product
	}
	line := Line{
		Seq:          row.Seq,
		ID:           row.ID,
		Type:         EventReturn,
		Warehouse:    row.Warehouse,
		SKU:          row.SKU,
		Product:      product,
		Customer:     customer,
		Reference:    sale.ID,
		Quantity:     row.Quantity,
		RevenueCents: -row.Cents,
	}

	// Reverse the original sale's allocations from the most recently consumed
	// batch backwards, restoring the exact cost to that same batch. A return is
	// never treated as a new receipt batch and never uses a current average.
	remaining := row.Quantity
	reversedCost := int64(0)
	for i := len(original.allocations) - 1; i >= 0 && remaining > 0; i-- {
		allocation := original.allocations[i]
		if allocation.remainingQty == 0 {
			continue
		}
		taken := remaining
		if taken > allocation.remainingQty {
			taken = allocation.remainingQty
		}
		cost := consumeCost(allocation.remainingCost, allocation.remainingQty, taken)
		allocation.remainingQty -= taken
		allocation.remainingCost -= cost
		remaining -= taken
		reversedCost += cost

		batch := allocation.batch
		batch.RemainingQuantity += taken
		batch.RemainingCostCents += cost
		batch.ReturnedQuantity += taken
		batch.ReturnedCostCents += cost

		reversal := allocation.allocation
		reversal.Seq = row.Seq
		reversal.EventID = row.ID
		reversal.EventType = EventReturn
		reversal.Quantity = taken
		reversal.CostCents = cost
		reversal.Direction = directionReverse
		reversal.Reference = sale.ID
		result.Allocations = append(result.Allocations, reversal)
	}
	line.CostCents = -reversedCost
	line.GrossCents = line.RevenueCents - line.CostCents
	result.Lines = append(result.Lines, line)
	original.returnedCents += row.Cents
	return nil
}

func buildSummaries(result *Result, batches map[string]*Batch) {
	sort.SliceStable(result.Lines, func(i, j int) bool {
		if result.Lines[i].Seq != result.Lines[j].Seq {
			return result.Lines[i].Seq < result.Lines[j].Seq
		}
		return result.Lines[i].ID < result.Lines[j].ID
	})
	// Allocations are appended while events are replayed in sequence order, and
	// within one event they are recorded in the order they were applied: FIFO
	// (oldest batch first) for a sale, and most-recently-consumed first for a
	// return reversal. A stable sort therefore preserves that meaningful order.
	sort.SliceStable(result.Allocations, func(i, j int) bool {
		if result.Allocations[i].Seq != result.Allocations[j].Seq {
			return result.Allocations[i].Seq < result.Allocations[j].Seq
		}
		return result.Allocations[i].EventID < result.Allocations[j].EventID
	})
	for _, batch := range batches {
		result.Batches = append(result.Batches, *batch)
	}
	sort.SliceStable(result.Batches, func(i, j int) bool {
		if result.Batches[i].ReceiptSeq != result.Batches[j].ReceiptSeq {
			return result.Batches[i].ReceiptSeq < result.Batches[j].ReceiptSeq
		}
		return result.Batches[i].ID < result.Batches[j].ID
	})

	productIndex := map[string]int{}
	customerProductIndex := map[string]int{}
	customerIndex := map[string]int{}
	for _, line := range result.Lines {
		netQty := line.Quantity
		if line.Type == EventReturn {
			netQty = -line.Quantity
		}
		result.NetRevenueCents += line.RevenueCents
		result.COGSCents += line.CostCents
		result.GrossCents += line.GrossCents
		if netQty > 0 {
			result.SoldQuantity += netQty
		} else {
			result.ReturnedQuantity -= netQty
		}

		customer := line.Customer
		if customer == "" {
			customer = "-"
		}
		cpKey := customer + "\x00" + line.SKU
		cpPosition, ok := customerProductIndex[cpKey]
		if !ok {
			result.CustomerProducts = append(result.CustomerProducts, CustomerProductSummary{
				Customer: customer,
				SKU:      line.SKU,
				Product:  line.Product,
			})
			cpPosition = len(result.CustomerProducts) - 1
			customerProductIndex[cpKey] = cpPosition
		}
		cp := &result.CustomerProducts[cpPosition]
		if line.Type == EventSale {
			cp.SoldQuantity += line.Quantity
			cp.RevenueCents += line.RevenueCents
			cp.COGSCents += line.CostCents
		} else {
			cp.ReturnedQuantity += line.Quantity
			cp.ReturnedRevenueCents += -line.RevenueCents
			cp.ReversedCostCents += -line.CostCents
		}
		cp.NetQuantity = cp.SoldQuantity - cp.ReturnedQuantity
		cp.NetRevenueCents = cp.RevenueCents - cp.ReturnedRevenueCents
		cp.NetCOGSCents = cp.COGSCents - cp.ReversedCostCents
		cp.GrossCents = cp.NetRevenueCents - cp.NetCOGSCents

		cPosition, ok := customerIndex[customer]
		if !ok {
			result.Customers = append(result.Customers, CustomerSummary{Customer: customer})
			cPosition = len(result.Customers) - 1
			customerIndex[customer] = cPosition
		}
		c := &result.Customers[cPosition]
		c.NetQuantity += netQty
		c.NetRevenueCents += line.RevenueCents
		c.NetCOGSCents += line.CostCents
		c.GrossCents += line.GrossCents

		productPosition, ok := productIndex[line.SKU]
		if !ok {
			result.Products = append(result.Products, ProductSummary{SKU: line.SKU, Product: line.Product})
			productPosition = len(result.Products) - 1
			productIndex[line.SKU] = productPosition
		}
		p := &result.Products[productPosition]
		p.NetQuantity += netQty
		p.NetRevenueCents += line.RevenueCents
		p.NetCOGSCents += line.CostCents
		p.GrossCents += line.GrossCents
	}

	sort.SliceStable(result.CustomerProducts, func(i, j int) bool {
		if result.CustomerProducts[i].Customer != result.CustomerProducts[j].Customer {
			return result.CustomerProducts[i].Customer < result.CustomerProducts[j].Customer
		}
		return result.CustomerProducts[i].SKU < result.CustomerProducts[j].SKU
	})
	sort.SliceStable(result.Customers, func(i, j int) bool {
		return result.Customers[i].Customer < result.Customers[j].Customer
	})
	sort.SliceStable(result.Products, func(i, j int) bool {
		if result.Products[i].SKU != result.Products[j].SKU {
			return result.Products[i].SKU < result.Products[j].SKU
		}
		return result.Products[i].Product < result.Products[j].Product
	})
}

// consumeCost returns the exact cost of taking `taken` units from a slice that
// holds `quantity` units worth `cost` cents. Each draw costs
// floor(cost*taken/quantity), so the remainder of the slice keeps the leftover
// cent; consuming everything costs exactly the slice cost and no cent is ever
// lost or created.
func consumeCost(cost, quantity, taken int64) int64 {
	if taken >= quantity {
		return cost
	}
	if taken <= 0 {
		return 0
	}
	return mulDiv(cost, taken, quantity)
}
