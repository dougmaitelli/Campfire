// Package recurrence validates calendar schedules and computes their next run.
package recurrence

import (
	"errors"
	"fmt"
	"strings"
	"time"
	_ "time/tzdata" // Keep named zones available in minimal container images.

	"github.com/robfig/cron/v3"
)

// Parse accepts standard five-field cron expressions in an explicit time zone.
func Parse(expression, zone string) (cron.Schedule, error) {
	if len(expression) > 256 || len(strings.Fields(expression)) != 5 {
		return nil, errors.New("use five cron fields: minute, hour, day, month, weekday")
	}

	if zone == "" || zone == "Local" || len(zone) > 100 || strings.ContainsAny(zone, " \t\r\n") {
		return nil, errors.New("choose a named time zone, such as UTC or America/Los_Angeles")
	}

	if _, err := time.LoadLocation(zone); err != nil {
		return nil, errors.New("unknown time zone")
	}

	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

	schedule, err := parser.Parse("CRON_TZ=" + zone + " " + expression)
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression: %w", err)
	}

	return schedule, nil
}

// Next returns future run times using the same parser as the scheduler.
func Next(expression, zone string, after time.Time, count int) ([]int64, error) {
	schedule, err := Parse(expression, zone)
	if err != nil {
		return nil, err
	}

	result := make([]int64, 0, count)
	for range count {
		after = schedule.Next(after)
		if after.IsZero() {
			return nil, errors.New("this cron expression has no upcoming run")
		}

		result = append(result, after.Unix())
	}

	return result, nil
}
