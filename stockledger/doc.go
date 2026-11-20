// Package stockledger performs inventory ledger cost accounting for a
// commerce page that is already open in an authenticated browser session.
//
// The package reads receipts, sales, and customer returns from the live DOM
// through Agouti (see ReadLedger and AgoutiPage), verifies that every row
// belongs to the same completed ledger version, and then replays the rows in
// stable business order (the ledger sequence number) to produce:
//
//   - a per-line FIFO allocation to the real receipt batches,
//   - the cost of each sale and the reversal of cost for each return,
//   - net sales revenue, cost of goods sold (COGS), and gross profit,
//   - the current valuation of every receipt batch,
//   - summaries by customer/product, by customer, and by product,
//   - files written to disk by WriteReport.
//
// All monetary values are integer cents ("分"). Amounts are never converted
// to floating point, so cent-level results are exact. A receipt's total cost
// is distributed over its quantity with a deterministic floor split; the
// remainder always stays with the unconsumed part of the batch, which means
// allocations of a fully consumed batch add up to exactly its receipt cost.
//
// # Page contract
//
// The ledger root element is selected by rootSelector (default
// "#stock-ledger"). It carries the ledger version and status:
//
//	<div id="stock-ledger" data-version="2026-11-20.1" data-status="completed">
//	  <table>
//	    <tbody>
//	      <tr data-type="receipt" data-id="R-1" data-seq="1"
//	          data-warehouse="WH1" data-sku="SKU-1" data-product="Widget"
//	          data-batch="LOT-A" data-qty="10" data-amount="100.00"></tr>
//	      <tr data-type="sale" data-id="S-1" data-seq="2"
//	          data-warehouse="WH1" data-sku="SKU-1" data-product="Widget"
//	          data-customer="C-1" data-qty="4" data-amount="80.00"></tr>
//	      <tr data-type="return" data-id="T-1" data-seq="3"
//	          data-warehouse="WH1" data-sku="SKU-1"
//	          data-ref="S-1" data-qty="2" data-amount="40.00"></tr>
//	    </tbody>
//	  </table>
//	</div>
//
// Any element containing the rows can be used as the root; rows are selected
// with "<root> tr[data-type]". Rows may appear in any order on the page: the
// sequence number is the authoritative business order, and duplicate or
// missing sequence numbers are rejected.
//
// Recognised attributes (aliases in parentheses):
//
//	data-type  receipt | sale | return
//	data-id    unique ledger event id
//	data-seq   positive integer business sequence
//	data-warehouse
//	data-sku
//	data-product (data-product-name)
//	data-qty (data-quantity)
//	data-batch (data-batch-id)       receipts only; the physical lot
//	data-customer                    sales; optional on returns (must match)
//	data-ref (data-sale-ref)         returns only; id of the original sale
//	data-amount                      receipt cost or sale/return revenue
//	data-amount-cents (data-cents)   the same value already expressed in cents
//
// If both data-amount and data-amount-cents are present they must agree.
// A return must not carry a batch: it is costed against the batches that the
// original sale actually consumed, never as a new batch.
//
// # Header-driven tables
//
// Real commerce pages often render an ordinary table whose columns are named
// only by their header cells. Pass the table selectors in Options
// (ReceiptsTable, SalesTable, ReturnsTable) or use them implicitly when the
// root contains tables but no attribute-tagged rows. The reader uses the
// first header row to map columns and accepts either English or Chinese
// labels, for example:
//
//	<table id="ledger">
//	  <thead><tr>
//	    <th>序号</th><th>类型</th><th>单号</th><th>仓库</th>
//	    <th>商品编码</th><th>批次</th><th>数量</th><th>金额</th>
//	    <th>客户</th><th>原单号</th>
//	  </tr></thead>
//	  <tbody>
//	    <tr><td>1</td><td>收货</td><td>R-1</td><td>WH1</td>
//	        <td>SKU-1</td><td>LOT-A</td><td>10</td><td>¥100.00</td>
//	        <td></td><td></td></tr>
//	  </tbody>
//	</table>
//
// When headers identify the tables, sections may drop the 类型 column; the
// section selector supplies the event type. A row that names a different
// type than its section is still rejected, so a receipt is never costed as a
// sale. A page that shows "版本: 2026-11-20.1" in a heading rather than in a
// data-version attribute is also accepted.
//
// The reader performs no clicks, form submissions, navigation, or scripts, so
// it cannot change remote inventory. Callers must supply an already
// authenticated page/session; the package has no login flow.
package stockledger
