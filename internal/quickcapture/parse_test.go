package quickcapture_test

import (
	"errors"
	"testing"
	"time"

	"github.com/vladgrskkh/psst/internal/quickcapture"
	"github.com/vladgrskkh/psst/pkg/option"
)

func testNow() time.Time {
	return time.Date(2026, 8, 17, 10, 0, 0, 0, time.Local)
}

func TestParse(t *testing.T) {
	t.Parallel()

	now := testNow()
	cases := []struct {
		name string
		in   string
		want quickcapture.Result
	}{
		{
			name: "full", in: "buy oat milk @tomorrow 9:00 #groceries",
			want: quickcapture.Result{
				Title:    "buy oat milk",
				Deadline: some(time.Date(2026, 8, 18, 9, 0, 0, 0, time.Local)),
				List:     someStr("groceries"),
			},
		},
		{
			name: "title only", in: "buy milk",
			want: quickcapture.Result{Title: "buy milk"},
		},
		{
			name: "date only", in: "buy milk @fri",
			want: quickcapture.Result{
				Title:    "buy milk",
				Deadline: some(time.Date(2026, 8, 21, 9, 0, 0, 0, time.Local)),
			},
		},
		{
			name: "list with spaces", in: "call mom #family stuff",
			want: quickcapture.Result{Title: "call mom", List: someStr("family stuff")},
		},
		{
			name: "at word", in: "stretch @tomorrow at 18:30",
			want: quickcapture.Result{
				Title:    "stretch",
				Deadline: some(time.Date(2026, 8, 18, 18, 30, 0, 0, time.Local)),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := quickcapture.Parse(tc.in, now, "09:00")
			if err != nil {
				t.Fatalf("Parse(%q): %v", tc.in, err)
			}
			if got.Title != tc.want.Title {
				t.Fatalf("Title = %q; want %q", got.Title, tc.want.Title)
			}
			wantD, wantOK := tc.want.Deadline.Get()
			gotD, gotOK := got.Deadline.Get()
			if wantOK != gotOK || (wantOK && !wantD.Equal(gotD)) {
				t.Fatalf("Deadline = %v; want %v", got.Deadline, tc.want.Deadline)
			}
			wantL, wantLOK := tc.want.List.Get()
			gotL, gotLOK := got.List.Get()
			if wantLOK != gotLOK || (wantLOK && wantL != gotL) {
				t.Fatalf("List = %v; want %v", got.List, tc.want.List)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	t.Parallel()

	if _, err := quickcapture.Parse("", testNow(), "09:00"); !errors.Is(err, quickcapture.ErrEmptyTitle) {
		t.Fatalf("err = %v; want ErrEmptyTitle", err)
	}
	if _, err := quickcapture.Parse("  @tomorrow ", testNow(), "09:00"); !errors.Is(err, quickcapture.ErrEmptyTitle) {
		t.Fatalf("err = %v; want ErrEmptyTitle", err)
	}
}

func some(t time.Time) option.Option[time.Time] { return option.Some(t) }
func someStr(s string) option.Option[string]    { return option.Some(s) }
