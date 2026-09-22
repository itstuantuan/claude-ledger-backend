package order

import (
	"errors"
	"regexp"

	"cloud-ledger-backend/internal/platform/apperror"
	"github.com/shopspring/decimal"
)

var moneyPattern = regexp.MustCompile(`^\d+(?:\.\d{1,2})?$`)
var quantityPattern = regexp.MustCompile(`^\d+(?:\.\d{1,3})?$`)

type CalculatedLine struct {
	MaterialID                                     string
	Quantity, UnitPrice, Discount, Gross, Subtotal decimal.Decimal
}
type Calculation struct {
	Lines                                                []CalculatedLine
	Goods, Discount, Final, Payment, Prepaid, Receivable decimal.Decimal
}

func Calculate(input Input) (Calculation, error) {
	if len(input.Items) == 0 {
		return Calculation{}, apperror.Validation(map[string][]string{"items": {"至少添加一种材料"}})
	}
	payment, err := parseMoney(input.PaymentAmount)
	if err != nil {
		return Calculation{}, apperror.Validation(map[string][]string{"paymentAmount": {"金额格式不正确"}})
	}
	prepaid, err := parseMoney(input.PrepaidDeduction)
	if err != nil {
		return Calculation{}, apperror.Validation(map[string][]string{"prepaidDeduction": {"金额格式不正确"}})
	}
	if payment.IsPositive() && (input.PaymentMethod == nil || *input.PaymentMethod == "") {
		return Calculation{}, apperror.Validation(map[string][]string{"paymentMethod": {"请选择付款方式"}})
	}
	if input.PaymentMethod != nil && *input.PaymentMethod != "" {
		valid := map[string]bool{"WECHAT": true, "ALIPAY": true, "CASH": true, "BANK_CARD": true, "OTHER": true}
		if !valid[*input.PaymentMethod] {
			return Calculation{}, apperror.Validation(map[string][]string{"paymentMethod": {"付款方式无效"}})
		}
	}
	if len([]rune(input.Note)) > 200 {
		return Calculation{}, apperror.Validation(map[string][]string{"note": {"备注不能超过 200 个字符"}})
	}
	result := Calculation{Payment: payment, Prepaid: prepaid, Lines: make([]CalculatedLine, len(input.Items))}
	seen := map[string]bool{}
	for i, line := range input.Items {
		if seen[line.MaterialID] {
			return Calculation{}, apperror.Validation(map[string][]string{"items": {"同一种材料不能重复添加"}})
		}
		seen[line.MaterialID] = true
		q, e := parseQuantity(line.Quantity)
		if e != nil || !q.IsPositive() {
			return Calculation{}, apperror.Validation(map[string][]string{"items": {"数量格式不正确"}})
		}
		price, e := parseMoney(line.UnitPrice)
		if e != nil {
			return Calculation{}, apperror.Validation(map[string][]string{"items": {"单价格式不正确"}})
		}
		discount, e := parseMoney(line.Discount)
		if e != nil {
			return Calculation{}, apperror.Validation(map[string][]string{"items": {"优惠格式不正确"}})
		}
		gross := price.Mul(q).Round(2)
		if discount.GreaterThan(gross) {
			return Calculation{}, apperror.New(422, "DISCOUNT_EXCEEDED", "优惠不能超过商品金额。")
		}
		subtotal := gross.Sub(discount)
		result.Lines[i] = CalculatedLine{MaterialID: line.MaterialID, Quantity: q, UnitPrice: price, Discount: discount, Gross: gross, Subtotal: subtotal}
		result.Goods = result.Goods.Add(gross)
		result.Discount = result.Discount.Add(discount)
	}
	result.Final = result.Goods.Sub(result.Discount)
	if !result.Final.IsPositive() {
		return Calculation{}, apperror.New(422, "ORDER_AMOUNT_ZERO", "用料单金额必须大于零。")
	}
	if payment.Add(prepaid).GreaterThan(result.Final) {
		return Calculation{}, apperror.New(422, "SETTLEMENT_EXCEEDED", "付款和预存抵扣不能超过应收金额。")
	}
	result.Receivable = result.Final.Sub(payment).Sub(prepaid)
	return result, nil
}
func parseMoney(v string) (decimal.Decimal, error) {
	if !moneyPattern.MatchString(v) {
		return decimal.Zero, errors.New("invalid money")
	}
	return decimal.NewFromString(v)
}
func parseQuantity(v string) (decimal.Decimal, error) {
	if !quantityPattern.MatchString(v) {
		return decimal.Zero, errors.New("invalid quantity")
	}
	return decimal.NewFromString(v)
}
