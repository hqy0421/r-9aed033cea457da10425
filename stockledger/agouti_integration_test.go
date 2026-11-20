package stockledger_test

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sclevine/agouti"
	"github.com/sclevine/agouti/stockledger"
)

// ledgerHTML is served from a local file and read through PhantomJS by the
// Agouti adapter, exercising the real DOM path end to end. The "logged-in"
// marker stands in for an authenticated session: the reader never logs in and
// must work purely from what the page shows.
const ledgerHTML = `<!doctype html>
<html><head><title>Inventory ledger</title></head>
<body>
  <div id="nav"><a href="#orders">Orders</a></div>
  <section id="stock-ledger" data-version="2026-11-20.1" data-status="completed">
    <table>
      <thead><tr><th>seq</th><th>type</th></tr></thead>
      <tbody>
        <tr data-type="receipt" data-id="R-100" data-seq="1"
            data-warehouse="WH-NORTH" data-sku="SKU-GADGET"
            data-product="Widget" data-batch="LOT-A"
            data-qty="12" data-amount="120.00"></tr>
        <tr data-type="receipt" data-id="R-101" data-seq="2"
            data-warehouse="WH-NORTH" data-sku="SKU-GADGET"
            data-product="Widget" data-batch="LOT-B"
            data-qty="6" data-amount="90.00"></tr>
        <tr data-type="sale" data-id="S-200" data-seq="3"
            data-warehouse="WH-NORTH" data-sku="SKU-GADGET"
            data-product="Widget" data-customer="ACME" data-qty="15" data-amount="337.50"></tr>
        <tr data-type="return" data-id="T-300" data-seq="4"
            data-warehouse="WH-NORTH" data-sku="SKU-GADGET"
            data-product="Widget" data-customer="ACME"
            data-ref="S-200" data-qty="4" data-amount="90.00"></tr>
        <tr data-type="receipt" data-id="R-102" data-seq="5"
            data-warehouse="WH-SOUTH" data-sku="SKU-FREEBIE"
            data-product="Sample" data-batch="LOT-F"
            data-qty="10" data-amount="0.00"></tr>
        <tr data-type="sale" data-id="S-201" data-seq="6"
            data-warehouse="WH-SOUTH" data-sku="SKU-FREEBIE"
            data-product="Sample" data-customer="BETA" data-qty="3" data-amount="15.00"></tr>
      </tbody>
    </table>
  </section>
</body></html>`

var _ = Describe("Reading the ledger through Agouti", func() {
	var (
		driver  *agouti.WebDriver
		page    *agouti.Page
		tempDir string
	)

	BeforeEach(func() {
		var err error
		tempDir, err = ioutil.TempDir("", "stockledger-agouti")
		Expect(err).ToNot(HaveOccurred())

		driver = agouti.PhantomJS()
		Expect(driver.Start()).To(Succeed())
		page, err = driver.NewPage()
		Expect(err).ToNot(HaveOccurred())

		ledgerFile := filepath.Join(tempDir, "ledger.html")
		Expect(ioutil.WriteFile(ledgerFile, []byte(ledgerHTML), 0644)).To(Succeed())
		Expect(page.Navigate("file://" + ledgerFile)).To(Succeed())
	})

	AfterEach(func() {
		Expect(driver.Stop()).To(Succeed())
		os.RemoveAll(tempDir)
	})

	It("reads the ledger from the live DOM and costs it", func() {
		ledger, err := stockledger.ReadLedger(stockledger.AgoutiPage(page), stockledger.Options{})
		Expect(err).ToNot(HaveOccurred())
		Expect(ledger.Version).To(Equal("2026-11-20.1"))
		Expect(ledger.Rows).To(HaveLen(6))

		result, err := ledger.Compute()
		Expect(err).ToNot(HaveOccurred())

		// FIFO: LOT-A (12 @ 10.00), then 3 of LOT-B (15.00 each).
		Expect(allocationsFor(result, "S-200")).To(Equal([]string{"LOT-A:12:120.00", "LOT-B:3:45.00"}))
		// The return reverses the last allocation first, then restores LOT-A.
		Expect(allocationsFor(result, "T-300")).To(Equal([]string{"LOT-B:3:45.00", "LOT-A:1:10.00"}))

		Expect(result.NetRevenueCents).To(Equal(int64(33750 - 9000 + 1500)))
		Expect(result.COGSCents).To(Equal(int64(16500 - 5500)))
		Expect(result.GrossCents).To(Equal(int64(17250 - 3500 + 1500)))

		Expect(batchByName(result, "LOT-A").RemainingQuantity).To(Equal(int64(1)))
		Expect(batchByName(result, "LOT-B").RemainingQuantity).To(Equal(int64(6)))
		Expect(batchByName(result, "LOT-F").RemainingQuantity).To(Equal(int64(7)))

		// The zero-cost batch survives with a zero cost and a real quantity.
		freebie := batchByName(result, "LOT-F")
		Expect(freebie.ReceiptCostCents).To(Equal(int64(0)))
		Expect(freebie.RemainingCostCents).To(Equal(int64(0)))
		Expect(freebie.ConsumedQuantity).To(Equal(int64(3)))
		Expect(lineByID(result, "S-201").CostCents).To(Equal(int64(0)))
		Expect(lineByID(result, "S-201").GrossCents).To(Equal(int64(1500)))

		Expect(result.CustomerProducts).To(HaveLen(2))
		Expect(result.CustomerProducts[0].Customer).To(Equal("ACME"))
		Expect(result.CustomerProducts[0].NetRevenueCents).To(Equal(int64(24750)))
		Expect(result.CustomerProducts[0].NetCOGSCents).To(Equal(int64(11000)))
		Expect(result.CustomerProducts[0].GrossCents).To(Equal(int64(13750)))
		Expect(result.CustomerProducts[1].Customer).To(Equal("BETA"))
		Expect(result.CustomerProducts[1].GrossCents).To(Equal(int64(1500)))
	})

	It("writes the JSON and CSV reports to disk, keeping zero rows", func() {
		ledger, err := stockledger.ReadLedger(stockledger.AgoutiPage(page), stockledger.Options{})
		Expect(err).ToNot(HaveOccurred())
		result, err := ledger.Compute()
		Expect(err).ToNot(HaveOccurred())

		jsonPath := filepath.Join(tempDir, "out", "report.json")
		Expect(stockledger.WriteReport(result, jsonPath)).To(Succeed())
		written, err := ioutil.ReadFile(jsonPath)
		Expect(err).ToNot(HaveOccurred())

		var document struct {
			Version string `json:"version"`
			Totals  struct {
				NetRevenue struct {
					Cents int64 `json:"cents"`
				} `json:"net_revenue"`
				CostOfGoodsSold struct {
					Cents int64 `json:"cents"`
				} `json:"cost_of_goods_sold"`
				GrossProfit struct {
					Cents int64 `json:"cents"`
				} `json:"gross_profit"`
			} `json:"totals"`
			Allocations      []map[string]interface{} `json:"allocations"`
			Batches          []map[string]interface{} `json:"batches"`
			CustomerProducts []map[string]interface{} `json:"customer_products"`
		}
		Expect(json.Unmarshal(written, &document)).To(Succeed())
		Expect(document.Version).To(Equal("2026-11-20.1"))
		Expect(document.Totals.NetRevenue.Cents).To(Equal(result.NetRevenueCents))
		Expect(document.Totals.CostOfGoodsSold.Cents).To(Equal(result.COGSCents))
		Expect(document.Totals.GrossProfit.Cents).To(Equal(result.GrossCents))
		Expect(document.Allocations).To(HaveLen(5))
		Expect(document.Batches).To(HaveLen(3))
		Expect(document.CustomerProducts).To(HaveLen(2))

		csvDir := filepath.Join(tempDir, "out", "csv")
		Expect(stockledger.WriteCSV(result, csvDir)).To(Succeed())
		for _, name := range []string{"lines.csv", "allocations.csv", "batches.csv", "customer_products.csv", "customers.csv", "products.csv"} {
			contents, err := ioutil.ReadFile(filepath.Join(csvDir, name))
			Expect(err).ToNot(HaveOccurred(), name)
			Expect(string(contents)).ToNot(BeEmpty(), name)
		}
	})

	It("is idempotent when the same live page is re-read", func() {
		first, err := stockledger.ReadLedger(stockledger.AgoutiPage(page), stockledger.Options{})
		Expect(err).ToNot(HaveOccurred())
		second, err := stockledger.ReadLedger(stockledger.AgoutiPage(page), stockledger.Options{})
		Expect(err).ToNot(HaveOccurred())

		firstResult, err := first.Compute()
		Expect(err).ToNot(HaveOccurred())
		secondResult, err := second.Compute()
		Expect(err).ToNot(HaveOccurred())

		Expect(secondResult.Allocations).To(Equal(firstResult.Allocations))
		Expect(secondResult.Batches).To(Equal(firstResult.Batches))
		Expect(secondResult.NetRevenueCents).To(Equal(firstResult.NetRevenueCents))
		Expect(secondResult.COGSCents).To(Equal(firstResult.COGSCents))
		Expect(secondResult.GrossCents).To(Equal(firstResult.GrossCents))
	})
})
