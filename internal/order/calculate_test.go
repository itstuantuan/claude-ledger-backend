package order

import (
	"cloud-ledger-backend/internal/platform/apperror"
	"errors"
	"testing"
	"time"
)

func TestCalculateMixedSettlementExactly(t *testing.T) {
	method := "WECHAT"
	c, err := Calculate(Input{Items: []ItemInput{{MaterialID: "m1", Quantity: "3", UnitPrice: "1000.00", Discount: "0.00"}}, PaymentAmount: "1000.00", PrepaidDeduction: "500.00", PaymentMethod: &method})
	if err != nil {
		t.Fatal(err)
	}
	if c.Final.StringFixed(2) != "3000.00" || c.Receivable.StringFixed(2) != "1500.00" {
		t.Fatalf("unexpected: %+v", c)
	}
	ledgerNet := c.Final.Sub(c.Payment).Sub(c.Prepaid)
	if !ledgerNet.Equal(c.Receivable) {
		t.Fatalf("ledger invariant failed: %s != %s", ledgerNet, c.Receivable)
	}
	if c.Payment.StringFixed(2) != "1000.00" {
		t.Fatal("financial transaction must contain only real payment")
	}
}

func TestParseOccurredUsesStoreTimezoneForBusinessDate(t *testing.T) {
	occurred, businessDay, err := parseOccurred("2026-09-22", "Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	if got := occurred.Format(time.RFC3339); got != "2026-09-21T16:00:00Z" {
		t.Fatalf("unexpected UTC occurrence: %s", got)
	}
	if got := businessDay.Format("2006-01-02"); got != "2026-09-22" {
		t.Fatalf("unexpected business day: %s", got)
	}

	occurred, businessDay, err = parseOccurred("2026-09-21T17:30:00Z", "Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	if got := occurred.Format(time.RFC3339); got != "2026-09-21T17:30:00Z" {
		t.Fatalf("unexpected preserved instant: %s", got)
	}
	if got := businessDay.Format("2006-01-02"); got != "2026-09-22" {
		t.Fatalf("unexpected local business day: %s", got)
	}
}
func TestCalculateRoundsAtLineAndRejectsExcess(t *testing.T) {
	c, err := Calculate(Input{Items: []ItemInput{{MaterialID: "m1", Quantity: "0.333", UnitPrice: "10.00", Discount: "0.01"}}, PaymentAmount: "0.00", PrepaidDeduction: "0.00"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Goods.StringFixed(2) != "3.33" || c.Final.StringFixed(2) != "3.32" {
		t.Fatalf("unexpected amounts %+v", c)
	}
	_, err = Calculate(Input{Items: []ItemInput{{MaterialID: "m1", Quantity: "1", UnitPrice: "1.00", Discount: "2.00"}}, PaymentAmount: "0.00", PrepaidDeduction: "0.00"})
	var app *apperror.Error
	if !errors.As(err, &app) || app.Code != "DISCOUNT_EXCEEDED" {
		t.Fatalf("unexpected %v", err)
	}
}

func TestCalculateRejectsZeroOrderAndUnknownPaymentMethod(t *testing.T) {
	_, err := Calculate(Input{Items: []ItemInput{{MaterialID: "m1", Quantity: "1", UnitPrice: "1.00", Discount: "1.00"}}, PaymentAmount: "0.00", PrepaidDeduction: "0.00"})
	var app *apperror.Error
	if !errors.As(err, &app) || app.Code != "ORDER_AMOUNT_ZERO" {
		t.Fatalf("unexpected %v", err)
	}
	method := "CRYPTO"
	_, err = Calculate(Input{Items: []ItemInput{{MaterialID: "m1", Quantity: "1", UnitPrice: "1.00", Discount: "0.00"}}, PaymentAmount: "1.00", PrepaidDeduction: "0.00", PaymentMethod: &method})
	if err == nil {
		t.Fatal("unknown payment method accepted")
	}
}
