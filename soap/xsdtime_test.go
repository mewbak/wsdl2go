package soap

import (
	"encoding/xml"
	"testing"
	"time"
)

func TestDateTimeRoundTrip(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"utc", "2023-06-15T14:30:00Z", "2023-06-15T14:30:00Z"},
		{"offset", "2023-06-15T14:30:00+05:00", "2023-06-15T14:30:00+05:00"},
		{"neg offset", "2023-06-15T14:30:00-08:00", "2023-06-15T14:30:00-08:00"},
		{"no tz", "2023-06-15T14:30:00", "2023-06-15T14:30:00"},
		{"nanoseconds", "2023-06-15T14:30:00.123456789Z", "2023-06-15T14:30:00.123456789Z"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			type wrapper struct {
				XMLName xml.Name `xml:"Root"`
				Value   DateTime `xml:"Value"`
			}
			input := `<Root><Value>` + tc.input + `</Value></Root>`
			var w wrapper
			if err := xml.Unmarshal([]byte(input), &w); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			data, err := xml.Marshal(&w)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			want := `<Root><Value>` + tc.want + `</Value></Root>`
			if string(data) != want {
				t.Errorf("round-trip:\n  want: %s\n  have: %s", want, string(data))
			}
		})
	}
}

func TestDateTimeToGoTime(t *testing.T) {
	dt := CreateDateTime(time.Date(2023, 6, 15, 14, 30, 0, 0, time.UTC), true)
	got := dt.ToGoTime()
	if got.Year() != 2023 || got.Month() != 6 || got.Day() != 15 {
		t.Errorf("unexpected date: %v", got)
	}
	if got.Location() != time.UTC {
		t.Errorf("expected UTC, got %v", got.Location())
	}
}

func TestDateTimeNoTzToGoTime(t *testing.T) {
	dt := CreateDateTime(time.Date(2023, 6, 15, 14, 30, 0, 0, time.UTC), false)
	got := dt.ToGoTime()
	if got.Location() != time.Local {
		t.Errorf("expected Local, got %v", got.Location())
	}
}

func TestDateTimeZero(t *testing.T) {
	type wrapper struct {
		XMLName xml.Name `xml:"Root"`
		Value   DateTime `xml:"Value"`
	}
	var w wrapper
	data, err := xml.Marshal(&w)
	if err != nil {
		t.Fatal(err)
	}
	// Zero DateTime skips the element entirely.
	if string(data) != `<Root></Root>` {
		t.Errorf("unexpected: %s", string(data))
	}
}

func TestDateRoundTrip(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"utc", "2023-06-15Z", "2023-06-15Z"},
		{"offset", "2023-06-15+05:00", "2023-06-15+05:00"},
		{"no tz", "2023-06-15", "2023-06-15"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			type wrapper struct {
				XMLName xml.Name `xml:"Root"`
				Value   Date     `xml:"Value"`
			}
			input := `<Root><Value>` + tc.input + `</Value></Root>`
			var w wrapper
			if err := xml.Unmarshal([]byte(input), &w); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			data, err := xml.Marshal(&w)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			want := `<Root><Value>` + tc.want + `</Value></Root>`
			if string(data) != want {
				t.Errorf("round-trip:\n  want: %s\n  have: %s", want, string(data))
			}
		})
	}
}

func TestDateToGoTime(t *testing.T) {
	d := CreateDate(time.Date(2023, 6, 15, 0, 0, 0, 0, time.UTC), true)
	got := d.ToGoTime()
	if got.Year() != 2023 || got.Month() != 6 || got.Day() != 15 {
		t.Errorf("unexpected date: %v", got)
	}
}

func TestTimeRoundTrip(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"utc", "14:30:00Z", "14:30:00Z"},
		{"offset", "14:30:00+05:00", "14:30:00+05:00"},
		{"no tz", "14:30:00", "14:30:00"},
		{"nanoseconds", "14:30:00.123Z", "14:30:00.123Z"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			type wrapper struct {
				XMLName xml.Name `xml:"Root"`
				Value   Time     `xml:"Value"`
			}
			input := `<Root><Value>` + tc.input + `</Value></Root>`
			var w wrapper
			if err := xml.Unmarshal([]byte(input), &w); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			data, err := xml.Marshal(&w)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			want := `<Root><Value>` + tc.want + `</Value></Root>`
			if string(data) != want {
				t.Errorf("round-trip:\n  want: %s\n  have: %s", want, string(data))
			}
		})
	}
}

func TestTimeAccessors(t *testing.T) {
	tm := CreateTime(14, 30, 45, 123000000, time.UTC)
	if tm.Hour() != 14 {
		t.Errorf("hour: want 14, have %d", tm.Hour())
	}
	if tm.Minute() != 30 {
		t.Errorf("minute: want 30, have %d", tm.Minute())
	}
	if tm.Second() != 45 {
		t.Errorf("second: want 45, have %d", tm.Second())
	}
	if tm.Nanosecond() != 123000000 {
		t.Errorf("nanosecond: want 123000000, have %d", tm.Nanosecond())
	}
	if tm.Location() != time.UTC {
		t.Errorf("location: want UTC, have %v", tm.Location())
	}
}

func TestTimeNoTzLocation(t *testing.T) {
	tm := CreateTime(14, 30, 0, 0, nil)
	if tm.Location() != nil {
		t.Errorf("expected nil location, got %v", tm.Location())
	}
}

func TestDateTimeAttr(t *testing.T) {
	type wrapper struct {
		XMLName xml.Name `xml:"Root"`
		Value   DateTime `xml:"value,attr"`
	}
	input := `<Root value="2023-06-15T14:30:00Z"/>`
	var w wrapper
	if err := xml.Unmarshal([]byte(input), &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if w.Value.inner.Year() != 2023 {
		t.Errorf("unexpected year: %d", w.Value.inner.Year())
	}

	data, err := xml.Marshal(&w)
	if err != nil {
		t.Fatal(err)
	}
	want := `<Root value="2023-06-15T14:30:00Z"></Root>`
	if string(data) != want {
		t.Errorf("marshal:\n  want: %s\n  have: %s", want, string(data))
	}
}
