package service

import (
	"testing"
	"time"

	"maragu.dev/is"
)

func TestLifetime(t *testing.T) {
	t.Run("says a lifetime in whole days, hours, or minutes, in the singular for one", func(t *testing.T) {
		for d, expected := range map[time.Duration]string{
			15 * time.Minute:   "15 minutes",
			time.Minute:        "1 minute",
			time.Hour:          "1 hour",
			90 * time.Minute:   "90 minutes",
			48 * time.Hour:     "2 days",
			24 * time.Hour:     "1 day",
			7 * 24 * time.Hour: "7 days",
			36 * time.Hour:     "36 hours",
		} {
			is.Equal(t, expected, lifetime(d))
		}
	})
}
