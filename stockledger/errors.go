package stockledger

import "fmt"

// Error codes returned by ReadLedger, Compute, and WriteReport. They are
// stable identifiers so callers can react to a specific failure without
// matching on human-readable text.
const (
	ErrReadFailed          = "read_failed"            // the page could not be read
	ErrMissingRoot         = "missing_root"           // ledger root element not found
	ErrMissingVersion      = "missing_version"        // ledger version is absent
	ErrMixedVersion        = "mixed_version"          // rows belong to different versions
	ErrNotCompleted        = "not_completed"          // ledger version is not completed
	ErrUnknownEventType    = "unknown_event_type"     // data-type is not receipt/sale/return
	ErrMissingField        = "missing_field"          // a required attribute is empty
	ErrInvalidNumber       = "invalid_number"         // amount/quantity/sequence cannot be parsed
	ErrConflictingAmount   = "conflicting_amount"     // amount and amount-cents disagree
	ErrDuplicateSequence   = "duplicate_sequence"     // two different events share a sequence
	ErrConflictingEvent    = "conflicting_event"      // same event id re-read with different values
	ErrDuplicateBatch      = "duplicate_batch"        // receipt batch id reused for another receipt
	ErrUnexpectedBatch     = "unexpected_batch"       // sale/return carries a batch
	ErrUnexpectedReference = "unexpected_reference"   // receipt/sale carries a return reference
	ErrMissingReference    = "missing_reference"      // return has no original sale reference
	ErrUnknownReference    = "unknown_reference"      // return references no known sale
	ErrReferenceNotSale    = "reference_not_sale"     // return references something that is not a sale
	ErrOutOfOrderReference = "out_of_order_reference" // return precedes its original sale
	ErrWarehouseMismatch   = "warehouse_mismatch"     // return/sale link crosses warehouses
	ErrSKUMismatch         = "sku_mismatch"           // return/sale link crosses SKUs
	ErrCustomerMismatch    = "customer_mismatch"      // return customer differs from the sale
	ErrInsufficientStock   = "insufficient_stock"     // sale exceeds available batch quantity
	ErrExcessReturn        = "excess_return"          // return exceeds the sale's unreturned quantity
	ErrOverflow            = "amount_overflow"        // totals are not representable
	ErrWriteFailed         = "write_failed"           // the report could not be written
)

// Error is the error type returned for ledger validation and accounting
// failures. Code is one of the Err* constants.
type Error struct {
	Code   string
	Detail string
}

func (e *Error) Error() string {
	if e.Detail == "" {
		return e.Code
	}
	return e.Code + ": " + e.Detail
}

func errorf(code, format string, args ...interface{}) *Error {
	return &Error{Code: code, Detail: fmt.Sprintf(format, args...)}
}
