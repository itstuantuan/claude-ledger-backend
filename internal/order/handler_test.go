package order

import "testing"

func TestValidDateRange(t *testing.T) {
	for _, v := range []struct {
		from, to string
		valid    bool
	}{{"2026-09-01", "2026-09-30", true}, {"", "", true}, {"2026-10-01", "2026-09-01", false}, {"bad", "2026-09-01", false}} {
		if got := validDateRange(v.from, v.to); got != v.valid {
			t.Fatalf("%+v got %v", v, got)
		}
	}
}
