package model

import (
	"strings"
	"time"

	"maragu.dev/errors"
)

// Group that this instance holds. One instance holds one group.
type Group struct {
	// Name of the group, as the host configured it.
	Name string

	// TimeZone the group's timeline is grouped into days in.
	TimeZone *time.Location
}

// NewGroup with the given name and IANA time zone name, such as "Europe/Copenhagen".
// An empty time zone is UTC. "Local" is refused, since it depends on the machine the app runs on.
func NewGroup(name, timeZone string) (Group, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Group{}, errors.New("group name is empty")
	}

	if timeZone == "Local" {
		return Group{}, errors.New(`group time zone "Local" depends on the machine; use an IANA name such as "Europe/Copenhagen"`)
	}

	loc, err := time.LoadLocation(timeZone)
	if err != nil {
		return Group{}, errors.Wrap(err, "error loading group time zone %q", timeZone)
	}

	return Group{Name: name, TimeZone: loc}, nil
}
