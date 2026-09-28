package pricing

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPricingHistoryConcurrencyAndReplay(t *testing.T) {
	dsn := os.Getenv("PRICING_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PRICING_TEST_DATABASE_URL is not set")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	ids := map[string]uuid.UUID{}
	for name, query := range map[string]string{
		"store":    "SELECT id::text FROM stores ORDER BY created_at LIMIT 1",
		"user":     "SELECT id::text FROM users ORDER BY created_at LIMIT 1",
		"worker":   "SELECT id::text FROM workers WHERE status='ACTIVE' ORDER BY created_at LIMIT 1",
		"material": "SELECT id::text FROM materials WHERE status='ACTIVE' ORDER BY created_at LIMIT 1",
	} {
		var raw string
		if err = db.Raw(query).Scan(&raw).Error; err != nil {
			t.Fatal(err)
		}
		ids[name] = uuid.MustParse(raw)
	}
	storeID, userID, workerID, materialID := ids["store"], ids["user"], ids["worker"], ids["material"]
	if err = db.Where("store_id=? AND worker_id=? AND material_id=?", storeID, workerID, materialID).Delete(&Model{}).Error; err != nil {
		t.Fatal(err)
	}

	firstAt := time.Now().UTC().Truncate(time.Second)
	service := &Service{db: db, now: func() time.Time { return firstAt }}
	initial, err := service.List(ctx, storeID, workerID, firstAt.Add(-time.Hour).Format(time.RFC3339), "")
	if err != nil {
		t.Fatal(err)
	}
	var initialItem Item
	for _, item := range initial {
		if item.MaterialID == materialID.String() {
			initialItem = item
		}
	}
	input := BatchInput{WorkerID: workerID.String(), Prices: []PriceInput{{MaterialID: materialID.String(), Price: stringPointer("123.45"), ExpectedVersion: initialItem.PriceVersion}}}
	meta := saveMeta{StoreID: storeID, UserID: userID, Key: "pricing-integration-" + uuid.NewString(), RequestID: uuid.NewString(), IP: "127.0.0.1"}
	current, replayed, err := service.Save(ctx, meta, input)
	if err != nil {
		t.Fatal(err)
	}
	if replayed {
		t.Fatal("first save was reported as replay")
	}
	assertPrice(t, current, materialID, "123.45")
	replayedItems, replayed, err := service.Save(ctx, meta, input)
	if err != nil || !replayed {
		t.Fatalf("replay failed: replayed=%v err=%v", replayed, err)
	}
	assertPrice(t, replayedItems, materialID, "123.45")

	old, err := service.List(ctx, storeID, workerID, firstAt.Add(-time.Minute).Format(time.RFC3339), "")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range old {
		if item.MaterialID == materialID.String() && item.CustomerPrice != initialItem.CustomerPrice {
			t.Fatalf("historical price changed: got %v want %v", item.CustomerPrice, initialItem.CustomerPrice)
		}
	}
	stale := input
	stale.Prices[0].Price = stringPointer("222.22")
	staleMeta := meta
	staleMeta.Key = "pricing-integration-" + uuid.NewString()
	if _, _, err = service.Save(ctx, staleMeta, stale); err == nil {
		t.Fatal("stale version was accepted")
	}
}

func stringPointer(value string) *string { return &value }
func assertPrice(t *testing.T, items []Item, materialID uuid.UUID, want string) {
	t.Helper()
	for _, item := range items {
		if item.MaterialID == materialID.String() {
			if item.CustomerPrice == nil || *item.CustomerPrice != want {
				t.Fatalf("price=%v want=%s", item.CustomerPrice, want)
			}
			return
		}
	}
	t.Fatal("material missing from pricing response")
}
