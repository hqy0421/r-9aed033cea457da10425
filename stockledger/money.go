package stockledger

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

const (
	maxQuantity = int64(1) << 40
	maxAmount   = int64(1) << 50
)

// ParseCents converts a decimal money amount into integer cents ("分").
//
// Accepted forms: "12", "12.3", "12.30", "+12.30", and amounts with thousands
// separators such as "1,234.56". At most two decimal places are accepted, no
// rounding is performed, and negative amounts are rejected. The empty string,
// exponent notation, currency symbols, and unseparated spaces are rejected.
func ParseCents(value string) (int64, error) {
	text := strings.TrimSpace(value)
	if text == "" {
		return 0, fmt.Errorf("empty amount")
	}
	text = strings.Replace(text, ",", "", -1)
	if strings.HasPrefix(text, "+") {
		text = text[1:]
	}
	if strings.HasPrefix(text, "-") {
		return 0, fmt.Errorf("amount %q is negative", value)
	}

	whole, fraction := text, ""
	if dot := strings.IndexByte(text, '.'); dot >= 0 {
		whole, fraction = text[:dot], text[dot+1:]
		if strings.IndexByte(fraction, '.') >= 0 {
			return 0, fmt.Errorf("amount %q has more than one decimal point", value)
		}
	}
	if whole == "" && fraction == "" {
		return 0, fmt.Errorf("amount %q has no digits", value)
	}
	if !allDigits(whole) || !allDigits(fraction) {
		return 0, fmt.Errorf("amount %q is not a decimal number", value)
	}
	if len(fraction) > 2 {
		return 0, fmt.Errorf("amount %q has more than two decimal places", value)
	}
	for len(fraction) < 2 {
		fraction += "0"
	}
	if whole == "" {
		whole = "0"
	}

	digits := strings.TrimLeft(whole+fraction, "0")
	if digits == "" {
		return 0, nil
	}
	if len(digits) > 18 {
		return 0, fmt.Errorf("amount %q is too large", value)
	}
	cents, err := strconv.ParseInt(whole+fraction, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("amount %q is not representable: %s", value, err)
	}
	if cents > maxAmount {
		return 0, fmt.Errorf("amount %q is too large", value)
	}
	return cents, nil
}

// ParseCentCount converts an attribute that already expresses a value in
// cents (for example data-amount-cents="1230") into an int64. Unlike
// ParseCents it does not scale by 100, so decimals are rejected.
func ParseCentCount(value string) (int64, error) {
	text := strings.Replace(strings.TrimSpace(value), ",", "", -1)
	if strings.HasPrefix(text, "+") {
		text = text[1:]
	}
	if strings.HasPrefix(text, "-") {
		return 0, fmt.Errorf("amount %q is negative", value)
	}
	if text == "" {
		return 0, fmt.Errorf("empty amount")
	}
	if !allDigits(text) {
		return 0, fmt.Errorf("cent amount %q is not a whole number of cents", value)
	}
	if len(strings.TrimLeft(text, "0")) > 18 {
		return 0, fmt.Errorf("amount %q is too large", value)
	}
	cents, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("amount %q is not representable: %s", value, err)
	}
	if cents > maxAmount {
		return 0, fmt.Errorf("amount %q is too large", value)
	}
	return cents, nil
}

// ParseQuantity converts a ledger quantity into an integer. Only whole,
// non-negative quantities are accepted; separators and a leading "+" are
// tolerated, everything else is rejected.
func ParseQuantity(value string) (int64, error) {
	text := strings.Replace(strings.TrimSpace(value), ",", "", -1)
	if strings.HasPrefix(text, "+") {
		text = text[1:]
	}
	if text == "" {
		return 0, fmt.Errorf("empty quantity")
	}
	if !allDigits(text) {
		return 0, fmt.Errorf("quantity %q is not a whole non-negative number", value)
	}
	if len(strings.TrimLeft(text, "0")) > 13 {
		return 0, fmt.Errorf("quantity %q is too large", value)
	}
	quantity, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("quantity %q is not representable: %s", value, err)
	}
	if quantity > maxQuantity {
		return 0, fmt.Errorf("quantity %q is too large", value)
	}
	return quantity, nil
}

// ParseSequence converts a business sequence number. Sequences are positive
// integers; the same textual rules as ParseQuantity apply.
func ParseSequence(value string) (int64, error) {
	seq, err := ParseQuantity(value)
	if err != nil {
		return 0, fmt.Errorf("sequence %q is invalid: %s", value, err)
	}
	if seq == 0 {
		return 0, fmt.Errorf("sequence %q must be positive", value)
	}
	return seq, nil
}

// FormatCents renders integer cents as a decimal string with exactly two
// decimal places, e.g. -1234 -> "-12.34".
func FormatCents(cents int64) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	return fmt.Sprintf("%s%d.%02d", sign, cents/100, cents%100)
}

func allDigits(text string) bool {
	for i := 0; i < len(text); i++ {
		if text[i] < '0' || text[i] > '9' {
			return false
		}
	}
	return true
}

// mulDiv returns floor(a*b/c) for non-negative a, b and positive c without
// overflowing int64. It is used for the exact cent split of a receipt batch.
func mulDiv(a, b, c int64) int64 {
	if a == 0 || b == 0 {
		return 0
	}
	result := new(big.Int).Mul(big.NewInt(a), big.NewInt(b))
	result.Quo(result, big.NewInt(c))
	return result.Int64()
}
