package stockledger_test

import (
	"errors"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sclevine/agouti/stockledger"
)

// The tests below run the reader against a small DOM model with a real (if
// minimal) CSS selector engine, so the selectors the reader issues are
// exercised the way a browser would resolve them.

type node struct {
	tag      string
	attrs    map[string]string
	text     string
	parent   *node
	children []*node
}

func newNode(tag string, attrs map[string]string) *node {
	if attrs == nil {
		attrs = map[string]string{}
	}
	return &node{tag: tag, attrs: attrs}
}

func (n *node) append(children ...*node) *node {
	for _, child := range children {
		child.parent = n
		n.children = append(n.children, child)
	}
	return n
}

func (n *node) innerText() string {
	if len(n.children) == 0 {
		return n.text
	}
	parts := []string{}
	if n.text != "" {
		parts = append(parts, n.text)
	}
	for _, child := range n.children {
		parts = append(parts, child.innerText())
	}
	return strings.Join(parts, "")
}

type fakePage struct {
	root *node
}

type fakeSelection struct {
	nodes []*node
}

func (p *fakePage) Find(selector string) stockledger.ElementReader {
	matches := p.selectAll(selector)
	if len(matches) > 0 {
		matches = matches[:1]
	}
	return &fakeSelection{nodes: matches}
}

func (p *fakePage) All(selector string) stockledger.ElementReader {
	return &fakeSelection{nodes: p.selectAll(selector)}
}

func (p *fakePage) selectAll(selector string) []*node {
	if p.root == nil {
		return nil
	}
	return query(p.root, selector)
}

func (s *fakeSelection) Attribute(name string) (string, error) {
	if len(s.nodes) == 0 {
		return "", nil
	}
	return s.nodes[0].attrs[name], nil
}

func (s *fakeSelection) Text() (string, error) {
	if len(s.nodes) == 0 {
		return "", nil
	}
	return s.nodes[0].innerText(), nil
}

func (s *fakeSelection) Count() (int, error) { return len(s.nodes), nil }

func (s *fakeSelection) At(index int) stockledger.ElementReader {
	return &fakeSelection{nodes: s.nodes[index : index+1]}
}

func (s *fakeSelection) Find(selector string) stockledger.ElementReader {
	for _, candidate := range s.nodes {
		matches := query(candidate, selector)
		if len(matches) > 0 {
			return &fakeSelection{nodes: matches[:1]}
		}
	}
	return &fakeSelection{}
}

func (s *fakeSelection) All(selector string) stockledger.ElementReader {
	matches := []*node{}
	for _, candidate := range s.nodes {
		matches = append(matches, query(candidate, selector)...)
	}
	return &fakeSelection{nodes: matches}
}

// query resolves a comma-separated selector list against a scope node.
func query(scope *node, selector string) []*node {
	matches := []*node{}
	seen := map[*node]bool{}
	for _, alternative := range strings.Split(selector, ",") {
		components := strings.Fields(alternative)
		if len(components) == 0 {
			continue
		}
		walk(scope, func(candidate *node) {
			if matchesComponents(candidate, components) && !seen[candidate] {
				seen[candidate] = true
				matches = append(matches, candidate)
			}
		})
	}
	return matches
}

func walk(scope *node, visit func(*node)) {
	for _, child := range scope.children {
		visit(child)
		walk(child, visit)
	}
}

func matchesComponents(candidate *node, components []string) bool {
	if !matchesComponent(candidate, components[len(components)-1]) {
		return false
	}
	ancestor := candidate.parent
	for index := len(components) - 2; index >= 0; index-- {
		for ancestor != nil && !matchesComponent(ancestor, components[index]) {
			ancestor = ancestor.parent
		}
		if ancestor == nil {
			return false
		}
		ancestor = ancestor.parent
	}
	return true
}

func matchesComponent(candidate *node, component string) bool {
	if strings.HasSuffix(component, ":first-child") {
		component = strings.TrimSuffix(component, ":first-child")
		if candidate.parent == nil || len(candidate.parent.children) == 0 || candidate.parent.children[0] != candidate {
			return false
		}
	}
	if strings.HasPrefix(component, ".") {
		return strings.Contains(" "+candidate.attrs["class"]+" ", " "+component[1:]+" ")
	}
	if strings.HasPrefix(component, "#") {
		return candidate.attrs["id"] == component[1:]
	}
	tag := component
	if bracket := strings.Index(component, "["); bracket >= 0 {
		tag = component[:bracket]
		attribute := strings.TrimSuffix(component[bracket+1:], "]")
		name := attribute
		if equals := strings.Index(attribute, "="); equals >= 0 {
			name = attribute[:equals]
			value := strings.Trim(attribute[equals+1:], `"'`)
			if candidate.attrs[name] != value {
				return false
			}
		} else if _, ok := candidate.attrs[name]; !ok {
			return false
		}
	}
	return tag == "" || tag == "*" || candidate.tag == tag
}

func element(tag string, attrs map[string]string, children ...*node) *node {
	return newNode(tag, attrs).append(children...)
}

func textNode(tag, text string) *node {
	node := newNode(tag, nil)
	node.text = text
	return node
}

// ledgerPage builds a page with a #stock-ledger section holding one table
// whose rows are attribute-tagged.
func ledgerPage(rootAttributes map[string]string, rows ...map[string]string) *fakePage {
	tableRows := []*node{}
	for _, attributes := range rows {
		tableRows = append(tableRows, newNode("tr", attributes))
	}
	return &fakePage{root: element("html", nil,
		element("body", nil,
			element("div", map[string]string{"id": "nav"}),
			element("section", withID(rootAttributes, "stock-ledger"),
				element("table", nil,
					newNode("tbody", nil).append(tableRows...),
				),
			),
		),
	)}
}

func withID(attributes map[string]string, id string) map[string]string {
	merged := map[string]string{"id": id}
	for name, value := range attributes {
		merged[name] = value
	}
	return merged
}

func tableSection(id string, headers []string, rows [][]string, tableAttributes map[string]string) *node {
	headerCells := []*node{}
	for _, header := range headers {
		headerCells = append(headerCells, textNode("th", header))
	}
	bodyRows := []*node{}
	for _, row := range rows {
		cells := []*node{}
		for _, cell := range row {
			cells = append(cells, textNode("td", cell))
		}
		bodyRows = append(bodyRows, newNode("tr", nil).append(cells...))
	}
	return element("section", withID(tableAttributes, id),
		element("table", nil,
			newNode("thead", nil).append(newNode("tr", nil).append(headerCells...)),
			newNode("tbody", nil).append(bodyRows...),
		),
	)
}

func ledgerRow(attributes map[string]string) map[string]string { return attributes }

var _ = Describe("Reading the ledger from a page", func() {
	It("reads a completed attribute-tagged ledger through the page", func() {
		page := ledgerPage(map[string]string{"data-version": "2026-11-20.1", "data-status": "completed"},
			ledgerRow(map[string]string{"data-type": "receipt", "data-id": "R1", "data-seq": "1", "data-warehouse": "WH1", "data-sku": "A", "data-batch": "LOT1", "data-qty": "5", "data-amount": "50.00"}),
			ledgerRow(map[string]string{"data-type": "sale", "data-id": "S1", "data-seq": "2", "data-warehouse": "WH1", "data-sku": "A", "data-customer": "C1", "data-qty": "2", "data-amount": "40.00"}),
			ledgerRow(map[string]string{"data-type": "return", "data-id": "T1", "data-seq": "3", "data-warehouse": "WH1", "data-sku": "A", "data-ref": "S1", "data-qty": "1", "data-amount": "20.00"}),
		)
		ledger, err := stockledger.ReadLedger(page, stockledger.Options{})
		Expect(err).ToNot(HaveOccurred())
		Expect(ledger.Version).To(Equal("2026-11-20.1"))
		Expect(ledger.Rows).To(HaveLen(3))

		result, err := ledger.Compute()
		Expect(err).ToNot(HaveOccurred())
		Expect(result.NetRevenueCents).To(Equal(int64(2000)))
		Expect(result.COGSCents).To(Equal(int64(1000)))
		Expect(result.GrossCents).To(Equal(int64(1000)))
	})

	It("reads a ledger whose rows are ordinary table cells", func() {
		page := &fakePage{root: element("html", nil, element("body", nil,
			element("div", map[string]string{"id": "stock-ledger", "data-version": "2026-11-20.1", "data-status": "completed"},
				tableSection("ledger-table",
					[]string{"序号", "类型", "单号", "仓库", "商品编码", "批次", "数量", "金额", "客户", "原单号"},
					[][]string{
						{"1", "收货", "R1", "WH1", "A", "LOT1", "10", "¥100.00", "", ""},
						{"2", "销售", "S1", "WH1", "A", "", "4", "240.00", "C1", ""},
						{"3", "退货", "T1", "WH1", "A", "", "2", "40.00", "C1", "S1"},
					}, nil),
			),
		))}
		ledger, err := stockledger.ReadLedger(page, stockledger.Options{})
		Expect(err).ToNot(HaveOccurred())
		Expect(ledger.Rows).To(HaveLen(3))
		Expect(ledger.Rows[0].Type).To(Equal(stockledger.EventReceipt))
		Expect(ledger.Rows[1].Type).To(Equal(stockledger.EventSale))
		Expect(ledger.Rows[2].Type).To(Equal(stockledger.EventReturn))
		Expect(ledger.Rows[0].Cents).To(Equal(int64(10000)))
		Expect(ledger.Rows[2].Reference).To(Equal("S1"))

		result, err := ledger.Compute()
		Expect(err).ToNot(HaveOccurred())
		Expect(result.NetRevenueCents).To(Equal(int64(20000)))
		Expect(result.COGSCents).To(Equal(int64(2000)))
		Expect(batchByName(result, "LOT1").RemainingQuantity).To(Equal(int64(8)))
	})

	It("distinguishes 金额 from 金额分 columns", func() {
		build := func(centCell string) *fakePage {
			return &fakePage{root: element("html", nil, element("body", nil,
				element("div", map[string]string{"id": "stock-ledger", "data-version": "v1"},
					tableSection("ledger-table",
						[]string{"序号", "类型", "单号", "仓库", "商品编码", "批次", "数量", "金额", "金额分"},
						[][]string{{"1", "收货", "R1", "WH1", "A", "LOT1", "4", "¥100.00", centCell}}, nil),
				),
			))}
		}
		ledger, err := stockledger.ReadLedger(build("10000"), stockledger.Options{})
		Expect(err).ToNot(HaveOccurred())
		Expect(ledger.Rows[0].Cents).To(Equal(int64(10000)))

		_, err = stockledger.ReadLedger(build("9900"), stockledger.Options{})
		expectCode(err, stockledger.ErrConflictingAmount)
	})

	It("fails a return row that has no reference column", func() {
		page := &fakePage{root: element("html", nil, element("body", nil,
			element("div", map[string]string{"id": "stock-ledger", "data-version": "2026-11-20.1"},
				tableSection("ledger-table",
					[]string{"序号", "类型", "单号", "仓库", "商品编码", "批次", "数量", "金额"},
					[][]string{
						{"1", "收货", "R1", "WH1", "A", "LOT1", "10", "100.00"},
						{"2", "销售", "S1", "WH1", "A", "", "4", "240.00"},
						{"3", "退货", "T1", "WH1", "A", "", "2", "40.00"},
					}, nil),
			),
		))}
		_, err := stockledger.ReadLedger(page, stockledger.Options{})
		expectCode(err, stockledger.ErrMissingReference)
	})

	It("reads an English-headed table with a reference column", func() {
		page := &fakePage{root: element("html", nil, element("body", nil,
			element("div", map[string]string{"id": "stock-ledger", "data-version": "v5"},
				tableSection("ledger-table",
					[]string{"Sequence", "Type", "ID", "Warehouse", "SKU", "Batch", "Qty", "Amount", "Customer", "Reference"},
					[][]string{
						{"1", "receipt", "R1", "WH1", "A", "LOT1", "10", "100.00", "", ""},
						{"2", "sale", "S1", "WH1", "A", "", "4", "240.00", "C1", ""},
						{"3", "return", "T1", "WH1", "A", "", "2", "40.00", "C1", "S1"},
					}, nil),
			),
		))}
		ledger, err := stockledger.ReadLedger(page, stockledger.Options{})
		Expect(err).ToNot(HaveOccurred())
		result, err := ledger.Compute()
		Expect(err).ToNot(HaveOccurred())
		Expect(result.Allocations).To(HaveLen(2))
		Expect(result.Batches[0].RemainingQuantity).To(Equal(int64(8)))
	})

	It("reads a ledger version from a heading", func() {
		page := &fakePage{root: element("html", nil, element("body", nil,
			element("section", map[string]string{"id": "stock-ledger"},
				textNode("h2", "库存流水 版本: 2026-11-20.7 状态: completed"),
				element("table", nil,
					element("thead", nil, newNode("tr", nil).append(
						textNode("th", "序号"), textNode("th", "类型"), textNode("th", "单号"),
						textNode("th", "仓库"), textNode("th", "商品编码"), textNode("th", "批次"),
						textNode("th", "数量"), textNode("th", "金额"))),
					element("tbody", nil, newNode("tr", nil).append(
						textNode("td", "1"), textNode("td", "收货"), textNode("td", "R1"),
						textNode("td", "WH1"), textNode("td", "A"), textNode("td", "LOT1"),
						textNode("td", "5"), textNode("td", "50.00"))),
				),
			),
		))}
		ledger, err := stockledger.ReadLedger(page, stockledger.Options{})
		Expect(err).ToNot(HaveOccurred())
		Expect(ledger.Version).To(Equal("2026-11-20.7"))
	})

	It("uses a custom root selector", func() {
		page := &fakePage{root: element("html", nil, element("body", nil,
			element("div", map[string]string{"id": "ledger", "data-version": "v2"},
				element("table", nil, element("tbody", nil, newNode("tr", map[string]string{
					"data-type": "receipt", "data-id": "R1", "data-seq": "1", "data-warehouse": "WH1",
					"data-sku": "A", "data-batch": "LOT1", "data-qty": "5", "data-amount": "50.00",
				}))),
			),
		))}
		ledger, err := stockledger.ReadLedger(page, stockledger.Options{RootSelector: "#ledger"})
		Expect(err).ToNot(HaveOccurred())
		Expect(ledger.Version).To(Equal("v2"))
	})

	It("fails when the root is missing", func() {
		_, err := stockledger.ReadLedger(&fakePage{root: element("html", nil)}, stockledger.Options{})
		expectCode(err, stockledger.ErrMissingRoot)
	})

	It("reads a table that uses its first row as the header", func() {
		page := &fakePage{root: element("html", nil, element("body", nil,
			element("div", map[string]string{"id": "stock-ledger", "data-version": "v1"},
				element("table", nil,
					newNode("tr", nil).append(
						textNode("th", "序号"), textNode("th", "类型"), textNode("th", "单号"),
						textNode("th", "仓库"), textNode("th", "商品编码"), textNode("th", "批次"),
						textNode("th", "数量"), textNode("th", "金额")),
					newNode("tr", nil).append(
						textNode("td", "1"), textNode("td", "收货"), textNode("td", "R1"),
						textNode("td", "WH1"), textNode("td", "A"), textNode("td", "LOT1"),
						textNode("td", "5"), textNode("td", "50.00")),
				),
			),
		))}
		ledger, err := stockledger.ReadLedger(page, stockledger.Options{})
		Expect(err).ToNot(HaveOccurred())
		Expect(ledger.Rows).To(HaveLen(1))
		Expect(ledger.Rows[0].ID).To(Equal("R1"))
	})

	It("ignores tables outside the ledger root", func() {
		page := &fakePage{root: element("html", nil, element("body", nil,
			element("div", map[string]string{"id": "unrelated"},
				element("table", nil,
					newNode("thead", nil).append(newNode("tr", nil).append(textNode("th", "序号"), textNode("th", "类型"))),
					newNode("tbody", nil).append(newNode("tr", nil).append(textNode("td", "1"), textNode("td", "sale"))))),
			element("div", map[string]string{"id": "stock-ledger", "data-version": "v1"},
				element("table", nil,
					newNode("thead", nil).append(newNode("tr", nil).append(
						textNode("th", "序号"), textNode("th", "类型"), textNode("th", "单号"),
						textNode("th", "仓库"), textNode("th", "商品编码"), textNode("th", "批次"),
						textNode("th", "数量"), textNode("th", "金额"))),
					newNode("tbody", nil).append(newNode("tr", nil).append(
						textNode("td", "1"), textNode("td", "收货"), textNode("td", "R1"),
						textNode("td", "WH1"), textNode("td", "A"), textNode("td", "LOT1"),
						textNode("td", "5"), textNode("td", "50.00")))),
			),
		))}
		ledger, err := stockledger.ReadLedger(page, stockledger.Options{})
		Expect(err).ToNot(HaveOccurred())
		Expect(ledger.Rows).To(HaveLen(1))
		Expect(ledger.Rows[0].Type).To(Equal(stockledger.EventReceipt))
	})

	It("reads section tables whose columns omit the type", func() {
		page := &fakePage{root: element("html", nil, element("body", nil,
			element("div", map[string]string{"id": "stock-ledger", "data-version": "v4"}),
			tableSection("receipts",
				[]string{"序号", "单号", "仓库", "商品编码", "批次", "数量", "金额"},
				[][]string{{"1", "R1", "WH1", "A", "LOT1", "10", "100.00"}}, nil),
			tableSection("sales",
				[]string{"序号", "单号", "仓库", "商品编码", "数量", "金额", "客户"},
				[][]string{{"2", "S1", "WH1", "A", "4", "80.00", "C1"}}, nil),
			tableSection("returns",
				[]string{"序号", "单号", "仓库", "商品编码", "数量", "金额", "原单号"},
				[][]string{{"3", "T1", "WH1", "A", "1", "20.00", "S1"}}, nil),
		))}
		ledger, err := stockledger.ReadLedger(page, stockledger.Options{
			RootSelector:  "#stock-ledger",
			ReceiptsTable: "#receipts table",
			SalesTable:    "#sales table",
			ReturnsTable:  "#returns table",
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(ledger.Rows).To(HaveLen(3))
		Expect(ledger.Rows[2].Reference).To(Equal("S1"))

		result, err := ledger.Compute()
		Expect(err).ToNot(HaveOccurred())
		Expect(result.NetRevenueCents).To(Equal(int64(6000)))
		Expect(result.COGSCents).To(Equal(int64(3000)))
		Expect(batchByName(result, "LOT1").RemainingQuantity).To(Equal(int64(7)))
	})

	It("fails when the version is missing", func() {
		page := ledgerPage(nil,
			ledgerRow(map[string]string{"data-type": "receipt", "data-id": "R1", "data-seq": "1", "data-warehouse": "WH1", "data-sku": "A", "data-batch": "LOT1", "data-qty": "5", "data-amount": "50.00"}),
		)
		_, err := stockledger.ReadLedger(page, stockledger.Options{})
		expectCode(err, stockledger.ErrMissingVersion)
	})

	It("refuses an in-flight ledger version", func() {
		page := ledgerPage(map[string]string{"data-version": "v3", "data-status": "draft"},
			ledgerRow(map[string]string{"data-type": "receipt", "data-id": "R1", "data-seq": "1", "data-warehouse": "WH1", "data-sku": "A", "data-batch": "LOT1", "data-qty": "5", "data-amount": "50.00"}),
		)
		_, err := stockledger.ReadLedger(page, stockledger.Options{})
		expectCode(err, stockledger.ErrNotCompleted)
	})

	It("does not double count when the same page is read twice", func() {
		page := ledgerPage(map[string]string{"data-version": "v1", "data-status": "complete"},
			ledgerRow(map[string]string{"data-type": "receipt", "data-id": "R1", "data-seq": "1", "data-warehouse": "WH1", "data-sku": "A", "data-batch": "LOT1", "data-qty": "5", "data-amount": "50.00"}),
			ledgerRow(map[string]string{"data-type": "sale", "data-id": "S1", "data-seq": "2", "data-warehouse": "WH1", "data-sku": "A", "data-customer": "C1", "data-qty": "2", "data-amount": "40.00"}),
		)
		first, err := stockledger.ReadLedger(page, stockledger.Options{})
		Expect(err).ToNot(HaveOccurred())
		second, err := stockledger.ReadLedger(page, stockledger.Options{})
		Expect(err).ToNot(HaveOccurred())
		Expect(second.Rows).To(Equal(first.Rows))

		firstResult, err := first.Compute()
		Expect(err).ToNot(HaveOccurred())
		secondResult, err := second.Compute()
		Expect(err).ToNot(HaveOccurred())
		Expect(secondResult.Batches).To(Equal(firstResult.Batches))
		Expect(secondResult.COGSCents).To(Equal(firstResult.COGSCents))
		Expect(secondResult.GrossCents).To(Equal(firstResult.GrossCents))
	})

	It("reads separate receipt and sale sections", func() {
		page := &fakePage{root: element("html", nil, element("body", nil,
			element("div", map[string]string{"id": "stock-ledger", "data-version": "v9"}),
			tableSection("receipts",
				[]string{"序号", "单号", "仓库", "商品编码", "批次", "数量", "金额"},
				[][]string{{"1", "R1", "WH1", "A", "LOT1", "5", "50.00"}}, nil),
			tableSection("sales",
				[]string{"序号", "单号", "仓库", "商品编码", "数量", "金额", "客户"},
				[][]string{{"2", "S1", "WH1", "A", "2", "40.00", "C1"}}, nil),
		))}
		ledger, err := stockledger.ReadLedger(page, stockledger.Options{
			RootSelector:     "#stock-ledger",
			ReceiptsSelector: "#receipts",
			SalesSelector:    "#sales",
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(ledger.Version).To(Equal("v9"))
		Expect(ledger.Rows).To(HaveLen(2))
		Expect(ledger.Rows[0].Type).To(Equal(stockledger.EventReceipt))
		Expect(ledger.Rows[1].Type).To(Equal(stockledger.EventSale))
	})

	It("fails when a section contains a row tagged as another event type", func() {
		page := &fakePage{root: element("html", nil, element("body", nil,
			element("section", map[string]string{"id": "receipts", "data-version": "v9"},
				element("table", nil, element("tbody", nil, newNode("tr", map[string]string{
					"data-type": "sale", "data-id": "S1", "data-seq": "1", "data-warehouse": "WH1",
					"data-sku": "A", "data-customer": "C1", "data-qty": "2", "data-amount": "40.00",
				}))),
			),
		))}
		_, err := stockledger.ReadLedger(page, stockledger.Options{ReceiptsSelector: "#receipts"})
		expectCode(err, stockledger.ErrUnknownEventType)
	})

	It("reports a read failure when the page cannot be queried", func() {
		_, err := stockledger.ReadLedger(failingPage{}, stockledger.Options{})
		expectCode(err, stockledger.ErrReadFailed)
	})
})

type failingPage struct{}

func (failingPage) Find(selector string) stockledger.ElementReader { return failingElement{} }

func (failingPage) All(selector string) stockledger.ElementReader { return failingElement{} }

type failingElement struct{}

func (failingElement) Attribute(name string) (string, error) {
	return "", errors.New("the browser session is gone")
}

func (failingElement) Text() (string, error) { return "", errors.New("the browser session is gone") }

func (failingElement) Count() (int, error) { return 0, errors.New("the browser session is gone") }

func (failingElement) At(index int) stockledger.ElementReader { return failingElement{} }

func (failingElement) Find(selector string) stockledger.ElementReader { return failingElement{} }

func (failingElement) All(selector string) stockledger.ElementReader { return failingElement{} }
