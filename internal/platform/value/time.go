package value

import (
	"fmt"
	"strings"
	"time"
)

var localDateTimeLayouts = []string{
	"2006-01-02T15:04",
	"2006-01-02T15:04:05",
	"2006-01-02T15:04:05.999999999",
}

// ParseBusinessTime accepts the date and datetime formats emitted by HTML
// date/datetime-local controls, plus RFC3339 timestamps from API clients.
// Values without an explicit offset are interpreted in the store timezone.
func ParseBusinessTime(raw, zone string) (time.Time, time.Time, error) {
	location, err := time.LoadLocation(zone)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}

	raw = strings.TrimSpace(raw)
	if day, parseErr := time.ParseInLocation(time.DateOnly, raw, location); parseErr == nil {
		return day.UTC(), day, nil
	}

	for _, layout := range localDateTimeLayouts {
		if local, parseErr := time.ParseInLocation(layout, raw, location); parseErr == nil {
			return local.UTC(), businessDay(local, location), nil
		}
	}

	instant, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid business time: %w", err)
	}
	return instant.UTC(), businessDay(instant, location), nil
}

func businessDay(value time.Time, location *time.Location) time.Time {
	local := value.In(location)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
}
