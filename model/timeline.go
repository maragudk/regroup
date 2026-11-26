package model

import (
	"cmp"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"slices"
	"time"
)

// TimelinePost is a [Post] as one reader sees it on the timeline.
type TimelinePost struct {
	Post

	// AuthorName is the name the group sees for the author.
	AuthorName string `db:"author_name"`

	// Comments is how many comments the post has.
	Comments int

	// New is whether the post is new to the reader: it is by someone else and was posted since the
	// reader's previous look at the timeline.
	New bool

	// NewComments is how many of the comments are new to the reader: by someone else, and added since the
	// reader last opened the post's comments.
	NewComments int `db:"new_comments"`
}

// TimelineComment is a [Comment] as one reader sees it.
type TimelineComment struct {
	Comment

	// AuthorName is the name the group sees for the author.
	AuthorName string `db:"author_name"`

	// New is whether the comment was new to the reader when the comments were opened: by someone else,
	// and added since the reader last opened the post's comments.
	New bool
}

// Timeline is one page of the timeline for one reader, grouped into days.
type Timeline struct {
	// Days of the page, newest first.
	Days []TimelineDay

	// Next is the cursor for the page after this one, or empty if this is the last page.
	Next string
}

// TimelineDay is one day of a [Timeline] page, in the group's time zone.
type TimelineDay struct {
	// Date in the group's time zone.
	Date Date

	// Today is whether Date is today in the group's time zone.
	Today bool

	// Yesterday is whether Date is the day before today in the group's time zone.
	Yesterday bool

	// Posts of the day, in the order of the page.
	Posts []TimelinePost
}

// Date in the calendar, without a time of day or a time zone.
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

// DateOf the given time, in its own location.
func DateOf(t time.Time) Date {
	y, m, d := t.Date()
	return Date{Year: y, Month: m, Day: d}
}

// AddDays to the date, which may be negative.
func (d Date) AddDays(days int) Date {
	return DateOf(time.Date(d.Year, d.Month, d.Day+days, 0, 0, 0, 0, time.UTC))
}

// Compare the date with another, returning -1 if d is earlier, 0 if they are the same, and +1 if d is later.
func (d Date) Compare(other Date) int {
	return cmp.Or(cmp.Compare(d.Year, other.Year), cmp.Compare(d.Month, other.Month), cmp.Compare(d.Day, other.Day))
}

// Time at midnight of the date in UTC, for formatting the date.
func (d Date) Time() time.Time {
	return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC)
}

// String satisfies [fmt.Stringer], in the form 2006-01-02.
func (d Date) String() string {
	return d.Time().Format(time.DateOnly)
}

var _ fmt.Stringer = Date{}

// GroupByDay the given posts, by the date each was posted in the given location.
// Days are newest first, and the posts of a day keep the order they were given in.
// A day is labelled as today or yesterday from the given current time.
func GroupByDay(posts []TimelinePost, loc *time.Location, now time.Time) []TimelineDay {
	today := DateOf(now.In(loc))
	yesterday := today.AddDays(-1)

	var days []TimelineDay
	indexes := map[Date]int{}
	for _, p := range posts {
		date := DateOf(p.Created.T.In(loc))
		i, ok := indexes[date]
		if !ok {
			i = len(days)
			indexes[date] = i
			days = append(days, TimelineDay{
				Date:      date,
				Today:     date == today,
				Yesterday: date == yesterday,
			})
		}
		days[i].Posts = append(days[i].Posts, p)
	}

	// Only out of order if the location's clocks went back past midnight, and then a stable sort keeps
	// each day's posts in the order given.
	slices.SortStableFunc(days, func(a, b TimelineDay) int {
		return b.Date.Compare(a.Date)
	})

	return days
}

// TimelineCursor is the position after the last post of a timeline page.
// The timeline is ordered by time posted and then by the order posts were stored in, newest first,
// so the cursor holds both.
type TimelineCursor struct {
	Created Time
	Seq     int64
}

const timelineCursorVersion = 1

var (
	cursorMinTime = time.Date(1, time.January, 1, 0, 0, 0, 0, time.UTC)
	cursorMaxTime = time.Date(9999, time.December, 31, 23, 59, 59, 999_000_000, time.UTC)
)

// String satisfies [fmt.Stringer], encoding the cursor as an opaque, URL-safe string.
func (c TimelineCursor) String() string {
	b := make([]byte, 0, 17)
	b = append(b, timelineCursorVersion)
	b = binary.BigEndian.AppendUint64(b, uint64(c.Created.T.UnixMilli()))
	b = binary.BigEndian.AppendUint64(b, uint64(c.Seq))
	return base64.RawURLEncoding.EncodeToString(b)
}

var _ fmt.Stringer = TimelineCursor{}

// ParseTimelineCursor from what [TimelineCursor.String] encoded.
// Anything else is [ErrorCursorInvalid].
func ParseTimelineCursor(s string) (TimelineCursor, error) {
	b, err := base64.RawURLEncoding.Strict().DecodeString(s)
	if err != nil || len(b) != 17 || b[0] != timelineCursorVersion {
		return TimelineCursor{}, ErrorCursorInvalid
	}

	// The decoder skips line breaks, so only the canonical encoding is accepted.
	if base64.RawURLEncoding.EncodeToString(b) != s {
		return TimelineCursor{}, ErrorCursorInvalid
	}

	created := time.UnixMilli(int64(binary.BigEndian.Uint64(b[1:9]))).UTC()
	if created.Before(cursorMinTime) || created.After(cursorMaxTime) {
		return TimelineCursor{}, ErrorCursorInvalid
	}

	seq := int64(binary.BigEndian.Uint64(b[9:17]))
	if seq < 1 {
		return TimelineCursor{}, ErrorCursorInvalid
	}

	return TimelineCursor{Created: Time{T: created}, Seq: seq}, nil
}
