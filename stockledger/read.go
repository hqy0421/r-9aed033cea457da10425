package stockledger

import (
	"strings"
)

// ElementReader is the subset of the Agouti selection API that the ledger
// reader needs. *agouti.Selection and *agouti.MultiSelection are adapted to it
// by AgoutiSelection and AgoutiMultiSelection, so unit tests can supply a fake
// DOM without a browser.
type ElementReader interface {
	Attribute(name string) (string, error)
	Text() (string, error)
	Count() (int, error)
	At(index int) ElementReader
	Find(selector string) ElementReader
	All(selector string) ElementReader
}

// Page is the subset of the Agouti page API that the ledger reader needs.
// *agouti.Page is adapted to it by AgoutiPage. Find returns a single-element
// selection and All returns a multi-element selection, mirroring Agouti.
type Page interface {
	Find(selector string) ElementReader
	All(selector string) ElementReader
}

// DefaultRootSelector is the ledger root element used when Options.RootSelector
// is empty.
const DefaultRootSelector = "#stock-ledger"

// rowSelector matches ledger row elements tagged with data attributes.
const rowSelector = "tr[data-type], tr[data-id], tr[data-event], tr[row-id]"

// Options customises how the ledger is read from the page.
type Options struct {
	// RootSelector selects the element that carries the ledger version and
	// contains the ledger rows. Defaults to DefaultRootSelector. It is required
	// unless a section selector or a table selector is provided.
	RootSelector string

	// ReceiptsSelector, SalesSelector, and ReturnsSelector optionally select the
	// rows of each section directly, for pages that keep receipts, sales, and
	// returns in separate tables and do not tag rows with data-type. Each
	// selector may match row elements that carry data-id/data-type, or
	// containers whose <tr> descendants carry them.
	//
	// When a section selector is used, every row it yields is treated as that
	// event type. A row that also carries data-type must agree with its section,
	// otherwise the read fails, so a receipt is never silently read as a sale.
	// Rows from all sections share one ledger version.
	ReceiptsSelector string
	SalesSelector    string
	ReturnsSelector  string

	// ReceiptsTable, SalesTable, and ReturnsTable are like the *Selector
	// options but for ordinary HTML tables whose columns are identified by their
	// <th> headers rather than by data attributes. Header labels may be English
	// or Chinese ("类型", "单号", "序号", "仓库", "商品编码", "批次", "数量",
	// "金额", "客户", "原单号"); see the package documentation.
	ReceiptsTable string
	SalesTable    string
	ReturnsTable  string
}

func (o Options) sectioned() bool {
	return o.ReceiptsSelector != "" || o.SalesSelector != "" || o.ReturnsSelector != "" ||
		o.ReceiptsTable != "" || o.SalesTable != "" || o.ReturnsTable != ""
}

// ReadLedger reads and validates one completed ledger version from an
// already-authenticated Agouti page. The page must already be on the ledger
// view; this function does not navigate, log in, click, type, submit, or run
// scripts, so remote inventory is never modified. Reading the same unchanged
// page twice yields the same Ledger, and calling ReadLedger again does not
// double count.
func ReadLedger(page Page, options Options) (*Ledger, error) {
	if page == nil {
		return nil, errorf(ErrReadFailed, "no page was provided")
	}
	if options.sectioned() {
		return readSections(page, options)
	}
	rootSelector := options.RootSelector
	if rootSelector == "" {
		rootSelector = DefaultRootSelector
	}

	root := page.Find(rootSelector)
	count, err := root.Count()
	if err != nil {
		return nil, errorf(ErrReadFailed, "could not locate ledger root %q: %s", rootSelector, err)
	}
	if count == 0 {
		return nil, errorf(ErrMissingRoot, "no element matched the ledger root %q", rootSelector)
	}

	version, err := readLedgerHeader(root, rootSelector)
	if err != nil {
		return nil, err
	}
	rows := page.All(rootSelector + " " + rowSelector)
	attributes, err := readRows(rows)
	if err != nil {
		return nil, err
	}
	if len(attributes) == 0 {
		// Fall back to header-driven tables inside the ledger root. Tables
		// elsewhere on the page are never read: they could belong to another
		// widget, and costing the wrong rows would silently corrupt the report.
		attributes, err = readTable(page.All(rootSelector + " table"))
		if err != nil {
			return nil, err
		}
	}
	if len(attributes) == 0 {
		return nil, errorf(ErrReadFailed,
			"the ledger root %q contains no ledger rows", rootSelector)
	}
	return ParseRows(version, attributes)
}

func readSections(page Page, options Options) (*Ledger, error) {
	version := ""
	if options.RootSelector != "" {
		root := page.Find(options.RootSelector)
		count, err := root.Count()
		if err != nil {
			return nil, errorf(ErrReadFailed, "could not locate ledger root %q: %s", options.RootSelector, err)
		}
		if count > 0 {
			if version, err = readLedgerHeader(root, options.RootSelector); err != nil {
				return nil, err
			}
		}
	}

	sections := []struct {
		selector  string
		table     string
		eventType EventType
		label     string
	}{
		{options.ReceiptsSelector, options.ReceiptsTable, EventReceipt, "receipts"},
		{options.SalesSelector, options.SalesTable, EventSale, "sales"},
		{options.ReturnsSelector, options.ReturnsTable, EventReturn, "returns"},
	}

	attributes := []Attributes{}
	for _, section := range sections {
		var sectionRows []Attributes
		var sectionVersion string
		var err error
		switch {
		case section.selector != "":
			sectionRows, sectionVersion, err = readSectionRows(page, section.selector, section.eventType, section.label)
		case section.table != "":
			sectionRows, sectionVersion, err = readSectionTable(page, section.table, section.eventType, section.label)
		default:
			continue
		}
		if err != nil {
			return nil, err
		}
		if version == "" {
			version = sectionVersion
		} else if sectionVersion != "" && sectionVersion != version {
			return nil, errorf(ErrMixedVersion,
				"the %s section belongs to ledger version %q, but the page reports %q",
				section.label, sectionVersion, version)
		}
		attributes = append(attributes, sectionRows...)
	}
	return ParseRows(version, attributes)
}

// Header text labels used when the version or status is shown as text rather
// than as an attribute, e.g. "库存流水 版本: 2026-11-20.1 状态: completed".
var (
	versionHeaderLabels = []string{"ledger version", "version", "版本号", "版本"}
	statusHeaderLabels  = []string{"ledger status", "status", "状态"}
)

func readLedgerHeader(root ElementReader, rootSelector string) (string, error) {
	version, err := readHeaderValue(root, attrVersion, versionHeaderLabels, "ledger version")
	if err != nil {
		return "", err
	}
	if version == "" {
		return "", errorf(ErrMissingVersion, "ledger root %q has no %s", rootSelector, attrVersion[0])
	}
	status, err := readHeaderValue(root, attrStatus, statusHeaderLabels, "ledger status")
	if err != nil {
		return "", err
	}
	if status != "" && !strings.EqualFold(status, "completed") && !strings.EqualFold(status, "complete") {
		return "", errorf(ErrNotCompleted,
			"ledger version %q is %q, not completed; refusing to cost an in-flight version",
			version, status)
	}
	return version, nil
}

// readHeaderValue reads a value from an attribute on the element or one of its
// header descendants, falling back to parsing text such as "版本: X".
func readHeaderValue(root ElementReader, attributes, labels []string, label string) (string, error) {
	if value, ok, err := readPresent(root, attributes); err != nil {
		return "", errorf(ErrReadFailed, "could not read %s (%s): %s", label, attributes[0], err)
	} else if ok {
		return value, nil
	}
	headers := root.All("h1, h2, h3, h4, header, .ledger-meta, [data-version], [data-status]")
	count, err := headers.Count()
	if err != nil {
		return "", errorf(ErrReadFailed, "could not read %s: %s", label, err)
	}
	for index := 0; index < count; index++ {
		header := headers.At(index)
		if value, ok, err := readPresent(header, attributes); err != nil {
			return "", errorf(ErrReadFailed, "could not read %s (%s): %s", label, attributes[0], err)
		} else if ok {
			return value, nil
		}
		text, err := header.Text()
		if err != nil {
			return "", errorf(ErrReadFailed, "could not read %s text: %s", label, err)
		}
		if value := labelledValue(text, labels); value != "" {
			return value, nil
		}
	}
	return "", nil
}

// labelledValue extracts the value following a label in text such as
// "版本: 2026-11-20.1" or "ledger version 2026-11-20.1". The value is taken
// verbatim (it may itself contain dots or dashes), and only the separators
// between the label and the value are discarded.
func labelledValue(text string, labels []string) string {
	lower := strings.ToLower(text)
	for _, label := range labels {
		position := strings.Index(lower, strings.ToLower(label))
		if position < 0 {
			continue
		}
		remainder := text[position+len(label):]
		remainder = strings.TrimLeft(remainder, " \t\r\n:：=-")
		fields := strings.Fields(remainder)
		if len(fields) == 0 {
			continue
		}
		value := strings.Trim(fields[0], ",，;；")
		if value != "" {
			return value
		}
	}
	return ""
}

// readSectionRows reads one attribute-tagged section. The selector may match
// the row elements themselves or containers that hold <tr> rows; in both
// cases DOM order is preserved and the section's event type is applied.
func readSectionRows(page Page, selector string, eventType EventType, label string) ([]Attributes, string, error) {
	matches := page.All(selector)
	count, err := matches.Count()
	if err != nil {
		return nil, "", errorf(ErrReadFailed, "could not locate the %s section %q: %s", label, selector, err)
	}
	if count == 0 {
		return nil, "", nil
	}

	version := ""
	collected := []Attributes{}
	if isRowElement(matches.At(0)) {
		collected, err = readRows(matches)
		if err != nil {
			return nil, "", err
		}
	} else {
		for index := 0; index < count; index++ {
			container := matches.At(index)
			containerVersion, err := readAlias(container, attrVersion, label+" section version")
			if err != nil {
				return nil, "", err
			}
			if containerVersion != "" {
				if version == "" {
					version = containerVersion
				} else if version != containerVersion {
					return nil, "", errorf(ErrMixedVersion,
						"the %s section mixes ledger versions %q and %q", label, version, containerVersion)
				}
			}
			containerRows, err := readRows(container.All(rowSelector))
			if err != nil {
				return nil, "", err
			}
			collected = append(collected, containerRows...)
		}
	}
	if len(collected) == 0 {
		// Fall back to a header-driven table inside the section.
		tableRows, tableVersion, err := readSectionTable(page, selector+" table", eventType, label)
		if err != nil {
			return nil, "", err
		}
		if tableVersion != "" {
			version = tableVersion
		}
		return tableRows, version, nil
	}

	forced, rowVersion, err := forceSectionType(collected, eventType, label)
	if err != nil {
		return nil, "", err
	}
	if version == "" {
		version = rowVersion
	}
	return forced, version, nil
}

// readSectionTable reads one ordinary HTML table whose columns are identified
// by header cells, and applies the section's event type to every row.
func readSectionTable(page Page, selector string, eventType EventType, label string) ([]Attributes, string, error) {
	matches := page.All(selector)
	count, err := matches.Count()
	if err != nil {
		return nil, "", errorf(ErrReadFailed, "could not locate the %s table %q: %s", label, selector, err)
	}
	if count == 0 {
		return nil, "", nil
	}
	collected := []Attributes{}
	for index := 0; index < count; index++ {
		tableRows, err := readTableElement(matches.At(index))
		if err != nil {
			return nil, "", err
		}
		collected = append(collected, tableRows...)
	}
	return forceSectionType(collected, eventType, label)
}

// readTable reads every matched table by its header row.
func readTable(tables ElementReader) ([]Attributes, error) {
	count, err := tables.Count()
	if err != nil {
		return nil, errorf(ErrReadFailed, "could not locate ledger tables: %s", err)
	}
	collected := []Attributes{}
	for index := 0; index < count; index++ {
		tableRows, err := readTableElement(tables.At(index))
		if err != nil {
			return nil, err
		}
		collected = append(collected, tableRows...)
	}
	return collected, nil
}

// readTableElement reads one <table>: the header row names the columns and each
// body row becomes a ledger row.
func readTableElement(table ElementReader) ([]Attributes, error) {
	headers := table.All("thead tr th, thead tr td")
	headerCount, err := headers.Count()
	if err != nil {
		return nil, errorf(ErrReadFailed, "could not read table headers: %s", err)
	}
	if headerCount == 0 {
		// Some pages use the first row for headers without <thead>.
		headers = table.All("tr:first-child th, tr:first-child td")
		if headerCount, err = headers.Count(); err != nil {
			return nil, errorf(ErrReadFailed, "could not read table headers: %s", err)
		}
	}
	if headerCount == 0 {
		return nil, nil
	}

	fieldByColumn := map[int]string{}
	for index := 0; index < headerCount; index++ {
		label, err := headers.At(index).Text()
		if err != nil {
			return nil, errorf(ErrReadFailed, "could not read table header %d: %s", index+1, err)
		}
		if field := fieldForLabel(label); field != "" {
			fieldByColumn[index] = field
		}
	}
	if len(fieldByColumn) == 0 {
		return nil, nil
	}

	bodyRows := table.All("tbody tr")
	if bodyCount, err := bodyRows.Count(); err != nil {
		return nil, errorf(ErrReadFailed, "could not read table rows: %s", err)
	} else if bodyCount == 0 {
		bodyRows = table.All("tr")
	}

	rowCount, err := bodyRows.Count()
	if err != nil {
		return nil, errorf(ErrReadFailed, "could not count table rows: %s", err)
	}
	attributes := make([]Attributes, 0, rowCount)
	for rowIndex := 0; rowIndex < rowCount; rowIndex++ {
		cells := bodyRows.At(rowIndex).All("td")
		cellCount, err := cells.Count()
		if err != nil {
			return nil, errorf(ErrReadFailed, "could not read row %d cells: %s", rowIndex+1, err)
		}
		if cellCount == 0 {
			// The header row itself carries <th> cells, not <td> cells.
			continue
		}
		fields := map[string]Field{}
		present := map[string]bool{}
		for columnIndex := 0; columnIndex < cellCount; columnIndex++ {
			field, ok := fieldByColumn[columnIndex]
			if !ok {
				continue
			}
			text, err := cells.At(columnIndex).Text()
			if err != nil {
				return nil, errorf(ErrReadFailed, "could not read row %d cell %d: %s", rowIndex+1, columnIndex+1, err)
			}
			text = strings.TrimSpace(text)
			if field == attrAmount[0] {
				text = cleanCellAmount(text)
			}
			if field == attrType[0] {
				if eventType, recognized := NormalizeEventType(text); !recognized {
					return nil, errorf(ErrUnknownEventType,
						"row %d has unsupported event type %q", rowIndex+1, text)
				} else {
					text = string(eventType)
				}
			}
			if text != "" {
				fields[field] = Field{Name: field, Value: text}
				present[field] = true
			}
		}
		if len(fields) == 0 {
			// A row with no recognised cells is not a ledger row (for example,
			// a header row selected alongside the body rows). Skip it so an
			// empty table still falls through to the next reader path.
			continue
		}
		attributes = append(attributes, Attributes{Fields: fields, Present: present})
	}
	return attributes, nil
}

// fieldForLabel maps a header label to a ledger field, preferring an exact
// match so "金额分" is not mistaken for "金额".
func fieldForLabel(label string) string {
	normalized := normalizeLabel(label)
	for _, column := range tableColumns {
		for _, candidate := range column.labels {
			if normalizeLabel(candidate) == normalized {
				return column.field
			}
		}
	}
	for _, column := range tableColumns {
		for _, candidate := range column.labels {
			if strings.Contains(normalized, normalizeLabel(candidate)) {
				return column.field
			}
		}
	}
	return ""
}

func forceSectionType(rows []Attributes, eventType EventType, label string) ([]Attributes, string, error) {
	version := ""
	for _, row := range rows {
		if value, ok := row.Get(attrType[0]); ok {
			if EventType(strings.ToLower(value)) != eventType {
				return nil, "", errorf(ErrUnknownEventType,
					"a row in the %s section is tagged %q; a section cannot mix event types",
					label, value)
			}
		}
		row.Fields[attrType[0]] = Field{Name: attrType[0], Value: string(eventType)}
		row.Present[attrType[0]] = true
		if rowVersion, ok := row.Get(attrVersion[0]); ok && rowVersion != "" {
			if version == "" {
				version = rowVersion
			} else if version != rowVersion {
				return nil, "", errorf(ErrMixedVersion,
					"the %s section mixes ledger versions %q and %q", label, version, rowVersion)
			}
		}
	}
	return rows, version, nil
}

func isRowElement(element ElementReader) bool {
	for _, names := range [][]string{attrType, attrID} {
		if _, ok, err := readPresent(element, names); err == nil && ok {
			return true
		}
	}
	for _, name := range []string{"data-event", "row-id"} {
		if value, err := element.Attribute(name); err == nil && value != "" {
			return true
		}
	}
	return false
}

func readRows(rows ElementReader) ([]Attributes, error) {
	count, err := rows.Count()
	if err != nil {
		return nil, errorf(ErrReadFailed, "could not count ledger rows: %s", err)
	}
	attributeGroups := [][]string{
		attrVersion, attrType, attrID, attrSeq, attrWarehouse, attrSKU,
		attrProduct, attrCustomer, attrBatch, attrQuantity, attrAmount,
		attrAmountCent, attrReference,
	}
	attributes := make([]Attributes, 0, count)
	for index := 0; index < count; index++ {
		row := rows.At(index)
		fields := map[string]Field{}
		present := map[string]bool{}
		for _, names := range attributeGroups {
			value, ok, err := readPresent(row, names)
			if err != nil {
				return nil, errorf(ErrReadFailed,
					"could not read %s from ledger row %d: %s", names[0], index+1, err)
			}
			if ok {
				fields[names[0]] = Field{Name: names[0], Value: value}
				present[names[0]] = true
			}
		}
		attributes = append(attributes, Attributes{Fields: fields, Present: present})
	}
	return attributes, nil
}

// readAlias returns the first non-empty value among the aliases, or "" when
// the element has none of them.
func readAlias(element ElementReader, names []string, label string) (string, error) {
	value, _, err := readPresent(element, names)
	if err != nil {
		return "", errorf(ErrReadFailed, "could not read %s (%s): %s", label, names[0], err)
	}
	return value, nil
}

func readPresent(element ElementReader, names []string) (string, bool, error) {
	for _, name := range names {
		value, err := element.Attribute(name)
		if err != nil {
			return "", false, err
		}
		if value != "" {
			return value, true, nil
		}
	}
	return "", false, nil
}
