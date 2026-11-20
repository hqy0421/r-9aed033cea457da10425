// Command stockledger reads a completed inventory ledger from an
// already-authenticated Agouti page and writes per-line FIFO cost accounting.
//
// It never logs in, clicks, submits a form, navigates (unless -nav is given),
// or runs page scripts, so the remote inventory is not modified.
//
// Examples:
//
//	# Attach to a WebDriver session that is already logged in and on the ledger.
//	stockledger -session http://127.0.0.1:4444/session/abc123 \
//	    -json out/report.json -csv out/csv
//
//	# Open a fresh page, navigate to an internal ledger view, then read it.
//	stockledger -driver http://127.0.0.1:4444 -nav http://erp.internal/ledger \
//	    -json report.json
//
//	# Ledgers that use one table per event type:
//	stockledger -driver http://127.0.0.1:4444 -root '#ledger' \
//	    -receipts '#receipts tbody' -sales '#sales tbody' -returns '#returns tbody' \
//	    -json report.json
//
//	# Ordinary HTML tables whose columns are named by their <th> headers:
//	stockledger -driver http://127.0.0.1:4444 -root '#ledger' \
//	    -receipts-table '#receipts table' -sales-table '#sales table' \
//	    -returns-table '#returns table' -json report.json
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/sclevine/agouti"
	"github.com/sclevine/agouti/stockledger"
)

type options struct {
	sessionURL  string
	driverURL   string
	navigate    string
	root        string
	receipts    string
	sales       string
	returns     string
	receiptsTbl string
	salesTbl    string
	returnsTbl  string
	jsonPath    string
	csvDir      string
}

func main() {
	opts := options{}
	flag.StringVar(&opts.sessionURL, "session", "",
		"WebDriver session URL to attach to, e.g. http://127.0.0.1:4444/session/<id>")
	flag.StringVar(&opts.driverURL, "driver", "",
		"WebDriver URL to open a new page from, e.g. http://127.0.0.1:4444")
	flag.StringVar(&opts.navigate, "nav", "",
		"optional ledger URL to navigate to after opening or joining the page")
	flag.StringVar(&opts.root, "root", stockledger.DefaultRootSelector,
		"CSS selector for the ledger root element carrying data-version/data-status")
	flag.StringVar(&opts.receipts, "receipts", "", "CSS selector for the receipts section (optional)")
	flag.StringVar(&opts.sales, "sales", "", "CSS selector for the sales section (optional)")
	flag.StringVar(&opts.returns, "returns", "", "CSS selector for the returns section (optional)")
	flag.StringVar(&opts.receiptsTbl, "receipts-table", "",
		"CSS selector for an ordinary HTML receipts table read by its <th> headers (optional)")
	flag.StringVar(&opts.salesTbl, "sales-table", "",
		"CSS selector for an ordinary HTML sales table read by its <th> headers (optional)")
	flag.StringVar(&opts.returnsTbl, "returns-table", "",
		"CSS selector for an ordinary HTML returns table read by its <th> headers (optional)")
	flag.StringVar(&opts.jsonPath, "json", "", "path of the JSON report to write (required)")
	flag.StringVar(&opts.csvDir, "csv", "", "directory for CSV reports (optional)")
	flag.Parse()

	if err := run(opts); err != nil {
		if ledgerError, ok := err.(*stockledger.Error); ok {
			fmt.Fprintf(os.Stderr, "stockledger failed [%s]: %s\n", ledgerError.Code, ledgerError.Error())
		} else {
			fmt.Fprintf(os.Stderr, "stockledger failed: %s\n", err)
		}
		os.Exit(1)
	}
}

func run(opts options) error {
	if opts.jsonPath == "" {
		return fmt.Errorf("-json is required: the report must be written to disk")
	}
	if (opts.sessionURL == "") == (opts.driverURL == "") {
		return fmt.Errorf("provide exactly one of -session (attach to a logged-in session) or -driver (open a new page)")
	}

	var (
		page *agouti.Page
		err  error
	)
	if opts.sessionURL != "" {
		// Attaching to an existing session must not close it on exit: the
		// session belongs to the user's logged-in browser, not to this tool.
		page = agouti.JoinPage(opts.sessionURL)
	} else {
		page, err = agouti.NewPage(opts.driverURL)
		if err != nil {
			return fmt.Errorf("could not open page: %s", err)
		}
		// Only a page this tool opened is destroyed on exit.
		defer page.Destroy()
	}
	if opts.navigate != "" {
		if err := page.Navigate(opts.navigate); err != nil {
			return fmt.Errorf("could not navigate to the ledger: %s", err)
		}
	}

	ledger, err := stockledger.ReadLedger(stockledger.AgoutiPage(page), stockledger.Options{
		RootSelector:     opts.root,
		ReceiptsSelector: opts.receipts,
		SalesSelector:    opts.sales,
		ReturnsSelector:  opts.returns,
		ReceiptsTable:    opts.receiptsTbl,
		SalesTable:       opts.salesTbl,
		ReturnsTable:     opts.returnsTbl,
	})
	if err != nil {
		return err
	}
	result, err := ledger.Compute()
	if err != nil {
		return err
	}
	if err := stockledger.WriteReport(result, opts.jsonPath); err != nil {
		return err
	}
	if opts.csvDir != "" {
		if err := stockledger.WriteCSV(result, opts.csvDir); err != nil {
			return err
		}
	}
	printSummary(result)
	return nil
}

func printSummary(result *stockledger.Result) {
	fmt.Printf("ledger version: %s\n", result.Version)
	fmt.Printf("net revenue: %s\n", stockledger.FormatCents(result.NetRevenueCents))
	fmt.Printf("cost of goods sold: %s\n", stockledger.FormatCents(result.COGSCents))
	fmt.Printf("gross profit: %s\n", stockledger.FormatCents(result.GrossCents))

	remaining := int64(0)
	for _, batch := range result.Batches {
		remaining += batch.RemainingCostCents
	}
	fmt.Printf("remaining batch valuation: %s across %d batches\n",
		stockledger.FormatCents(remaining), len(result.Batches))

	customers := make([]string, 0, len(result.Customers))
	for _, summary := range result.Customers {
		customers = append(customers, summary.Customer)
	}
	sort.Strings(customers)
	for _, customer := range customers {
		for _, summary := range result.CustomerProducts {
			if summary.Customer != customer {
				continue
			}
			fmt.Printf("  %s / %s: net revenue %s, net cost %s, gross %s\n",
				summary.Customer, summary.SKU,
				stockledger.FormatCents(summary.NetRevenueCents),
				stockledger.FormatCents(summary.NetCOGSCents),
				stockledger.FormatCents(summary.GrossCents))
		}
	}
}
