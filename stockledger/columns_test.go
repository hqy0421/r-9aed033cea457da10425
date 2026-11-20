package stockledger_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sclevine/agouti/stockledger"
)

var _ = Describe("Column and label normalisation", func() {
	Describe("NormalizeEventType", func() {
		events := []struct {
			label string
			want  stockledger.EventType
		}{
			{"receipt", stockledger.EventReceipt},
			{"Receipt", stockledger.EventReceipt},
			{"  inbound ", stockledger.EventReceipt},
			{"收货", stockledger.EventReceipt},
			{"采购入库", stockledger.EventReceipt},
			{"sale", stockledger.EventSale},
			{"Sales", stockledger.EventSale},
			{"销售出库", stockledger.EventSale},
			{"return", stockledger.EventReturn},
			{"RMA", stockledger.EventReturn},
			{"销售退货", stockledger.EventReturn},
			{"退 回", stockledger.EventReturn},
		}
		for _, event := range events {
			event := event
			It("maps "+event.label, func() {
				got, ok := stockledger.NormalizeEventType(event.label)
				Expect(ok).To(BeTrue())
				Expect(got).To(Equal(event.want))
			})
		}

		It("rejects an unrelated type", func() {
			_, ok := stockledger.NormalizeEventType("adjustment")
			Expect(ok).To(BeFalse())
		})
	})
})
