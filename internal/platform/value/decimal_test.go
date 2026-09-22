package value

import (
	"encoding/json"
	"testing"
)

func TestMoneyRequiresDecimalString(t *testing.T) {
	for _, valid := range []string{`"0"`, `"0.1"`, `"999999999999999999.99"`, `"-1.00"`} {
		var value Money
		if err := json.Unmarshal([]byte(valid), &value); err != nil {
			t.Fatalf("%s should be valid: %v", valid, err)
		}
	}
	for _, invalid := range []string{`0.1`, `"1e3"`, `"0.001"`, `"1,000"`, `""`} {
		var value Money
		if err := json.Unmarshal([]byte(invalid), &value); err == nil {
			t.Fatalf("%s should be invalid", invalid)
		}
	}
}

func TestQuantityPrecision(t *testing.T) {
	var quantity Quantity
	if err := json.Unmarshal([]byte(`"1.125"`), &quantity); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`"1.1251"`), &quantity); err == nil {
		t.Fatal("expected excess precision to fail")
	}
}
