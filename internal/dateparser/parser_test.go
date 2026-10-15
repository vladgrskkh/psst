package dateparser_test

import (
	"errors"
	"testing"
	"time"

	"github.com/vladgrskkh/psst/internal/dateparser"
)

func testNow() time.Time {
	return time.Date(2026, 8, 17, 10, 0, 0, 0, time.Local) // Monday
}

func TestParse(t *testing.T) {
	t.Parallel()

	now := testNow()
	cases := []struct {
		name string
		in   string
		want time.Time
	}{
		{"iso date", "2026-08-20", time.Date(2026, 8, 20, 9, 0, 0, 0, time.Local)},
		{"iso datetime", "2026-08-20 14:00", time.Date(2026, 8, 20, 14, 0, 0, 0, time.Local)},
		{"today", "today", time.Date(2026, 8, 17, 9, 0, 0, 0, time.Local)},
		{"today with later time", "today 15:00", time.Date(2026, 8, 17, 15, 0, 0, 0, time.Local)},
		{"today with past time rolls to next week", "today 09:00", time.Date(2026, 8, 24, 9, 0, 0, 0, time.Local)},
		{"tomorrow", "tomorrow", time.Date(2026, 8, 18, 9, 0, 0, 0, time.Local)},
		{"tomorrow with time and at", "tomorrow at 18:30", time.Date(2026, 8, 18, 18, 30, 0, 0, time.Local)},
		{"weekday fri", "fri 18:30", time.Date(2026, 8, 21, 18, 30, 0, 0, time.Local)},
		{"weekday full name", "sunday", time.Date(2026, 8, 23, 9, 0, 0, 0, time.Local)},
		{"relative hours", "in 2h", time.Date(2026, 8, 17, 12, 0, 0, 0, time.Local)},
		{"relative minutes", "in 30m", time.Date(2026, 8, 17, 10, 30, 0, 0, time.Local)},
		{"relative days", "in 2d", time.Date(2026, 8, 19, 10, 0, 0, 0, time.Local)},
		{"relative weeks", "in 2w", time.Date(2026, 8, 31, 10, 0, 0, 0, time.Local)},
		{"relative weeks with time", "in 2w at 18:30", time.Date(2026, 8, 31, 18, 30, 0, 0, time.Local)},
		{"relative days with time", "in 2d 18:30", time.Date(2026, 8, 19, 18, 30, 0, 0, time.Local)},
		{"relative days with pm", "in 2d at 9pm", time.Date(2026, 8, 19, 21, 0, 0, 0, time.Local)},
		{"relative days with midnight", "in 1d at 00:00", time.Date(2026, 8, 18, 0, 0, 0, 0, time.Local)},
		{"relative keywords uppercase", "IN 2W AT 18:30", time.Date(2026, 8, 31, 18, 30, 0, 0, time.Local)},
		{"time only future", "15:00", time.Date(2026, 8, 17, 15, 0, 0, 0, time.Local)},
		{"time only past rolls to tomorrow", "09:00", time.Date(2026, 8, 18, 9, 0, 0, 0, time.Local)},
		{"am suffix", "9am", time.Date(2026, 8, 18, 9, 0, 0, 0, time.Local)},
		{"pm suffix", "9 pm", time.Date(2026, 8, 17, 21, 0, 0, 0, time.Local)},
		{"tomorrow with uppercase PM", "tomorrow at 9PM", time.Date(2026, 8, 18, 21, 0, 0, 0, time.Local)},
		{"bare lowercase pm", "9pm", time.Date(2026, 8, 17, 21, 0, 0, 0, time.Local)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := dateparser.Parse(tc.in, now, "09:00")
			if err != nil {
				t.Fatalf("Parse(%q): %v", tc.in, err)
			}
			if !got.Equal(tc.want) {
				t.Fatalf("Parse(%q) = %v; want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	t.Parallel()

	for _, in := range []string{
		"", "   ", "banana", "in 2x", "99:99", "@tomorrow",
		"in 2w at 99:99", "in 2w at", "in 2h at 18:30", "in -2w at 18:30",
	} {
		_, err := dateparser.Parse(in, testNow(), "09:00")
		if !errors.Is(err, dateparser.ErrInvalidDate) {
			t.Fatalf("Parse(%q) error = %v; want ErrInvalidDate", in, err)
		}
	}
}

func TestRelativeClockKeepsCalendarDateAcrossDST(t *testing.T) {
	t.Parallel()
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 3, 7, 23, 30, 0, 0, location)
	got, err := dateparser.Parse("in 1w at 18:30", now, "09:00")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 3, 14, 18, 30, 0, 0, location)
	if !got.Equal(want) {
		t.Fatalf("relative calendar deadline = %v; want %v", got, want)
	}
}

func TestRelativeClockSkipsMidnightDSTGap(t *testing.T) {
	t.Parallel()
	location, err := time.LoadLocation("America/Santiago")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 5, 0, 30, 0, 0, location)
	got, err := dateparser.Parse("in 1d at 18:30", now, "09:00")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 9, 6, 18, 30, 0, 0, location)
	if !got.Equal(want) {
		t.Fatalf("deadline across midnight DST = %v; want %v", got, want)
	}
}
