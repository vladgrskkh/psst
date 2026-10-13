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

	for _, in := range []string{"", "   ", "banana", "in 2x", "99:99", "@tomorrow"} {
		_, err := dateparser.Parse(in, testNow(), "09:00")
		if !errors.Is(err, dateparser.ErrInvalidDate) {
			t.Fatalf("Parse(%q) error = %v; want ErrInvalidDate", in, err)
		}
	}
}
