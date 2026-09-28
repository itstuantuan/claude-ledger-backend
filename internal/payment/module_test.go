package payment

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func TestParseOccurredAcceptsDatetimeLocal(t *testing.T) {
	occurred, businessDay, err := parseOccurred("2026-09-28T12:30", "Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	if got := occurred.Format(time.RFC3339); got != "2026-09-28T04:30:00Z" {
		t.Fatalf("unexpected occurrence: %s", got)
	}
	if got := businessDay.Format(time.DateOnly); got != "2026-09-28" {
		t.Fatalf("unexpected business day: %s", got)
	}
}

func TestPlanAllocationsFIFO(t *testing.T) {
	first, second := uuid.New(), uuid.New()
	plans, remaining := planAllocations([]orderModel{
		{ID: first, OrderNo: "YL1", OutstandingAmount: decimal.RequireFromString("30.00")},
		{ID: second, OrderNo: "YL2", OutstandingAmount: decimal.RequireFromString("50.00")},
	}, decimal.RequireFromString("60.00"))
	if len(plans) != 2 || plans[0].OrderID != first || !plans[0].Amount.Equal(decimal.NewFromInt(30)) || plans[1].OrderID != second || !plans[1].Amount.Equal(decimal.NewFromInt(30)) || !remaining.IsZero() {
		t.Fatalf("unexpected FIFO allocation: %#v remaining=%s", plans, remaining)
	}
}

func TestPlanSpecifiedAllocation(t *testing.T) {
	first, second := uuid.New(), uuid.New()
	orders := []orderModel{
		{ID: first, OrderNo: "YL1", OutstandingAmount: decimal.RequireFromString("30.00")},
		{ID: second, OrderNo: "YL2", OutstandingAmount: decimal.RequireFromString("50.00")},
	}
	plans, remaining, err := planSpecifiedAllocations(orders, []AllocationInput{{OrderID: second.String(), Amount: "20.00"}}, decimal.RequireFromString("20.00"))
	if err != nil || len(plans) != 1 || plans[0].OrderID != second || !remaining.IsZero() {
		t.Fatalf("unexpected specified allocation: %#v remaining=%s err=%v", plans, remaining, err)
	}
	if _, _, err = planSpecifiedAllocations(orders, []AllocationInput{{OrderID: first.String(), Amount: "31.00"}}, decimal.RequireFromString("31.00")); err == nil {
		t.Fatal("expected over-allocation error")
	}
}
