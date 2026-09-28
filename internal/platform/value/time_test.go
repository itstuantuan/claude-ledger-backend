package value

import (
	"testing"
	"time"
)

func TestParseBusinessTimeSupportsFrontendAndAPIFormats(t *testing.T) {
	tests := []struct {
		name, input, occurred, day string
	}{
		{"date", "2026-09-28", "2026-09-27T16:00:00Z", "2026-09-28"},
		{"datetime local minutes", "2026-09-28T12:30", "2026-09-28T04:30:00Z", "2026-09-28"},
		{"datetime local seconds", "2026-09-28T12:30:45", "2026-09-28T04:30:45Z", "2026-09-28"},
		{"RFC3339", "2026-09-27T17:30:00Z", "2026-09-27T17:30:00Z", "2026-09-28"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			occurred, day, err := ParseBusinessTime(test.input, "Asia/Shanghai")
			if err != nil {
				t.Fatal(err)
			}
			if got := occurred.Format(time.RFC3339); got != test.occurred {
				t.Fatalf("occurred %s, want %s", got, test.occurred)
			}
			if got := day.Format(time.DateOnly); got != test.day {
				t.Fatalf("day %s, want %s", got, test.day)
			}
		})
	}
}

func TestParseBusinessTimeRejectsUnknownFormatAndZone(t *testing.T) {
	if _, _, err := ParseBusinessTime("2026/09/28 12:30", "Asia/Shanghai"); err == nil {
		t.Fatal("expected invalid format")
	}
	if _, _, err := ParseBusinessTime("2026-09-28T12:30", "Invalid/Zone"); err == nil {
		t.Fatal("expected invalid timezone")
	}
}
