package stockledger_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestStockLedger(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Stock Ledger Suite")
}
