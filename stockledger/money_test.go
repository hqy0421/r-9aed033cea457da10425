package stockledger_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sclevine/agouti/stockledger"
)

var _ = Describe("Money", func() {
	Describe("ParseCents", func() {
		It("parses whole amounts as cents", func() {
			cents, err := stockledger.ParseCents("12")
			Expect(err).ToNot(HaveOccurred())
			Expect(cents).To(Equal(int64(1200)))
		})

		It("parses two decimal places as cents", func() {
			cents, err := stockledger.ParseCents("12.34")
			Expect(err).ToNot(HaveOccurred())
			Expect(cents).To(Equal(int64(1234)))
		})

		It("pads a single decimal place", func() {
			cents, err := stockledger.ParseCents("0.5")
			Expect(err).ToNot(HaveOccurred())
			Expect(cents).To(Equal(int64(50)))
		})

		It("parses a leading decimal point", func() {
			cents, err := stockledger.ParseCents(".07")
			Expect(err).ToNot(HaveOccurred())
			Expect(cents).To(Equal(int64(7)))
		})

		It("accepts thousands separators", func() {
			cents, err := stockledger.ParseCents("1,234.56")
			Expect(err).ToNot(HaveOccurred())
			Expect(cents).To(Equal(int64(123456)))
		})

		It("accepts a leading plus sign", func() {
			cents, err := stockledger.ParseCents("+3.10")
			Expect(err).ToNot(HaveOccurred())
			Expect(cents).To(Equal(int64(310)))
		})

		It("parses zero", func() {
			cents, err := stockledger.ParseCents("0.00")
			Expect(err).ToNot(HaveOccurred())
			Expect(cents).To(Equal(int64(0)))
		})

		It("rejects a third decimal place rather than rounding", func() {
			_, err := stockledger.ParseCents("1.005")
			Expect(err).To(HaveOccurred())
		})

		It("rejects negative amounts", func() {
			_, err := stockledger.ParseCents("-1.00")
			Expect(err).To(HaveOccurred())
		})

		It("rejects empty amounts", func() {
			_, err := stockledger.ParseCents("  ")
			Expect(err).To(HaveOccurred())
		})

		It("rejects exponents, symbols, and spaces", func() {
			for _, value := range []string{"1e3", "$1.00", "1 000", "1.2.3", "abc", "."} {
				_, err := stockledger.ParseCents(value)
				Expect(err).To(HaveOccurred(), value)
			}
		})

		It("rejects amounts beyond the supported range", func() {
			_, err := stockledger.ParseCents("99999999999999999999")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("ParseQuantity", func() {
		It("parses whole quantities", func() {
			quantity, err := stockledger.ParseQuantity("17")
			Expect(err).ToNot(HaveOccurred())
			Expect(quantity).To(Equal(int64(17)))
		})

		It("accepts thousands separators", func() {
			quantity, err := stockledger.ParseQuantity("1,200")
			Expect(err).ToNot(HaveOccurred())
			Expect(quantity).To(Equal(int64(1200)))
		})

		It("rejects fractional quantities", func() {
			_, err := stockledger.ParseQuantity("1.5")
			Expect(err).To(HaveOccurred())
		})

		It("rejects negative quantities", func() {
			_, err := stockledger.ParseQuantity("-2")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("ParseSequence", func() {
		It("parses positive integers", func() {
			sequence, err := stockledger.ParseSequence("42")
			Expect(err).ToNot(HaveOccurred())
			Expect(sequence).To(Equal(int64(42)))
		})

		It("rejects zero", func() {
			_, err := stockledger.ParseSequence("0")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("ParseCentCount", func() {
		It("reads a value that is already in cents without scaling", func() {
			cents, err := stockledger.ParseCentCount("1230")
			Expect(err).ToNot(HaveOccurred())
			Expect(cents).To(Equal(int64(1230)))
		})

		It("rejects a decimal because cents are whole", func() {
			_, err := stockledger.ParseCentCount("12.30")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("FormatCents", func() {
		It("renders cents with two decimal places", func() {
			Expect(stockledger.FormatCents(0)).To(Equal("0.00"))
			Expect(stockledger.FormatCents(7)).To(Equal("0.07"))
			Expect(stockledger.FormatCents(123456)).To(Equal("1234.56"))
		})

		It("renders negative cents with a leading sign", func() {
			Expect(stockledger.FormatCents(-1234)).To(Equal("-12.34"))
		})
	})
})
