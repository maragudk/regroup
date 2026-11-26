package model_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"hegel.dev/go/hegel"
	"hegel.dev/go/hegel/temporal"
	"maragu.dev/is"

	"app/model"
)

func TestGroupByDay(t *testing.T) {
	copenhagen, err := time.LoadLocation("Europe/Copenhagen")
	is.NotError(t, err)

	t.Run("groups posts into days in the given location, newest first, labelled today and yesterday", func(t *testing.T) {
		now := time.Date(2026, 11, 26, 9, 0, 0, 0, copenhagen)
		posts := []model.TimelinePost{
			postAt("p1", time.Date(2026, 11, 26, 8, 0, 0, 0, copenhagen)),
			postAt("p2", time.Date(2026, 11, 26, 0, 30, 0, 0, copenhagen)), // 23:30 the day before in UTC
			postAt("p3", time.Date(2026, 11, 25, 23, 59, 0, 0, copenhagen)),
			postAt("p4", time.Date(2026, 11, 23, 12, 0, 0, 0, copenhagen)),
		}

		days := model.GroupByDay(posts, copenhagen, now)

		is.Equal(t, 3, len(days))

		is.Equal(t, model.Date{Year: 2026, Month: time.November, Day: 26}, days[0].Date)
		is.True(t, days[0].Today)
		is.True(t, !days[0].Yesterday)
		is.EqualSlice(t, []model.PostID{"p1", "p2"}, postIDs(days[0].Posts))

		is.Equal(t, model.Date{Year: 2026, Month: time.November, Day: 25}, days[1].Date)
		is.True(t, !days[1].Today)
		is.True(t, days[1].Yesterday)
		is.EqualSlice(t, []model.PostID{"p3"}, postIDs(days[1].Posts))

		is.Equal(t, model.Date{Year: 2026, Month: time.November, Day: 23}, days[2].Date)
		is.True(t, !days[2].Today)
		is.True(t, !days[2].Yesterday)
		is.EqualSlice(t, []model.PostID{"p4"}, postIDs(days[2].Posts))
	})

	t.Run("takes today from the current time in the given location, not in UTC", func(t *testing.T) {
		// 00:30 in Copenhagen on the 26th is still the 25th in UTC.
		now := time.Date(2026, 11, 25, 23, 30, 0, 0, time.UTC)
		posts := []model.TimelinePost{postAt("p1", time.Date(2026, 11, 25, 23, 15, 0, 0, time.UTC))}

		days := model.GroupByDay(posts, copenhagen, now)

		is.Equal(t, 1, len(days))
		is.Equal(t, model.Date{Year: 2026, Month: time.November, Day: 26}, days[0].Date)
		is.True(t, days[0].Today)
	})

	t.Run("labels yesterday across the end of a month and a year", func(t *testing.T) {
		now := time.Date(2027, 1, 1, 10, 0, 0, 0, copenhagen)
		posts := []model.TimelinePost{postAt("p1", time.Date(2026, 12, 31, 10, 0, 0, 0, copenhagen))}

		days := model.GroupByDay(posts, copenhagen, now)

		is.True(t, days[0].Yesterday)
	})

	t.Run("puts both sides of the night clocks go back on their own dates", func(t *testing.T) {
		// Clocks in Copenhagen went from 03:00 back to 02:00 on 2026-10-25.
		now := time.Date(2026, 10, 26, 12, 0, 0, 0, copenhagen)
		before := time.Date(2026, 10, 24, 23, 30, 0, 0, copenhagen)
		after := before.Add(25 * time.Hour) // 23:30 on the 25th, since that day had 25 hours
		posts := []model.TimelinePost{postAt("p2", after), postAt("p1", before)}

		days := model.GroupByDay(posts, copenhagen, now)

		is.Equal(t, 2, len(days))
		is.Equal(t, model.Date{Year: 2026, Month: time.October, Day: 25}, days[0].Date)
		is.True(t, days[0].Yesterday)
		is.Equal(t, model.Date{Year: 2026, Month: time.October, Day: 24}, days[1].Date)
	})

	t.Run("gives no days for no posts", func(t *testing.T) {
		is.Equal(t, 0, len(model.GroupByDay(nil, copenhagen, time.Now())))
	})

	t.Run("puts every post in exactly one day, its local date, with days in order, in any time zone", func(t *testing.T) {
		hegel.Test(t, func(ht *hegel.T) {
			loc := hegel.Draw(ht, temporal.Zones())
			inZone := temporal.ZonedDatetimes().Zones(hegel.Just(loc))
			now := hegel.Draw(ht, inZone)
			times := hegel.Draw(ht, hegel.Lists(hegel.OneOf(
				inZone,
				// Close to now, so that today and yesterday come up.
				hegel.Map(temporal.Durations().Min(-72*time.Hour).Max(time.Hour), func(d time.Duration) time.Time { return now.Add(d) }),
			)).MaxSize(30))

			var posts []model.TimelinePost
			for i, at := range times {
				posts = append(posts, postAt(model.PostID(fmt.Sprint("p", i)), at))
			}
			slices.SortStableFunc(posts, func(a, b model.TimelinePost) int { return b.Created.T.Compare(a.Created.T) })

			days := model.GroupByDay(posts, loc, now)

			today := localDate(now, loc)
			yesterday := model.DateOf(today.Time().AddDate(0, 0, -1))
			seen := map[model.PostID]int{}
			for i, day := range days {
				if i > 0 && days[i-1].Date.Compare(day.Date) <= 0 {
					ht.Fatalf("day %v comes after day %v", day.Date, days[i-1].Date)
				}
				if len(day.Posts) == 0 {
					ht.Fatalf("day %v has no posts", day.Date)
				}
				if day.Today != (day.Date == today) {
					ht.Fatalf("day %v has today %v, but today is %v", day.Date, day.Today, today)
				}
				if day.Yesterday != (day.Date == yesterday) {
					ht.Fatalf("day %v has yesterday %v, but yesterday is %v", day.Date, day.Yesterday, yesterday)
				}
				for _, p := range day.Posts {
					seen[p.ID]++
					if got := localDate(p.Created.T, loc); got != day.Date {
						ht.Fatalf("post at %v (%v) is in day %v, but its local date is %v", p.Created.T, p.Created.T.In(loc), day.Date, got)
					}
				}
			}

			for _, p := range posts {
				if seen[p.ID] != 1 {
					ht.Fatalf("post %v at %v is in %v days", p.ID, p.Created.T, seen[p.ID])
				}
			}
			if len(seen) != len(posts) {
				ht.Fatalf("got %v posts in days, but gave %v", len(seen), len(posts))
			}

			// Grouping keeps the order of the page within each day.
			for _, day := range days {
				var given []model.PostID
				for _, p := range posts {
					if localDate(p.Created.T, loc) == day.Date {
						given = append(given, p.ID)
					}
				}
				if !slices.Equal(given, postIDs(day.Posts)) {
					ht.Fatalf("day %v has posts %v, but they were given in the order %v", day.Date, postIDs(day.Posts), given)
				}
			}
		}, hegel.WithTestCases(500))
	})
}

func TestDate(t *testing.T) {
	t.Run("adds days across months, years, and leap days", func(t *testing.T) {
		is.Equal(t, model.Date{Year: 2028, Month: time.February, Day: 29}, model.Date{Year: 2028, Month: time.March, Day: 1}.AddDays(-1))
		is.Equal(t, model.Date{Year: 2027, Month: time.January, Day: 1}, model.Date{Year: 2026, Month: time.December, Day: 31}.AddDays(1))
	})

	t.Run("compares dates", func(t *testing.T) {
		a := model.Date{Year: 2026, Month: time.November, Day: 26}
		is.Equal(t, 0, a.Compare(a))
		is.Equal(t, -1, a.Compare(a.AddDays(1)))
		is.Equal(t, 1, a.Compare(a.AddDays(-1)))
		is.Equal(t, 1, a.Compare(model.Date{Year: 2025, Month: time.December, Day: 31}))
	})

	t.Run("formats as year, month, and day", func(t *testing.T) {
		is.Equal(t, "2026-11-06", model.Date{Year: 2026, Month: time.November, Day: 6}.String())
	})
}

func TestTimelineCursor(t *testing.T) {
	t.Run("round-trips through its string", func(t *testing.T) {
		c := model.TimelineCursor{Created: model.Time{T: time.Date(2026, 11, 26, 9, 15, 0, 123_000_000, time.UTC)}, Seq: 42}

		parsed, err := model.ParseTimelineCursor(c.String())
		is.NotError(t, err)
		is.True(t, c.Created.T.Equal(parsed.Created.T))
		is.Equal(t, c.Seq, parsed.Seq)
	})

	t.Run("is URL-safe text", func(t *testing.T) {
		c := model.TimelineCursor{Created: model.Time{T: time.Date(9999, 12, 31, 23, 59, 59, 999_000_000, time.UTC)}, Seq: 1<<63 - 1}
		is.True(t, !strings.ContainsAny(c.String(), "+/=?&% \n"), c.String())
	})

	for _, s := range []string{"", "x", "abc", "AQAAAZvNMb8AAAAAAAAAAAE=", "AgAAAZvNMb8AAAAAAAAAACo", "AQAAAZvNMb8AAAAAAAAAAAA", "AQAAAZvNMb8AAAAA\nAAAAACo", "AQAAAZvNMb8AAAAAAAAAACo-"} {
		t.Run(fmt.Sprintf("refuses %q as invalid", s), func(t *testing.T) {
			_, err := model.ParseTimelineCursor(s)
			is.Error(t, model.ErrorCursorInvalid, err)
		})
	}

	t.Run("round-trips any time posted within years 1 to 9999 and any position", func(t *testing.T) {
		hegel.Test(t, func(ht *hegel.T) {
			created := hegel.Draw(ht, temporal.Datetimes().Precision(temporal.PrecisionMilliseconds))
			seq := hegel.Draw(ht, hegel.Integers[int64](1, 1<<63-1))
			c := model.TimelineCursor{Created: model.Time{T: created}, Seq: seq}

			parsed, err := model.ParseTimelineCursor(c.String())
			if err != nil {
				ht.Fatalf("could not parse %q from %+v: %v", c.String(), c, err)
			}
			if !parsed.Created.T.Equal(created) || parsed.Seq != seq {
				ht.Fatalf("parsed %+v from %+v", parsed, c)
			}
		})
	})
}

func FuzzParseTimelineCursor(f *testing.F) {
	f.Add("")
	f.Add("AQAAAZvNMb8AAAAAAAAAACo")
	f.Add(model.TimelineCursor{Created: model.Time{T: time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC)}, Seq: 1}.String())
	f.Add("AQAAAZvNMb8AAAAA\nAAAAACo")
	f.Add("not a cursor")

	f.Fuzz(func(t *testing.T, s string) {
		c, err := model.ParseTimelineCursor(s)
		if err != nil {
			if err != model.ErrorCursorInvalid {
				t.Fatalf("unexpected error %v", err)
			}
			if c != (model.TimelineCursor{}) {
				t.Fatalf("got cursor %+v along with an error", c)
			}
			return
		}

		// Whatever is accepted is a cursor that could have been made, and the only encoding of it.
		if c.Seq < 1 {
			t.Fatalf("accepted position %v", c.Seq)
		}
		if y := c.Created.T.Year(); y < 1 || y > 9999 {
			t.Fatalf("accepted year %v", y)
		}
		if c.String() != s {
			t.Fatalf("accepted %q, which encodes back as %q", s, c.String())
		}
	})
}

func postAt(id model.PostID, at time.Time) model.TimelinePost {
	return model.TimelinePost{Post: model.Post{ID: id, Created: model.Time{T: at}}}
}

func postIDs(posts []model.TimelinePost) []model.PostID {
	var ids []model.PostID
	for _, p := range posts {
		ids = append(ids, p.ID)
	}
	return ids
}

// localDate of the time in the location, worked out from the offset rather than with [time.Time.In].
func localDate(t time.Time, loc *time.Location) model.Date {
	_, offset := t.In(loc).Zone()
	return model.DateOf(t.UTC().Add(time.Duration(offset) * time.Second))
}
