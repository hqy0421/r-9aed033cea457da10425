package stockledger_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sclevine/agouti/stockledger"
)

func receiptRow(id string, seq int, warehouse, sku, batch string, qty int, amount string) stockledger.Attributes {
	return row(map[string]string{
		"data-type": "receipt", "data-id": id, "data-seq": itoa(seq),
		"data-warehouse": warehouse, "data-sku": sku, "data-batch": batch,
		"data-qty": itoa(qty), "data-amount": amount,
	})
}

func saleRow(id string, seq int, warehouse, sku, customer string, qty int, amount string) stockledger.Attributes {
	return row(map[string]string{
		"data-type": "sale", "data-id": id, "data-seq": itoa(seq),
		"data-warehouse": warehouse, "data-sku": sku, "data-customer": customer,
		"data-qty": itoa(qty), "data-amount": amount,
	})
}

func returnRow(id string, seq int, warehouse, sku, reference string, qty int, amount string) stockledger.Attributes {
	return row(map[string]string{
		"data-type": "return", "data-id": id, "data-seq": itoa(seq),
		"data-warehouse": warehouse, "data-sku": sku, "data-ref": reference,
		"data-qty": itoa(qty), "data-amount": amount,
	})
}

func itoa(value int) string {
	if value < 0 {
		return "-" + itoa(-value)
	}
	if value < 10 {
		return string(rune('0' + value))
	}
	return itoa(value/10) + string(rune('0'+value%10))
}

func compute(version string, rows ...stockledger.Attributes) (*stockledger.Result, error) {
	ledger, err := stockledger.ParseRows(version, rows)
	if err != nil {
		return nil, err
	}
	return ledger.Compute()
}

// allocationsFor returns the allocations of one event as "batch:qty:cents"
// strings, in allocation order.
func allocationsFor(result *stockledger.Result, eventID string) []string {
	var rendered []string
	for _, allocation := range result.Allocations {
		if allocation.EventID == eventID {
			rendered = append(rendered, allocation.Batch+":"+itoa(int(allocation.Quantity))+":"+stockledger.FormatCents(allocation.CostCents))
		}
	}
	return rendered
}

func batchByName(result *stockledger.Result, id string) stockledger.Batch {
	for _, batch := range result.Batches {
		if batch.ID == id {
			return batch
		}
	}
	return stockledger.Batch{}
}

func lineByID(result *stockledger.Result, id string) stockledger.Line {
	for _, line := range result.Lines {
		if line.ID == id {
			return line
		}
	}
	return stockledger.Line{}
}

var _ = Describe("FIFO cost accounting", func() {
	It("allocates a sale across receipt batches in first-in-first-out order", func() {
		result, err := compute("v1",
			receiptRow("R1", 1, "WH1", "A", "LOT1", 10, "100.00"),
			receiptRow("R2", 2, "WH1", "A", "LOT2", 10, "140.00"),
			saleRow("S1", 3, "WH1", "A", "C1", 12, "240.00"),
		)
		Expect(err).ToNot(HaveOccurred())
		Expect(allocationsFor(result, "S1")).To(Equal([]string{"LOT1:10:100.00", "LOT2:2:28.00"}))
		Expect(lineByID(result, "S1").CostCents).To(Equal(int64(12800)))
		Expect(result.NetRevenueCents).To(Equal(int64(24000)))
		Expect(result.COGSCents).To(Equal(int64(12800)))
		Expect(result.GrossCents).To(Equal(int64(11200)))

		Expect(batchByName(result, "LOT1").RemainingQuantity).To(Equal(int64(0)))
		Expect(batchByName(result, "LOT1").RemainingCostCents).To(Equal(int64(0)))
		Expect(batchByName(result, "LOT2").RemainingQuantity).To(Equal(int64(8)))
		Expect(batchByName(result, "LOT2").RemainingCostCents).To(Equal(int64(11200)))
	})

	It("splits a batch's cent cost exactly and never invents or loses a cent", func() {
		// 3 units for 10.00 cannot split evenly: 3.33 + 3.33 + 3.34.
		result, err := compute("v1",
			receiptRow("R1", 1, "WH1", "A", "LOT1", 3, "10.00"),
			saleRow("S1", 2, "WH1", "A", "C1", 1, "5.00"),
			saleRow("S2", 3, "WH1", "A", "C1", 1, "5.00"),
			saleRow("S3", 4, "WH1", "A", "C1", 1, "5.00"),
		)
		Expect(err).ToNot(HaveOccurred())
		Expect(allocationsFor(result, "S1")).To(Equal([]string{"LOT1:1:3.33"}))
		Expect(allocationsFor(result, "S2")).To(Equal([]string{"LOT1:1:3.33"}))
		Expect(allocationsFor(result, "S3")).To(Equal([]string{"LOT1:1:3.34"}))
		Expect(result.COGSCents).To(Equal(int64(1000)))
		Expect(batchByName(result, "LOT1").RemainingCostCents).To(Equal(int64(0)))
	})

	It("reverses a partial return from the last batch the sale consumed", func() {
		result, err := compute("v1",
			receiptRow("R1", 1, "WH1", "A", "LOT1", 10, "100.00"),
			receiptRow("R2", 2, "WH1", "A", "LOT2", 10, "140.00"),
			saleRow("S1", 3, "WH1", "A", "C1", 12, "240.00"),
			returnRow("T1", 4, "WH1", "A", "S1", 3, "60.00"),
		)
		Expect(err).ToNot(HaveOccurred())
		// Reverse from LOT2 (the most recent allocation) backwards.
		Expect(allocationsFor(result, "T1")).To(Equal([]string{"LOT2:2:28.00", "LOT1:1:10.00"}))
		Expect(lineByID(result, "T1").CostCents).To(Equal(int64(-3800)))
		Expect(lineByID(result, "T1").GrossCents).To(Equal(int64(-2200)))

		// The original sale keeps its own cost; the reversal is separate.
		Expect(lineByID(result, "S1").CostCents).To(Equal(int64(12800)))
		Expect(result.NetRevenueCents).To(Equal(int64(18000)))
		Expect(result.COGSCents).To(Equal(int64(9000)))
		Expect(result.GrossCents).To(Equal(int64(9000)))

		Expect(batchByName(result, "LOT1").RemainingQuantity).To(Equal(int64(1)))
		Expect(batchByName(result, "LOT1").RemainingCostCents).To(Equal(int64(1000)))
		Expect(batchByName(result, "LOT2").RemainingQuantity).To(Equal(int64(10)))
		Expect(batchByName(result, "LOT2").RemainingCostCents).To(Equal(int64(14000)))
	})

	It("restores the returned units to their original batch and position for a later sale", func() {
		result, err := compute("v1",
			receiptRow("R1", 1, "WH1", "A", "LOT1", 10, "100.00"),
			receiptRow("R2", 2, "WH1", "A", "LOT2", 10, "300.00"),
			saleRow("S1", 3, "WH1", "A", "C1", 10, "500.00"),
			returnRow("T1", 4, "WH1", "A", "S1", 4, "200.00"),
			saleRow("S2", 5, "WH1", "A", "C2", 6, "300.00"),
		)
		Expect(err).ToNot(HaveOccurred())
		// LOT1 is restored to 4 units, so the next sale must take it before LOT2.
		Expect(allocationsFor(result, "S2")).To(Equal([]string{"LOT1:4:40.00", "LOT2:2:60.00"}))
		Expect(batchByName(result, "LOT1").RemainingQuantity).To(Equal(int64(0)))
		Expect(batchByName(result, "LOT2").RemainingQuantity).To(Equal(int64(8)))
	})

	It("never treats a return as a new receipt batch", func() {
		result, err := compute("v1",
			receiptRow("R1", 1, "WH1", "A", "LOT1", 5, "50.00"),
			saleRow("S1", 2, "WH1", "A", "C1", 5, "80.00"),
			returnRow("T1", 3, "WH1", "A", "S1", 2, "32.00"),
		)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.Batches).To(HaveLen(1))
		Expect(result.Batches[0].ID).To(Equal("LOT1"))
		Expect(result.Batches[0].RemainingQuantity).To(Equal(int64(2)))
		Expect(result.Batches[0].RemainingCostCents).To(Equal(int64(2000)))
	})

	It("costs a return from the original cost, not from a current average", func() {
		result, err := compute("v1",
			receiptRow("R1", 1, "WH1", "A", "LOT1", 10, "100.00"),
			saleRow("S1", 2, "WH1", "A", "C1", 10, "200.00"),
			receiptRow("R2", 3, "WH1", "A", "LOT2", 10, "1000.00"),
			returnRow("T1", 4, "WH1", "A", "S1", 1, "20.00"),
		)
		Expect(err).ToNot(HaveOccurred())
		// The average cost after R2 would be 55.00, but the return reverses 10.00.
		Expect(allocationsFor(result, "T1")).To(Equal([]string{"LOT1:1:10.00"}))
		Expect(lineByID(result, "T1").CostCents).To(Equal(int64(-1000)))
		Expect(batchByName(result, "LOT1").RemainingCostCents).To(Equal(int64(1000)))
		Expect(batchByName(result, "LOT2").RemainingCostCents).To(Equal(int64(100000)))
	})

	It("reverses the most recent allocations first across several partial returns", func() {
		result, err := compute("v1",
			receiptRow("R1", 1, "WH1", "A", "LOT1", 5, "50.00"),
			receiptRow("R2", 2, "WH1", "A", "LOT2", 5, "75.00"),
			saleRow("S1", 3, "WH1", "A", "C1", 8, "160.00"),
			returnRow("T1", 4, "WH1", "A", "S1", 2, "40.00"),
			returnRow("T2", 5, "WH1", "A", "S1", 4, "80.00"),
		)
		Expect(err).ToNot(HaveOccurred())
		Expect(allocationsFor(result, "S1")).To(Equal([]string{"LOT1:5:50.00", "LOT2:3:45.00"}))
		Expect(allocationsFor(result, "T1")).To(Equal([]string{"LOT2:2:30.00"}))
		Expect(allocationsFor(result, "T2")).To(Equal([]string{"LOT2:1:15.00", "LOT1:3:30.00"}))
		Expect(batchByName(result, "LOT1").RemainingQuantity).To(Equal(int64(3)))
		Expect(batchByName(result, "LOT2").RemainingQuantity).To(Equal(int64(5)))
		Expect(result.COGSCents).To(Equal(int64(2000)))
		Expect(result.NetRevenueCents).To(Equal(int64(4000)))
	})

	It("keeps batches per warehouse and never allocates across warehouses", func() {
		result, err := compute("v1",
			receiptRow("R1", 1, "WH1", "A", "LOT1", 5, "50.00"),
			receiptRow("R2", 2, "WH2", "A", "LOT2", 5, "90.00"),
			saleRow("S1", 3, "WH2", "A", "C1", 3, "30.00"),
		)
		Expect(err).ToNot(HaveOccurred())
		Expect(allocationsFor(result, "S1")).To(Equal([]string{"LOT2:3:54.00"}))
		Expect(batchByName(result, "LOT1").RemainingQuantity).To(Equal(int64(5)))
		Expect(batchByName(result, "LOT2").RemainingQuantity).To(Equal(int64(2)))
	})

	It("keeps batches per SKU and never allocates across products", func() {
		result, err := compute("v1",
			receiptRow("R1", 1, "WH1", "A", "LOT-A", 5, "50.00"),
			receiptRow("R2", 2, "WH1", "B", "LOT-B", 5, "500.00"),
			saleRow("S1", 3, "WH1", "B", "C1", 2, "300.00"),
		)
		Expect(err).ToNot(HaveOccurred())
		Expect(allocationsFor(result, "S1")).To(Equal([]string{"LOT-B:2:200.00"}))
	})

	It("retains zero-cost and zero-balance rows instead of dropping them", func() {
		result, err := compute("v1",
			receiptRow("R1", 1, "WH1", "A", "FREE", 2, "0.00"),
			saleRow("S1", 2, "WH1", "A", "C1", 2, "10.00"),
			receiptRow("R2", 3, "WH1", "A", "LOT2", 2, "20.00"),
			saleRow("S2", 4, "WH1", "A", "C1", 2, "30.00"),
		)
		Expect(err).ToNot(HaveOccurred())
		Expect(lineByID(result, "S1").CostCents).To(Equal(int64(0)))
		Expect(lineByID(result, "S1").GrossCents).To(Equal(int64(1000)))
		Expect(allocationsFor(result, "S1")).To(Equal([]string{"FREE:2:0.00"}))
		Expect(result.Batches).To(HaveLen(2))
		Expect(batchByName(result, "FREE").RemainingQuantity).To(Equal(int64(0)))
		Expect(batchByName(result, "FREE").RemainingCostCents).To(Equal(int64(0)))
		Expect(result.CustomerProducts).To(HaveLen(1))
		Expect(result.CustomerProducts[0].NetRevenueCents).To(Equal(int64(4000)))
	})

	It("summarises by customer and product including returns", func() {
		result, err := compute("v1",
			receiptRow("R1", 1, "WH1", "A", "LOT1", 10, "100.00"),
			receiptRow("R2", 2, "WH1", "B", "LOT2", 10, "200.00"),
			saleRow("S1", 3, "WH1", "A", "C1", 4, "100.00"),
			saleRow("S2", 4, "WH1", "B", "C1", 2, "60.00"),
			saleRow("S3", 5, "WH1", "A", "C2", 1, "30.00"),
			returnRow("T1", 6, "WH1", "A", "S1", 1, "25.00"),
		)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.CustomerProducts).To(HaveLen(3))

		first := result.CustomerProducts[0]
		Expect(first.Customer).To(Equal("C1"))
		Expect(first.SKU).To(Equal("A"))
		Expect(first.SoldQuantity).To(Equal(int64(4)))
		Expect(first.ReturnedQuantity).To(Equal(int64(1)))
		Expect(first.NetQuantity).To(Equal(int64(3)))
		Expect(first.RevenueCents).To(Equal(int64(10000)))
		Expect(first.ReturnedRevenueCents).To(Equal(int64(2500)))
		Expect(first.NetRevenueCents).To(Equal(int64(7500)))
		Expect(first.COGSCents).To(Equal(int64(4000)))
		Expect(first.ReversedCostCents).To(Equal(int64(1000)))
		Expect(first.NetCOGSCents).To(Equal(int64(3000)))
		Expect(first.GrossCents).To(Equal(int64(4500)))

		Expect(result.Customers).To(HaveLen(2))
		Expect(result.Customers[0].Customer).To(Equal("C1"))
		Expect(result.Customers[0].NetRevenueCents).To(Equal(int64(13500)))
		Expect(result.Customers[0].GrossCents).To(Equal(int64(6500)))

		Expect(result.Products).To(HaveLen(2))
		Expect(result.Products[0].SKU).To(Equal("A"))
		Expect(result.Products[0].NetRevenueCents).To(Equal(int64(10500)))
		Expect(result.Products[0].GrossCents).To(Equal(int64(6500)))

		Expect(result.NetRevenueCents).To(Equal(int64(16500)))
		Expect(result.COGSCents).To(Equal(int64(8000)))
		Expect(result.GrossCents).To(Equal(int64(8500)))
	})

	It("is idempotent for an unchanged ledger read twice", func() {
		rows := []stockledger.Attributes{
			receiptRow("R1", 1, "WH1", "A", "LOT1", 5, "50.00"),
			saleRow("S1", 2, "WH1", "A", "C1", 2, "40.00"),
			returnRow("T1", 3, "WH1", "A", "S1", 1, "20.00"),
		}
		first, err := compute("v1", rows...)
		Expect(err).ToNot(HaveOccurred())
		second, err := compute("v1", append(append([]stockledger.Attributes{}, rows...), rows...)...)
		Expect(err).ToNot(HaveOccurred())
		Expect(second.Lines).To(Equal(first.Lines))
		Expect(second.Allocations).To(Equal(first.Allocations))
		Expect(second.Batches).To(Equal(first.Batches))
		Expect(second.NetRevenueCents).To(Equal(first.NetRevenueCents))
		Expect(second.COGSCents).To(Equal(first.COGSCents))
	})

	It("fails on a sale that exceeds available stock", func() {
		_, err := compute("v1",
			receiptRow("R1", 1, "WH1", "A", "LOT1", 3, "30.00"),
			saleRow("S1", 2, "WH1", "A", "C1", 4, "80.00"),
		)
		expectCode(err, stockledger.ErrInsufficientStock)
	})

	It("fails when stock is only available in a different warehouse", func() {
		_, err := compute("v1",
			receiptRow("R1", 1, "WH1", "A", "LOT1", 10, "100.00"),
			saleRow("S1", 2, "WH2", "A", "C1", 1, "10.00"),
		)
		expectCode(err, stockledger.ErrInsufficientStock)
	})

	It("fails on a return larger than the sale", func() {
		_, err := compute("v1",
			receiptRow("R1", 1, "WH1", "A", "LOT1", 10, "100.00"),
			saleRow("S1", 2, "WH1", "A", "C1", 2, "40.00"),
			returnRow("T1", 3, "WH1", "A", "S1", 3, "60.00"),
		)
		expectCode(err, stockledger.ErrExcessReturn)
	})

	It("fails when returns together exceed the original sale", func() {
		_, err := compute("v1",
			receiptRow("R1", 1, "WH1", "A", "LOT1", 10, "100.00"),
			saleRow("S1", 2, "WH1", "A", "C1", 4, "40.00"),
			returnRow("T1", 3, "WH1", "A", "S1", 3, "30.00"),
			returnRow("T2", 4, "WH1", "A", "S1", 2, "20.00"),
		)
		expectCode(err, stockledger.ErrExcessReturn)
	})

	It("fails when a return's revenue would exceed the sale's revenue", func() {
		_, err := compute("v1",
			receiptRow("R1", 1, "WH1", "A", "LOT1", 10, "100.00"),
			saleRow("S1", 2, "WH1", "A", "C1", 2, "40.00"),
			returnRow("T1", 3, "WH1", "A", "S1", 2, "45.00"),
		)
		expectCode(err, stockledger.ErrExcessReturn)
	})

	It("accepts a full return of a sale", func() {
		result, err := compute("v1",
			receiptRow("R1", 1, "WH1", "A", "LOT1", 10, "100.00"),
			saleRow("S1", 2, "WH1", "A", "C1", 4, "40.00"),
			returnRow("T1", 3, "WH1", "A", "S1", 4, "40.00"),
		)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.NetRevenueCents).To(Equal(int64(0)))
		Expect(result.COGSCents).To(Equal(int64(0)))
		Expect(result.GrossCents).To(Equal(int64(0)))
		Expect(batchByName(result, "LOT1").RemainingQuantity).To(Equal(int64(10)))
		Expect(result.CustomerProducts).To(HaveLen(1))
		Expect(result.CustomerProducts[0].NetQuantity).To(Equal(int64(0)))
	})

	It("fails when a return references an unknown sale", func() {
		_, err := compute("v1",
			receiptRow("R1", 1, "WH1", "A", "LOT1", 10, "100.00"),
			returnRow("T1", 2, "WH1", "A", "S9", 1, "10.00"),
		)
		expectCode(err, stockledger.ErrUnknownReference)
	})

	It("fails when a return references a receipt instead of a sale", func() {
		_, err := compute("v1",
			receiptRow("R1", 1, "WH1", "A", "LOT1", 10, "100.00"),
			returnRow("T1", 2, "WH1", "A", "R1", 1, "10.00"),
		)
		expectCode(err, stockledger.ErrReferenceNotSale)
	})

	It("fails when a return precedes its original sale", func() {
		_, err := compute("v1",
			receiptRow("R1", 1, "WH1", "A", "LOT1", 10, "100.00"),
			returnRow("T1", 2, "WH1", "A", "S1", 1, "10.00"),
			saleRow("S1", 3, "WH1", "A", "C1", 2, "20.00"),
		)
		expectCode(err, stockledger.ErrOutOfOrderReference)
	})

	It("fails when a return crosses warehouses", func() {
		_, err := compute("v1",
			receiptRow("R1", 1, "WH1", "A", "LOT1", 10, "100.00"),
			saleRow("S1", 2, "WH1", "A", "C1", 2, "20.00"),
			returnRow("T1", 3, "WH2", "A", "S1", 1, "10.00"),
		)
		expectCode(err, stockledger.ErrWarehouseMismatch)
	})

	It("fails when a return crosses products", func() {
		_, err := compute("v1",
			receiptRow("R1", 1, "WH1", "A", "LOT1", 10, "100.00"),
			receiptRow("R2", 2, "WH1", "B", "LOT2", 10, "100.00"),
			saleRow("S1", 3, "WH1", "A", "C1", 2, "20.00"),
			returnRow("T1", 4, "WH1", "B", "S1", 1, "10.00"),
		)
		expectCode(err, stockledger.ErrSKUMismatch)
	})

	It("fails when a return names a different customer than the sale", func() {
		_, err := compute("v1",
			receiptRow("R1", 1, "WH1", "A", "LOT1", 10, "100.00"),
			saleRow("S1", 2, "WH1", "A", "C1", 2, "20.00"),
			row(map[string]string{
				"data-type": "return", "data-id": "T1", "data-seq": "3", "data-warehouse": "WH1",
				"data-sku": "A", "data-ref": "S1", "data-customer": "C2", "data-qty": "1", "data-amount": "10.00",
			}),
		)
		expectCode(err, stockledger.ErrCustomerMismatch)
	})

	It("fails when two receipts reuse a batch id", func() {
		_, err := compute("v1",
			receiptRow("R1", 1, "WH1", "A", "LOT1", 5, "50.00"),
			receiptRow("R2", 2, "WH1", "A", "LOT1", 5, "50.00"),
		)
		expectCode(err, stockledger.ErrDuplicateBatch)
	})

	It("orders events by sequence even when the DOM order is shuffled", func() {
		result, err := compute("v1",
			returnRow("T1", 3, "WH1", "A", "S1", 1, "20.00"),
			saleRow("S1", 2, "WH1", "A", "C1", 2, "40.00"),
			receiptRow("R1", 1, "WH1", "A", "LOT1", 5, "50.00"),
		)
		Expect(err).ToNot(HaveOccurred())
		Expect(allocationsFor(result, "S1")).To(Equal([]string{"LOT1:2:20.00"}))
		Expect(allocationsFor(result, "T1")).To(Equal([]string{"LOT1:1:10.00"}))
	})

	It("derives a return's customer from the original sale when omitted", func() {
		result, err := compute("v1",
			receiptRow("R1", 1, "WH1", "A", "LOT1", 5, "50.00"),
			saleRow("S1", 2, "WH1", "A", "C1", 2, "40.00"),
			returnRow("T1", 3, "WH1", "A", "S1", 1, "20.00"),
		)
		Expect(err).ToNot(HaveOccurred())
		Expect(lineByID(result, "T1").Customer).To(Equal("C1"))
		Expect(result.Customers).To(HaveLen(1))
	})

	It("reports the remaining valuation of every batch, including untouched ones", func() {
		result, err := compute("v1",
			receiptRow("R1", 1, "WH1", "A", "LOT1", 5, "50.00"),
			receiptRow("R2", 2, "WH1", "B", "LOT2", 5, "75.00"),
			saleRow("S1", 3, "WH1", "A", "C1", 2, "40.00"),
		)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.Batches).To(HaveLen(2))
		Expect(result.Batches[0].ReceiptQuantity).To(Equal(int64(5)))
		Expect(result.Batches[0].ConsumedQuantity).To(Equal(int64(2)))
		Expect(result.Batches[0].RemainingQuantity).To(Equal(int64(3)))
		Expect(result.Batches[0].RemainingCostCents).To(Equal(int64(3000)))
		Expect(result.Batches[1].ConsumedQuantity).To(Equal(int64(0)))
		Expect(result.Batches[1].RemainingCostCents).To(Equal(int64(7500)))
	})

	It("fails rather than overflowing when the ledger totals are unrepresentable", func() {
		// Each amount is the largest single amount the parser accepts, so two
		// of them cannot be summed.
		_, err := compute("v1",
			receiptRow("R1", 1, "WH1", "A", "LOT1", 1, "11258999068426.24"),
			receiptRow("R2", 2, "WH1", "B", "LOT2", 1, "11258999068426.24"),
		)
		expectCode(err, stockledger.ErrOverflow)
	})

	It("never loses or invents a cent when one batch is drawn down in many slices", func() {
		// 10 units costing 100.00: each unit is worth 10.00, so ten separate
		// one-unit sales must sum back to exactly the receipt cost.
		rows := []stockledger.Attributes{receiptRow("R1", 1, "WH1", "A", "LOT1", 10, "100.00")}
		for index := 1; index <= 10; index++ {
			rows = append(rows, saleRow("S"+itoa(index), index+1, "WH1", "A", "C1", 1, "10.00"))
		}
		result, err := compute("v1", rows...)
		Expect(err).ToNot(HaveOccurred())

		total := int64(0)
		for _, line := range result.Lines {
			total += line.CostCents
		}
		Expect(total).To(Equal(int64(10000)))
		Expect(batchByName(result, "LOT1").RemainingQuantity).To(Equal(int64(0)))
		Expect(batchByName(result, "LOT1").RemainingCostCents).To(Equal(int64(0)))
	})
})
