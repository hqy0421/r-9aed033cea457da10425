package stockledger

import (
	"sort"
	"strconv"
	"strings"
)

// Attribute names read by the ledger parser. Bracket entries are accepted
// aliases; the first name is what the report and errors use.
var (
	attrType       = []string{"data-type"}
	attrID         = []string{"data-id"}
	attrSeq        = []string{"data-seq", "data-sequence"}
	attrWarehouse  = []string{"data-warehouse"}
	attrSKU        = []string{"data-sku"}
	attrProduct    = []string{"data-product", "data-product-name"}
	attrCustomer   = []string{"data-customer"}
	attrBatch      = []string{"data-batch", "data-batch-id"}
	attrQuantity   = []string{"data-qty", "data-quantity"}
	attrAmount     = []string{"data-amount"}
	attrAmountCent = []string{"data-amount-cents", "data-cents"}
	attrReference  = []string{"data-ref", "data-sale-ref"}
	attrVersion    = []string{"data-version"}
	attrStatus     = []string{"data-status"}
)

// EventType is the kind of movement recorded in the ledger.
type EventType string

const (
	EventReceipt EventType = "receipt"
	EventSale    EventType = "sale"
	EventReturn  EventType = "return"
)

// Field identifies one parsed attribute in validation errors.
type Field struct {
	Name  string
	Value string
}

// Attributes is the raw attribute view of a single ledger row, with aliases
// already resolved. Present records whether any alias was found in the DOM.
type Attributes struct {
	Fields  map[string]Field
	Present map[string]bool
}

// Get returns the value of a canonical attribute and whether it was present.
func (a Attributes) Get(name string) (string, bool) {
	field, ok := a.Fields[name]
	return field.Value, ok
}

// Row is one ledger line read from the page. Rows are ordered on the page by
// DataSeq; the sequence number is the authoritative business order.
type Row struct {
	Index     int
	Type      EventType
	ID        string
	Seq       int64
	Warehouse string
	SKU       string
	Product   string
	Customer  string
	Batch     string
	Quantity  int64
	Cents     int64
	Reference string
	Version   string
}

// Ledger is the parsed result of reading one completed ledger version.
type Ledger struct {
	Version string
	Rows    []Row
}

// ParseRows converts raw DOM attributes into a validated, sequence-ordered
// Ledger. It rejects unknown event types, missing or malformed numbers, mixed
// ledger versions, duplicate sequence numbers, and the same event id appearing
// twice with different values.
//
// Parsing rows that were already parsed yields the same Ledger, so re-reading
// an unchanged page never double counts.
func ParseRows(version string, rows []Attributes) (*Ledger, error) {
	if version == "" {
		return nil, errorf(ErrMissingVersion, "the ledger page does not expose a ledger version")
	}
	ledger := &Ledger{Version: version}
	eventByID := map[string]Row{}
	eventBySeq := map[int64]Row{}

	for index, attributes := range rows {
		row, err := parseRow(index, attributes)
		if err != nil {
			return nil, err
		}
		if row.Version != "" && row.Version != version {
			return nil, errorf(ErrMixedVersion,
				"row %d (%s) belongs to ledger version %q, but the page reports %q",
				row.Index+1, row.ID, row.Version, version)
		}
		row.Version = version

		if previous, ok := eventByID[row.ID]; ok {
			if !sameEvent(previous, row) {
				return nil, errorf(ErrConflictingEvent,
					"event %s was read again with different values (sequence %d vs %d, %s %d vs %s %d cents)",
					row.ID, previous.Seq, row.Seq, previous.Type, previous.Cents, row.Type, row.Cents)
			}
			continue
		}
		if previous, ok := eventBySeq[row.Seq]; ok {
			return nil, errorf(ErrDuplicateSequence,
				"sequence %d is used by both %s and %s", row.Seq, previous.ID, row.ID)
		}
		eventByID[row.ID] = row
		eventBySeq[row.Seq] = row
		ledger.Rows = append(ledger.Rows, row)
	}

	sort.SliceStable(ledger.Rows, func(i, j int) bool {
		if ledger.Rows[i].Seq != ledger.Rows[j].Seq {
			return ledger.Rows[i].Seq < ledger.Rows[j].Seq
		}
		return ledger.Rows[i].ID < ledger.Rows[j].ID
	})
	return ledger, nil
}

func parseRow(index int, attributes Attributes) (Row, error) {
	row := Row{Index: index}
	rowVersion, _ := stringField(attributes, attrVersion)
	row.Version = rowVersion

	rawType, ok := stringField(attributes, attrType)
	if !ok {
		return Row{}, errorf(ErrMissingField, "row %d has no %s", index+1, attrType[0])
	}
	switch EventType(strings.ToLower(rawType)) {
	case EventReceipt, EventSale, EventReturn:
		row.Type = EventType(strings.ToLower(rawType))
	default:
		return Row{}, errorf(ErrUnknownEventType, "row %d has unsupported %s %q", index+1, attrType[0], rawType)
	}

	row.ID, ok = stringField(attributes, attrID)
	if !ok {
		return Row{}, errorf(ErrMissingField, "%s row %d has no %s", row.Type, index+1, attrID[0])
	}

	rawSeq, ok := stringField(attributes, attrSeq)
	if !ok {
		return Row{}, errorf(ErrMissingField, "%s %s has no %s", row.Type, row.ID, attrSeq[0])
	}
	var err error
	if row.Seq, err = ParseSequence(rawSeq); err != nil {
		return Row{}, errorf(ErrInvalidNumber, "%s %s: %s", row.Type, row.ID, err)
	}

	if row.Warehouse, ok = stringField(attributes, attrWarehouse); !ok {
		return Row{}, errorf(ErrMissingField, "%s %s has no %s", row.Type, row.ID, attrWarehouse[0])
	}
	if row.SKU, ok = stringField(attributes, attrSKU); !ok {
		return Row{}, errorf(ErrMissingField, "%s %s has no %s", row.Type, row.ID, attrSKU[0])
	}
	row.Product, _ = stringField(attributes, attrProduct)
	row.Customer, _ = stringField(attributes, attrCustomer)

	rawQuantity, ok := stringField(attributes, attrQuantity)
	if !ok {
		return Row{}, errorf(ErrMissingField, "%s %s has no %s", row.Type, row.ID, attrQuantity[0])
	}
	if row.Quantity, err = ParseQuantity(rawQuantity); err != nil {
		return Row{}, errorf(ErrInvalidNumber, "%s %s: %s", row.Type, row.ID, err)
	}
	if row.Quantity == 0 {
		return Row{}, errorf(ErrInvalidNumber, "%s %s has zero quantity", row.Type, row.ID)
	}
	if row.Cents, err = parseAmount(attributes, row.Type, row.ID); err != nil {
		return Row{}, err
	}

	row.Batch, ok = stringField(attributes, attrBatch)
	if ok && row.Type != EventReceipt {
		return Row{}, errorf(ErrUnexpectedBatch,
			"%s %s names batch %q; only receipts own a batch", row.Type, row.ID, row.Batch)
	}
	if !ok && row.Type == EventReceipt {
		return Row{}, errorf(ErrMissingField, "receipt %s has no %s", row.ID, attrBatch[0])
	}

	row.Reference, ok = stringField(attributes, attrReference)
	if row.Type == EventReturn {
		if !ok {
			return Row{}, errorf(ErrMissingReference, "return %s has no %s", row.ID, attrReference[0])
		}
	} else if ok {
		return Row{}, errorf(ErrUnexpectedReference,
			"%s %s references sale %q; only returns reference a sale", row.Type, row.ID, row.Reference)
	}

	if row.Type == EventReceipt && row.Customer != "" {
		return Row{}, errorf(ErrMissingField, "receipt %s must not name a customer", row.ID)
	}
	return row, nil
}

func parseAmount(attributes Attributes, eventType EventType, id string) (int64, error) {
	rawAmount, hasAmount := stringField(attributes, attrAmount)
	rawCents, hasCents := stringField(attributes, attrAmountCent)
	if !hasAmount && !hasCents {
		return 0, errorf(ErrMissingField, "%s %s has no %s", eventType, id, attrAmount[0])
	}

	var amountCents, centsCents int64
	var err error
	if hasAmount {
		if amountCents, err = ParseCents(rawAmount); err != nil {
			return 0, errorf(ErrInvalidNumber, "%s %s: %s", eventType, id, err)
		}
	}
	if hasCents {
		if centsCents, err = ParseCentCount(rawCents); err != nil {
			return 0, errorf(ErrInvalidNumber, "%s %s: %s", eventType, id, err)
		}
	}
	if hasAmount && hasCents && amountCents != centsCents {
		return 0, errorf(ErrConflictingAmount,
			"%s %s reports %s (%d cents) and %s (%d cents)",
			eventType, id, rawAmount, amountCents, rawCents, centsCents)
	}
	if hasAmount {
		return amountCents, nil
	}
	return centsCents, nil
}

func stringField(attributes Attributes, names []string) (string, bool) {
	for _, name := range names {
		if value, ok := attributes.Get(name); ok {
			value = strings.TrimSpace(value)
			if value != "" {
				return value, true
			}
			return "", false
		}
	}
	return "", false
}

func sameEvent(left, right Row) bool {
	return left.Seq == right.Seq &&
		left.Type == right.Type &&
		left.Warehouse == right.Warehouse &&
		left.SKU == right.SKU &&
		left.Product == right.Product &&
		left.Customer == right.Customer &&
		left.Batch == right.Batch &&
		left.Quantity == right.Quantity &&
		left.Cents == right.Cents &&
		left.Reference == right.Reference
}

// RowKey identifies an event for diagnostics and idempotent re-reads.
func (r Row) RowKey() string {
	return strconv.FormatInt(r.Seq, 10) + ":" + r.ID
}
