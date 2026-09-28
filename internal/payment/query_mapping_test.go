package payment

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm/schema"
)

func TestPaymentQueryMapsPaymentColumns(t *testing.T) {
	model, err := schema.Parse(&queryRow{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatal(err)
	}
	id, workerID := uuid.New(), uuid.New()
	occurred := time.Date(2026, 9, 28, 4, 30, 0, 0, time.UTC)
	values := map[string]any{
		"id": id, "worker_id": workerID, "payment_no": "SK202609280001",
		"amount": decimal.RequireFromString("28.00"), "payment_method": "CASH",
		"occurred_at": occurred, "created_at": occurred, "remark": "test",
		"worker_name": "worker", "operator_name": "admin",
		"receivable_after": decimal.RequireFromString("2248.00"),
	}
	var row queryRow
	for column, value := range values {
		field := model.LookUpField(column)
		if field == nil {
			t.Errorf("payment query does not map column %s", column)
			continue
		}
		if err := field.Set(context.Background(), reflect.ValueOf(&row).Elem(), value); err != nil {
			t.Errorf("setting %s: %v", column, err)
		}
	}
	if t.Failed() {
		t.FailNow()
	}
	if row.ID != id || row.WorkerID != workerID || row.PaymentNo != "SK202609280001" || row.PaymentMethod != "CASH" || row.Amount.StringFixed(2) != "28.00" || !row.OccurredAt.Equal(occurred) || !row.CreatedAt.Equal(occurred) || row.Remark != "test" {
		t.Fatalf("payment fields were not populated: %+v", row)
	}
}
