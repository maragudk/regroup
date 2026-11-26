package model_test

import (
	"testing"
	"time"

	"maragu.dev/is"

	"app/model"
)

func TestNewGroup(t *testing.T) {
	t.Run("has the given name and time zone", func(t *testing.T) {
		g, err := model.NewGroup(" The Loft ", "Europe/Copenhagen")
		is.NotError(t, err)
		is.Equal(t, "The Loft", g.Name)
		is.Equal(t, "Europe/Copenhagen", g.TimeZone.String())
	})

	t.Run("is in UTC with no time zone", func(t *testing.T) {
		g, err := model.NewGroup("The Loft", "")
		is.NotError(t, err)
		is.True(t, g.TimeZone == time.UTC)
	})

	t.Run("refuses an empty name", func(t *testing.T) {
		_, err := model.NewGroup("  ", "UTC")
		is.True(t, err != nil)
	})

	t.Run("refuses an unknown time zone", func(t *testing.T) {
		_, err := model.NewGroup("The Loft", "Europe/Atlantis")
		is.True(t, err != nil)
	})

	t.Run("refuses the machine's local time zone", func(t *testing.T) {
		_, err := model.NewGroup("The Loft", "Local")
		is.True(t, err != nil)
	})
}
