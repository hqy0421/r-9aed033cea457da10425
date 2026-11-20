package stockledger_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sclevine/agouti/stockledger"
)

// row builds a raw DOM attribute map for a ledger row. Empty values are
// omitted so "missing attribute" behaviour is exercised as it would be in the
// browser.
func row(attributes map[string]string) stockledger.Attributes {
	fields := map[string]stockledger.Field{}
	present := map[string]bool{}
	for name, value := range attributes {
		if value == "" {
			continue
		}
		fields[name] = stockledger.Field{Name: name, Value: value}
		present[name] = true
	}
	return stockledger.Attributes{Fields: fields, Present: present}
}

func parse(version string, rows ...stockledger.Attributes) (*stockledger.Ledger, error) {
	return stockledger.ParseRows(version, rows)
}

func expectCode(err error, code string) {
	ExpectWithOffset(1, err).To(HaveOccurred())
	ledgerError, ok := err.(*stockledger.Error)
	ExpectWithOffset(1, ok).To(BeTrue(), "expected *stockledger.Error, got %T", err)
	ExpectWithOffset(1, ledgerError.Code).To(Equal(code), ledgerError.Error())
}

var _ = Describe("Ledger parsing", func() {
	It("parses a completed ledger version in sequence order", func() {
		ledger, err := parse("v1",
			row(map[string]string{"data-type": "sale", "data-id": "S1", "data-seq": "2", "data-warehouse": "WH1", "data-sku": "A", "data-customer": "C1", "data-qty": "2", "data-amount": "20.00"}),
			row(map[string]string{"data-type": "receipt", "data-id": "R1", "data-seq": "1", "data-warehouse": "WH1", "data-sku": "A", "data-batch": "B1", "data-qty": "5", "data-amount": "50.00"}),
		)
		Expect(err).ToNot(HaveOccurred())
		Expect(ledger.Version).To(Equal("v1"))
		Expect(ledger.Rows).To(HaveLen(2))
		Expect(ledger.Rows[0].ID).To(Equal("R1"))
		Expect(ledger.Rows[1].ID).To(Equal("S1"))
		Expect(ledger.Rows[0].Type).To(Equal(stockledger.EventReceipt))
		Expect(ledger.Rows[0].Batch).To(Equal("B1"))
		Expect(ledger.Rows[0].Cents).To(Equal(int64(5000)))
	})

	It("accepts the documented attribute aliases", func() {
		ledger, err := parse("v1", row(map[string]string{
			"data-type": "receipt", "data-id": "R1", "data-sequence": "1",
			"data-warehouse": "WH1", "data-sku": "A", "data-product-name": "Widget",
			"data-batch-id": "LOT", "data-quantity": "3", "data-cents": "930",
		}))
		Expect(err).ToNot(HaveOccurred())
		Expect(ledger.Rows[0].Product).To(Equal("Widget"))
		Expect(ledger.Rows[0].Batch).To(Equal("LOT"))
		Expect(ledger.Rows[0].Quantity).To(Equal(int64(3)))
		Expect(ledger.Rows[0].Cents).To(Equal(int64(930)))
	})

	It("agrees when amount and amount-cents describe the same value", func() {
		ledger, err := parse("v1", row(map[string]string{
			"data-type": "receipt", "data-id": "R1", "data-seq": "1",
			"data-warehouse": "WH1", "data-sku": "A", "data-batch": "B1",
			"data-qty": "1", "data-amount": "12.30", "data-amount-cents": "1230",
		}))
		Expect(err).ToNot(HaveOccurred())
		Expect(ledger.Rows[0].Cents).To(Equal(int64(1230)))
	})

	It("fails when amount and amount-cents disagree", func() {
		_, err := parse("v1", row(map[string]string{
			"data-type": "receipt", "data-id": "R1", "data-seq": "1",
			"data-warehouse": "WH1", "data-sku": "A", "data-batch": "B1",
			"data-qty": "1", "data-amount": "12.30", "data-amount-cents": "1231",
		}))
		expectCode(err, stockledger.ErrConflictingAmount)
	})

	It("fails when the page reports no ledger version", func() {
		_, err := parse("", row(map[string]string{
			"data-type": "receipt", "data-id": "R1", "data-seq": "1",
			"data-warehouse": "WH1", "data-sku": "A", "data-batch": "B1",
			"data-qty": "1", "data-amount": "1.00",
		}))
		expectCode(err, stockledger.ErrMissingVersion)
	})

	It("fails when a row belongs to a different ledger version", func() {
		_, err := parse("v1", row(map[string]string{
			"data-type": "receipt", "data-id": "R1", "data-seq": "1",
			"data-warehouse": "WH1", "data-sku": "A", "data-batch": "B1",
			"data-qty": "1", "data-amount": "1.00", "data-version": "v2",
		}))
		expectCode(err, stockledger.ErrMixedVersion)
	})

	It("fails on an unsupported event type", func() {
		_, err := parse("v1", row(map[string]string{
			"data-type": "adjustment", "data-id": "X1", "data-seq": "1",
			"data-warehouse": "WH1", "data-sku": "A", "data-qty": "1", "data-amount": "1.00",
		}))
		expectCode(err, stockledger.ErrUnknownEventType)
	})

	It("fails when a required field is missing", func() {
		_, err := parse("v1", row(map[string]string{
			"data-type": "receipt", "data-id": "R1", "data-seq": "1",
			"data-warehouse": "WH1", "data-sku": "A", "data-qty": "1", "data-amount": "1.00",
		}))
		expectCode(err, stockledger.ErrMissingField)
	})

	It("fails on malformed numbers", func() {
		_, err := parse("v1", row(map[string]string{
			"data-type": "receipt", "data-id": "R1", "data-seq": "1",
			"data-warehouse": "WH1", "data-sku": "A", "data-batch": "B1",
			"data-qty": "1.5", "data-amount": "1.00",
		}))
		expectCode(err, stockledger.ErrInvalidNumber)
	})

	It("fails on a zero-quantity row", func() {
		_, err := parse("v1", row(map[string]string{
			"data-type": "receipt", "data-id": "R1", "data-seq": "1",
			"data-warehouse": "WH1", "data-sku": "A", "data-batch": "B1",
			"data-qty": "0", "data-amount": "0.00",
		}))
		expectCode(err, stockledger.ErrInvalidNumber)
	})

	It("fails on a duplicate sequence number", func() {
		_, err := parse("v1",
			row(map[string]string{"data-type": "receipt", "data-id": "R1", "data-seq": "1", "data-warehouse": "WH1", "data-sku": "A", "data-batch": "B1", "data-qty": "1", "data-amount": "1.00"}),
			row(map[string]string{"data-type": "receipt", "data-id": "R2", "data-seq": "1", "data-warehouse": "WH1", "data-sku": "A", "data-batch": "B2", "data-qty": "1", "data-amount": "1.00"}),
		)
		expectCode(err, stockledger.ErrDuplicateSequence)
	})

	It("does not double count an identical row read twice", func() {
		receipt := map[string]string{"data-type": "receipt", "data-id": "R1", "data-seq": "1", "data-warehouse": "WH1", "data-sku": "A", "data-batch": "B1", "data-qty": "5", "data-amount": "50.00"}
		ledger, err := parse("v1", row(receipt), row(receipt))
		Expect(err).ToNot(HaveOccurred())
		Expect(ledger.Rows).To(HaveLen(1))
	})

	It("fails when the same event id is read with different values", func() {
		_, err := parse("v1",
			row(map[string]string{"data-type": "receipt", "data-id": "R1", "data-seq": "1", "data-warehouse": "WH1", "data-sku": "A", "data-batch": "B1", "data-qty": "5", "data-amount": "50.00"}),
			row(map[string]string{"data-type": "receipt", "data-id": "R1", "data-seq": "1", "data-warehouse": "WH1", "data-sku": "A", "data-batch": "B1", "data-qty": "6", "data-amount": "50.00"}),
		)
		expectCode(err, stockledger.ErrConflictingEvent)
	})

	It("fails when a sale carries a batch", func() {
		_, err := parse("v1", row(map[string]string{
			"data-type": "sale", "data-id": "S1", "data-seq": "1", "data-warehouse": "WH1",
			"data-sku": "A", "data-customer": "C1", "data-batch": "B1", "data-qty": "1", "data-amount": "2.00",
		}))
		expectCode(err, stockledger.ErrUnexpectedBatch)
	})

	It("fails when a receipt references a sale", func() {
		_, err := parse("v1", row(map[string]string{
			"data-type": "receipt", "data-id": "R1", "data-seq": "1", "data-warehouse": "WH1",
			"data-sku": "A", "data-batch": "B1", "data-ref": "S1", "data-qty": "1", "data-amount": "1.00",
		}))
		expectCode(err, stockledger.ErrUnexpectedReference)
	})

	It("fails when a return has no original sale reference", func() {
		_, err := parse("v1", row(map[string]string{
			"data-type": "return", "data-id": "T1", "data-seq": "1", "data-warehouse": "WH1",
			"data-sku": "A", "data-qty": "1", "data-amount": "1.00",
		}))
		expectCode(err, stockledger.ErrMissingReference)
	})

	It("accepts rows in any DOM order because the sequence orders them", func() {
		ledger, err := parse("v1",
			row(map[string]string{"data-type": "return", "data-id": "T1", "data-seq": "3", "data-warehouse": "WH1", "data-sku": "A", "data-ref": "S1", "data-qty": "1", "data-amount": "2.00"}),
			row(map[string]string{"data-type": "sale", "data-id": "S1", "data-seq": "2", "data-warehouse": "WH1", "data-sku": "A", "data-customer": "C1", "data-qty": "2", "data-amount": "4.00"}),
			row(map[string]string{"data-type": "receipt", "data-id": "R1", "data-seq": "1", "data-warehouse": "WH1", "data-sku": "A", "data-batch": "B1", "data-qty": "5", "data-amount": "5.00"}),
		)
		Expect(err).ToNot(HaveOccurred())
		var ids []string
		for _, parsedRow := range ledger.Rows {
			ids = append(ids, parsedRow.ID)
		}
		Expect(ids).To(Equal([]string{"R1", "S1", "T1"}))
	})
})
