package stockledger

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
)

// Amount is a cent amount rendered both as an integer and as a decimal string
// in the JSON report.
type Amount struct {
	Cents int64  `json:"cents"`
	Value string `json:"amount"`
}

// NewAmount wraps integer cents for serialisation.
func NewAmount(cents int64) Amount {
	return Amount{Cents: cents, Value: FormatCents(cents)}
}

// Money is a cents helper for callers that only need formatting.
func Money(cents int64) string { return FormatCents(cents) }

type allocationReport struct {
	Seq            int64     `json:"seq"`
	EventID        string    `json:"event_id"`
	EventType      EventType `json:"event_type"`
	Direction      string    `json:"direction"`
	Warehouse      string    `json:"warehouse"`
	SKU            string    `json:"sku"`
	Product        string    `json:"product"`
	Batch          string    `json:"batch"`
	ReceiptID      string    `json:"receipt_id"`
	ReceiptSeq     int64     `json:"receipt_seq"`
	Quantity       int64     `json:"quantity"`
	QuantitySigned int64     `json:"quantity_signed"`
	Cost           Amount    `json:"cost"`
	CostSigned     Amount    `json:"cost_signed"`
	Reference      string    `json:"reference,omitempty"`
}

type lineReport struct {
	Seq         int64     `json:"seq"`
	ID          string    `json:"id"`
	Type        EventType `json:"type"`
	Warehouse   string    `json:"warehouse"`
	SKU         string    `json:"sku"`
	Product     string    `json:"product"`
	Customer    string    `json:"customer"`
	Reference   string    `json:"reference,omitempty"`
	Quantity    int64     `json:"quantity"`
	Revenue     Amount    `json:"revenue"`
	Cost        Amount    `json:"cost"`
	GrossProfit Amount    `json:"gross_profit"`
}

type batchReport struct {
	ID                string `json:"id"`
	ReceiptID         string `json:"receipt_id"`
	ReceiptSeq        int64  `json:"receipt_seq"`
	Warehouse         string `json:"warehouse"`
	SKU               string `json:"sku"`
	Product           string `json:"product"`
	ReceiptQuantity   int64  `json:"receipt_quantity"`
	ReceiptCost       Amount `json:"receipt_cost"`
	ConsumedQuantity  int64  `json:"consumed_quantity"`
	ConsumedCost      Amount `json:"consumed_cost"`
	ReturnedQuantity  int64  `json:"returned_quantity"`
	ReturnedCost      Amount `json:"returned_cost"`
	RemainingQuantity int64  `json:"remaining_quantity"`
	RemainingCost     Amount `json:"remaining_cost"`
	ZeroBalance       bool   `json:"zero_balance"`
}

type customerProductReport struct {
	Customer         string `json:"customer"`
	SKU              string `json:"sku"`
	Product          string `json:"product"`
	SoldQuantity     int64  `json:"sold_quantity"`
	ReturnedQuantity int64  `json:"returned_quantity"`
	NetQuantity      int64  `json:"net_quantity"`
	Revenue          Amount `json:"revenue"`
	ReturnedRevenue  Amount `json:"returned_revenue"`
	NetRevenue       Amount `json:"net_revenue"`
	Cost             Amount `json:"cost"`
	ReversedCost     Amount `json:"reversed_cost"`
	NetCost          Amount `json:"net_cost"`
	GrossProfit      Amount `json:"gross_profit"`
	ZeroActivity     bool   `json:"zero_activity"`
}

type customerReport struct {
	Customer    string `json:"customer"`
	NetQuantity int64  `json:"net_quantity"`
	NetRevenue  Amount `json:"net_revenue"`
	NetCost     Amount `json:"net_cost"`
	GrossProfit Amount `json:"gross_profit"`
}

type productReport struct {
	SKU         string `json:"sku"`
	Product     string `json:"product"`
	NetQuantity int64  `json:"net_quantity"`
	NetRevenue  Amount `json:"net_revenue"`
	NetCost     Amount `json:"net_cost"`
	GrossProfit Amount `json:"gross_profit"`
}

type totalsReport struct {
	NetRevenue       Amount `json:"net_revenue"`
	CostOfGoodsSold  Amount `json:"cost_of_goods_sold"`
	GrossProfit      Amount `json:"gross_profit"`
	SoldQuantity     int64  `json:"sold_quantity"`
	ReturnedQuantity int64  `json:"returned_quantity"`
}

type reportDocument struct {
	Version          string                  `json:"version"`
	Totals           totalsReport            `json:"totals"`
	Lines            []lineReport            `json:"lines"`
	Allocations      []allocationReport      `json:"allocations"`
	Batches          []batchReport           `json:"batches"`
	CustomerProducts []customerProductReport `json:"customer_products"`
	Customers        []customerReport        `json:"customers"`
	Products         []productReport         `json:"products"`
}

// Report converts a Result into the JSON document written by WriteReport.
func (r *Result) Report() interface{} {
	document := reportDocument{
		Version: r.Version,
		Totals: totalsReport{
			NetRevenue:       NewAmount(r.NetRevenueCents),
			CostOfGoodsSold:  NewAmount(r.COGSCents),
			GrossProfit:      NewAmount(r.GrossCents),
			SoldQuantity:     r.SoldQuantity,
			ReturnedQuantity: r.ReturnedQuantity,
		},
		Lines:            []lineReport{},
		Allocations:      []allocationReport{},
		Batches:          []batchReport{},
		CustomerProducts: []customerProductReport{},
		Customers:        []customerReport{},
		Products:         []productReport{},
	}
	for _, line := range r.Lines {
		document.Lines = append(document.Lines, lineReport{
			Seq:         line.Seq,
			ID:          line.ID,
			Type:        line.Type,
			Warehouse:   line.Warehouse,
			SKU:         line.SKU,
			Product:     line.Product,
			Customer:    line.Customer,
			Reference:   line.Reference,
			Quantity:    line.Quantity,
			Revenue:     NewAmount(line.RevenueCents),
			Cost:        NewAmount(line.CostCents),
			GrossProfit: NewAmount(line.GrossCents),
		})
	}
	for _, allocation := range r.Allocations {
		document.Allocations = append(document.Allocations, allocationReport{
			Seq:            allocation.Seq,
			EventID:        allocation.EventID,
			EventType:      allocation.EventType,
			Direction:      allocation.Direction,
			Warehouse:      allocation.Warehouse,
			SKU:            allocation.SKU,
			Product:        allocation.Product,
			Batch:          allocation.Batch,
			ReceiptID:      allocation.ReceiptID,
			ReceiptSeq:     allocation.ReceiptSeq,
			Quantity:       allocation.Quantity,
			QuantitySigned: allocation.SignedQuantity(),
			Cost:           NewAmount(allocation.CostCents),
			CostSigned:     NewAmount(allocation.SignedCostCents()),
			Reference:      allocation.Reference,
		})
	}
	for _, batch := range r.Batches {
		document.Batches = append(document.Batches, batchReport{
			ID:                batch.ID,
			ReceiptID:         batch.ReceiptID,
			ReceiptSeq:        batch.ReceiptSeq,
			Warehouse:         batch.Warehouse,
			SKU:               batch.SKU,
			Product:           batch.Product,
			ReceiptQuantity:   batch.ReceiptQuantity,
			ReceiptCost:       NewAmount(batch.ReceiptCostCents),
			ConsumedQuantity:  batch.ConsumedQuantity,
			ConsumedCost:      NewAmount(batch.ConsumedCostCents),
			ReturnedQuantity:  batch.ReturnedQuantity,
			ReturnedCost:      NewAmount(batch.ReturnedCostCents),
			RemainingQuantity: batch.RemainingQuantity,
			RemainingCost:     NewAmount(batch.RemainingCostCents),
			ZeroBalance:       batch.RemainingQuantity == 0 && batch.RemainingCostCents == 0,
		})
	}
	for _, summary := range r.CustomerProducts {
		document.CustomerProducts = append(document.CustomerProducts, customerProductReport{
			Customer:         summary.Customer,
			SKU:              summary.SKU,
			Product:          summary.Product,
			SoldQuantity:     summary.SoldQuantity,
			ReturnedQuantity: summary.ReturnedQuantity,
			NetQuantity:      summary.NetQuantity,
			Revenue:          NewAmount(summary.RevenueCents),
			ReturnedRevenue:  NewAmount(summary.ReturnedRevenueCents),
			NetRevenue:       NewAmount(summary.NetRevenueCents),
			Cost:             NewAmount(summary.COGSCents),
			ReversedCost:     NewAmount(summary.ReversedCostCents),
			NetCost:          NewAmount(summary.NetCOGSCents),
			GrossProfit:      NewAmount(summary.GrossCents),
			ZeroActivity:     summary.NetQuantity == 0 && summary.NetRevenueCents == 0 && summary.NetCOGSCents == 0,
		})
	}
	for _, summary := range r.Customers {
		document.Customers = append(document.Customers, customerReport{
			Customer:    summary.Customer,
			NetQuantity: summary.NetQuantity,
			NetRevenue:  NewAmount(summary.NetRevenueCents),
			NetCost:     NewAmount(summary.NetCOGSCents),
			GrossProfit: NewAmount(summary.GrossCents),
		})
	}
	for _, summary := range r.Products {
		document.Products = append(document.Products, productReport{
			SKU:         summary.SKU,
			Product:     summary.Product,
			NetQuantity: summary.NetQuantity,
			NetRevenue:  NewAmount(summary.NetRevenueCents),
			NetCost:     NewAmount(summary.NetCOGSCents),
			GrossProfit: NewAmount(summary.GrossCents),
		})
	}
	return document
}

// WriteReport writes the JSON report to filename, creating parent directories
// as needed. The file is written to a temporary name and renamed so a partial
// report is never left behind.
func WriteReport(result *Result, filename string) error {
	if result == nil {
		return errorf(ErrWriteFailed, "nil result")
	}
	if filename == "" {
		return errorf(ErrWriteFailed, "no report filename was provided")
	}
	encoded, err := json.MarshalIndent(result.Report(), "", "  ")
	if err != nil {
		return errorf(ErrWriteFailed, "could not encode report: %s", err)
	}
	encoded = append(encoded, '\n')
	return writeFile(filename, encoded)
}

// WriteCSV writes one CSV per report section into dir:
//
//	lines.csv, allocations.csv, batches.csv,
//	customer_products.csv, customers.csv, products.csv
//
// Zero-cost lines and zero-balance batches are included.
func WriteCSV(result *Result, dir string) error {
	if result == nil {
		return errorf(ErrWriteFailed, "nil result")
	}
	if dir == "" {
		return errorf(ErrWriteFailed, "no CSV directory was provided")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return errorf(ErrWriteFailed, "could not create %s: %s", dir, err)
	}

	lines := [][]string{{"seq", "id", "type", "warehouse", "sku", "product", "customer", "reference", "quantity", "revenue_cents", "revenue", "cost_cents", "cost", "gross_cents", "gross"}}
	for _, line := range result.Lines {
		lines = append(lines, []string{
			strconv.FormatInt(line.Seq, 10), line.ID, string(line.Type), line.Warehouse,
			line.SKU, line.Product, line.Customer, line.Reference,
			strconv.FormatInt(line.Quantity, 10),
			strconv.FormatInt(line.RevenueCents, 10), FormatCents(line.RevenueCents),
			strconv.FormatInt(line.CostCents, 10), FormatCents(line.CostCents),
			strconv.FormatInt(line.GrossCents, 10), FormatCents(line.GrossCents),
		})
	}
	if err := writeCSV(filepath.Join(dir, "lines.csv"), lines); err != nil {
		return err
	}

	allocations := [][]string{{"seq", "event_id", "event_type", "direction", "warehouse", "sku", "product", "batch", "receipt_id", "receipt_seq", "quantity", "quantity_signed", "cost_cents", "cost", "cost_signed_cents", "cost_signed", "reference"}}
	for _, allocation := range result.Allocations {
		allocations = append(allocations, []string{
			strconv.FormatInt(allocation.Seq, 10), allocation.EventID, string(allocation.EventType),
			allocation.Direction, allocation.Warehouse, allocation.SKU, allocation.Product,
			allocation.Batch, allocation.ReceiptID, strconv.FormatInt(allocation.ReceiptSeq, 10),
			strconv.FormatInt(allocation.Quantity, 10), strconv.FormatInt(allocation.SignedQuantity(), 10),
			strconv.FormatInt(allocation.CostCents, 10), FormatCents(allocation.CostCents),
			strconv.FormatInt(allocation.SignedCostCents(), 10), FormatCents(allocation.SignedCostCents()),
			allocation.Reference,
		})
	}
	if err := writeCSV(filepath.Join(dir, "allocations.csv"), allocations); err != nil {
		return err
	}

	batches := [][]string{{"batch", "receipt_id", "receipt_seq", "warehouse", "sku", "product", "receipt_quantity", "receipt_cost_cents", "receipt_cost", "consumed_quantity", "consumed_cost_cents", "consumed_cost", "returned_quantity", "returned_cost_cents", "returned_cost", "remaining_quantity", "remaining_cost_cents", "remaining_cost", "zero_balance"}}
	for _, batch := range result.Batches {
		batches = append(batches, []string{
			batch.ID, batch.ReceiptID, strconv.FormatInt(batch.ReceiptSeq, 10),
			batch.Warehouse, batch.SKU, batch.Product,
			strconv.FormatInt(batch.ReceiptQuantity, 10),
			strconv.FormatInt(batch.ReceiptCostCents, 10), FormatCents(batch.ReceiptCostCents),
			strconv.FormatInt(batch.ConsumedQuantity, 10),
			strconv.FormatInt(batch.ConsumedCostCents, 10), FormatCents(batch.ConsumedCostCents),
			strconv.FormatInt(batch.ReturnedQuantity, 10),
			strconv.FormatInt(batch.ReturnedCostCents, 10), FormatCents(batch.ReturnedCostCents),
			strconv.FormatInt(batch.RemainingQuantity, 10),
			strconv.FormatInt(batch.RemainingCostCents, 10), FormatCents(batch.RemainingCostCents),
			strconv.FormatBool(batch.RemainingQuantity == 0 && batch.RemainingCostCents == 0),
		})
	}
	if err := writeCSV(filepath.Join(dir, "batches.csv"), batches); err != nil {
		return err
	}

	customerProducts := [][]string{{"customer", "sku", "product", "sold_quantity", "returned_quantity", "net_quantity", "revenue_cents", "revenue", "returned_revenue_cents", "returned_revenue", "net_revenue_cents", "net_revenue", "cost_cents", "cost", "reversed_cost_cents", "reversed_cost", "net_cost_cents", "net_cost", "gross_cents", "gross"}}
	for _, summary := range result.CustomerProducts {
		customerProducts = append(customerProducts, []string{
			summary.Customer, summary.SKU, summary.Product,
			strconv.FormatInt(summary.SoldQuantity, 10), strconv.FormatInt(summary.ReturnedQuantity, 10),
			strconv.FormatInt(summary.NetQuantity, 10),
			strconv.FormatInt(summary.RevenueCents, 10), FormatCents(summary.RevenueCents),
			strconv.FormatInt(summary.ReturnedRevenueCents, 10), FormatCents(summary.ReturnedRevenueCents),
			strconv.FormatInt(summary.NetRevenueCents, 10), FormatCents(summary.NetRevenueCents),
			strconv.FormatInt(summary.COGSCents, 10), FormatCents(summary.COGSCents),
			strconv.FormatInt(summary.ReversedCostCents, 10), FormatCents(summary.ReversedCostCents),
			strconv.FormatInt(summary.NetCOGSCents, 10), FormatCents(summary.NetCOGSCents),
			strconv.FormatInt(summary.GrossCents, 10), FormatCents(summary.GrossCents),
		})
	}
	if err := writeCSV(filepath.Join(dir, "customer_products.csv"), customerProducts); err != nil {
		return err
	}

	customers := [][]string{{"customer", "net_quantity", "net_revenue_cents", "net_revenue", "net_cost_cents", "net_cost", "gross_cents", "gross"}}
	for _, summary := range result.Customers {
		customers = append(customers, []string{
			summary.Customer, strconv.FormatInt(summary.NetQuantity, 10),
			strconv.FormatInt(summary.NetRevenueCents, 10), FormatCents(summary.NetRevenueCents),
			strconv.FormatInt(summary.NetCOGSCents, 10), FormatCents(summary.NetCOGSCents),
			strconv.FormatInt(summary.GrossCents, 10), FormatCents(summary.GrossCents),
		})
	}
	if err := writeCSV(filepath.Join(dir, "customers.csv"), customers); err != nil {
		return err
	}

	products := [][]string{{"sku", "product", "net_quantity", "net_revenue_cents", "net_revenue", "net_cost_cents", "net_cost", "gross_cents", "gross"}}
	for _, summary := range result.Products {
		products = append(products, []string{
			summary.SKU, summary.Product, strconv.FormatInt(summary.NetQuantity, 10),
			strconv.FormatInt(summary.NetRevenueCents, 10), FormatCents(summary.NetRevenueCents),
			strconv.FormatInt(summary.NetCOGSCents, 10), FormatCents(summary.NetCOGSCents),
			strconv.FormatInt(summary.GrossCents, 10), FormatCents(summary.GrossCents),
		})
	}
	return writeCSV(filepath.Join(dir, "products.csv"), products)
}

func writeCSV(filename string, rows [][]string) error {
	buffer := &bytes.Buffer{}
	writer := csv.NewWriter(buffer)
	if err := writer.WriteAll(rows); err != nil {
		return errorf(ErrWriteFailed, "could not encode %s: %s", filename, err)
	}
	return writeFile(filename, buffer.Bytes())
}

func writeFile(filename string, contents []byte) error {
	dir := filepath.Dir(filename)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return errorf(ErrWriteFailed, "could not create %s: %s", dir, err)
		}
	}
	temporary, err := ioutil.TempFile(dir, "."+filepath.Base(filename)+".tmp")
	if err != nil {
		return errorf(ErrWriteFailed, "could not create temporary file for %s: %s", filename, err)
	}
	temporaryName := temporary.Name()
	if _, err := io.Copy(temporary, bytes.NewReader(contents)); err != nil {
		temporary.Close()
		os.Remove(temporaryName)
		return errorf(ErrWriteFailed, "could not write %s: %s", filename, err)
	}
	if err := temporary.Close(); err != nil {
		os.Remove(temporaryName)
		return errorf(ErrWriteFailed, "could not close %s: %s", filename, err)
	}
	if err := os.Chmod(temporaryName, 0644); err != nil {
		os.Remove(temporaryName)
		return errorf(ErrWriteFailed, "could not set permissions on %s: %s", filename, err)
	}
	if err := os.Rename(temporaryName, filename); err != nil {
		os.Remove(temporaryName)
		return errorf(ErrWriteFailed, "could not replace %s: %s", filename, err)
	}
	return nil
}
