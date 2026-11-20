package stockledger

import (
	"strings"
)

// column lists the header labels that map to one ledger field. Labels are
// compared after normalisation (case-folded, spaces and punctuation removed),
// so "商品编码", "SKU", and "sku_code" all match the same column.
type column struct {
	field  string
	labels []string
}

var tableColumns = []column{
	{attrType[0], []string{"类型", "单据类型", "业务类型", "流水类型", "type", "event", "eventtype", "kind"}},
	{attrID[0], []string{"单号", "单据号", "单据编号", "流水号", "编号", "id", "no", "number", "document", "documentid", "eventid"}},
	{attrSeq[0], []string{"序号", "顺序", "顺序号", "业务序号", "seq", "sequence", "seqno", "order"}},
	{attrWarehouse[0], []string{"仓库", "仓位", "库房", "warehouse", "location", "store"}},
	{attrSKU[0], []string{"商品编码", "商品编号", "商品代码", "货号", "sku", "skucode", "itemcode", "code"}},
	{attrProduct[0], []string{"商品", "商品名称", "品名", "产品", "product", "productname", "item", "itemname", "name"}},
	{attrCustomer[0], []string{"客户", "顾客", "客户名称", "customer", "client", "buyer"}},
	{attrBatch[0], []string{"批次", "批次号", "批号", "lot", "lotno", "batch", "batchno", "batchid"}},
	{attrQuantity[0], []string{"数量", "件数", "数目", "qty", "quantity", "units", "count"}},
	{attrAmount[0], []string{"金额", "金额元", "成本", "成本金额", "售价", "销售金额", "售价元", "销售价", "amount", "total", "price", "revenue", "cost"}},
	{attrAmountCent[0], []string{"金额分", "分", "amountcents", "cents", "centamount"}},
	{attrReference[0], []string{"原单号", "原销售单号", "原销售", "关联单号", "退货引用", "ref", "reference", "original", "saleid", "saleref"}},
	{attrVersion[0], []string{"版本", "版本号", "版次", "version", "ledgerversion"}},
}

var eventTypeAliases = map[string]EventType{
	"receipt": EventReceipt, "receipts": EventReceipt, "receive": EventReceipt,
	"received": EventReceipt, "grn": EventReceipt, "inbound": EventReceipt,
	"in": EventReceipt, "purchase": EventReceipt, "收货": EventReceipt,
	"入库": EventReceipt, "采购": EventReceipt, "采购入库": EventReceipt,
	"采购收货": EventReceipt, "收货入库": EventReceipt,

	"sale": EventSale, "sales": EventSale, "sold": EventSale,
	"outbound": EventSale, "out": EventSale, "invoice": EventSale,
	"delivery": EventSale, "shipment": EventSale, "销售": EventSale,
	"出库": EventSale, "销售出库": EventSale, "发货": EventSale, "发货出库": EventSale,

	"return": EventReturn, "returns": EventReturn, "refund": EventReturn,
	"rma": EventReturn, "reversal": EventReturn, "returned": EventReturn,
	"退货": EventReturn, "销售退货": EventReturn, "退回": EventReturn, "退": EventReturn,
}

// NormalizeEventType maps a page's event type label to a ledger event type.
// It accepts the canonical English names and the Chinese labels used by
// commerce pages; anything else is rejected.
func NormalizeEventType(value string) (EventType, bool) {
	normalized := normalizeLabel(value)
	if eventType, ok := eventTypeAliases[normalized]; ok {
		return eventType, true
	}
	return "", false
}

// normalizeLabel folds a header or type label so small differences in spacing,
// case, and separators do not matter.
func normalizeLabel(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(
		" ", "", "\t", "", "\n", "", "\r", "",
		"_", "", "-", "", "/", "", ":", "", "：", "",
		"(", "", ")", "", "（", "", "）", "", ".", "",
		"*", "", "￥", "", "¥", "", "$", "",
	)
	return replacer.Replace(value)
}

// cleanCellAmount removes common presentation decorations from a money cell so
// the plain decimal can be parsed: currency symbols, the 元 suffix, spaces, and
// a sign. The sign carries no information because the event type determines it.
func cleanCellAmount(value string) string {
	cleaned := strings.TrimSpace(value)
	replacer := strings.NewReplacer(
		"￥", "", "¥", "", "$", "", "RMB", "", "rmb", "", "元", "", " ", "",
	)
	cleaned = replacer.Replace(cleaned)
	cleaned = strings.TrimPrefix(cleaned, "+")
	cleaned = strings.TrimPrefix(cleaned, "-")
	return cleaned
}
