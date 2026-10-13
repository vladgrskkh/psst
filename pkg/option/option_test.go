package option_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/vladgrskkh/psst/pkg/option"
)

func TestGetDistinguishesPresentAndAbsent(t *testing.T) {
	t.Parallel()

	got, ok := option.Some("x").Get()
	if !ok || got != "x" {
		t.Fatalf("Some.Get() = %q, %v; want %q, true", got, ok, "x")
	}
	if _, present := option.None[string]().Get(); present {
		t.Fatal("None.Get() reported a value")
	}
}

func TestValueOrUsesFallbackOnlyWhenAbsent(t *testing.T) {
	t.Parallel()

	if got := option.Some(3).ValueOr(9); got != 3 {
		t.Fatalf("Some.ValueOr() = %d; want 3", got)
	}
	if got := option.None[int]().ValueOr(9); got != 9 {
		t.Fatalf("None.ValueOr() = %d; want 9", got)
	}
}

func TestJSONRoundTrip(t *testing.T) {
	t.Parallel()

	wantTime := time.Date(2026, 8, 20, 14, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name string
		in   option.Option[time.Time]
	}{
		{name: "present", in: option.Some(wantTime)},
		{name: "absent", in: option.None[time.Time]()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			data, marshalErr := json.Marshal(tc.in)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			var got option.Option[time.Time]
			if decodeErr := json.Unmarshal(data, &got); decodeErr != nil {
				t.Fatal(decodeErr)
			}
			want, wantOK := tc.in.Get()
			gotTime, gotOK := got.Get()
			if wantOK != gotOK || (wantOK && !want.Equal(gotTime)) {
				t.Fatalf("round trip = %v, %v; want %v, %v", gotTime, gotOK, want, wantOK)
			}
		})
	}
}

func TestUnmarshalJSONNullWithWhitespace(t *testing.T) {
	t.Parallel()

	var got option.Option[string]
	if err := json.Unmarshal([]byte("  null \n"), &got); err != nil {
		t.Fatal(err)
	}
	if got.IsPresent() {
		t.Fatal("whitespace-padded null must decode as None")
	}
}

func TestOmitzeroOmitsAbsentOptionButKeepsPresentZero(t *testing.T) {
	t.Parallel()

	type payload struct {
		Absent  option.Option[string] `json:"absent,omitzero"`
		Present option.Option[string] `json:"present,omitzero"`
	}
	data, err := json.Marshal(payload{Present: option.Some("")})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), `{"present":""}`; got != want {
		t.Fatalf("JSON = %s; want %s", got, want)
	}
}
