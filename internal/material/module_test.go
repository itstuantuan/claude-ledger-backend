package material

import "testing"

func TestValidateMaterialMoneyAndRequiredFields(t *testing.T) {
	valid := Input{Name: "材料", Category: "内墙漆", Brand: "品牌", Specification: "18L", Unit: "桶", DefaultPrice: "100.00", CostPrice: "80.00", Status: "ACTIVE"}
	if _, _, err := validate(valid); err != nil {
		t.Fatal(err)
	}
	for _, price := range []string{"1e3", "1.001", "-1", ""} {
		invalid := valid
		invalid.DefaultPrice = price
		if _, _, err := validate(invalid); err == nil {
			t.Fatalf("accepted %q", price)
		}
	}
}
