package recurrence_test

import (
	"testing"
	"time"

	"campfire/internal/campfire/recurrence"
)

func TestCalendarRunTimes(t *testing.T) {
	for _, test := range []struct{ name, expression, zone, after, want string }{
		{"hourly", "0 * * * *", "UTC", "2026-01-01T00:01:00Z", "2026-01-01T01:00:00Z"},
		{"steps", "*/15 * * * *", "UTC", "2026-01-01T00:15:00Z", "2026-01-01T00:30:00Z"},
		{"weekdays", "0 3 * * 1-5", "UTC", "2026-01-02T04:00:00Z", "2026-01-05T03:00:00Z"},
		{"named weekday", "0 3 * * MON", "UTC", "2026-01-01T00:00:00Z", "2026-01-05T03:00:00Z"},
		{"day or weekday", "0 3 1 * MON", "UTC", "2026-01-01T00:00:00Z", "2026-01-01T03:00:00Z"},
		{"named zone", "0 3 * * *", "Asia/Tokyo", "2026-01-01T00:00:00Z", "2026-01-01T18:00:00Z"},
		{"spring DST", "0 3 * * *", "America/Los_Angeles", "2026-03-07T12:00:00Z", "2026-03-08T10:00:00Z"},
		{"autumn DST", "0 3 * * *", "America/Los_Angeles", "2026-10-31T12:00:00Z", "2026-11-01T11:00:00Z"},
		{"leap day", "0 0 29 2 *", "UTC", "2026-01-01T00:00:00Z", "2028-02-29T00:00:00Z"},
	} {
		t.Run(test.name, func(t *testing.T) {
			after, err := time.Parse(time.RFC3339, test.after)
			if err != nil {
				t.Fatal(err)
			}

			next, err := recurrence.Next(test.expression, test.zone, after, 3)
			if err != nil {
				t.Fatal(err)
			}

			if got := time.Unix(next[0], 0).UTC().Format(time.RFC3339); got != test.want {
				t.Fatalf("got %s; want %s", got, test.want)
			}

			if len(next) != 3 || next[1] <= next[0] || next[2] <= next[1] {
				t.Fatal("preview must contain three increasing run times", next)
			}
		})
	}
}

func TestRejectInvalidSchedules(t *testing.T) {
	for _, test := range []struct{ expression, zone string }{
		{"", "UTC"}, {"* * * *", "UTC"}, {"* * * * * *", "UTC"}, {"60 * * * *", "UTC"}, {"0 0 31 2 *", "UTC"}, {"@every 1m", "UTC"}, {"CRON_TZ=UTC * * * * *", "UTC"}, {"* * * * *", "Not/AZone"}, {"* * * * *", "Local"}, {"* * * * *", ""},
	} {
		if _, err := recurrence.Next(test.expression, test.zone, time.Now(), 3); err == nil {
			t.Errorf("accepted %q in %q", test.expression, test.zone)
		}
	}
}
