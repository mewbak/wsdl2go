package soap

import (
	"encoding/xml"
	"strings"
	"time"
)

const (
	dateTimeLayout = "2006-01-02T15:04:05.999999999Z07:00"
	dateLayout     = "2006-01-02Z07:00"
	timeLayout     = "15:04:05.999999999Z07:00"
)

// DateTime represents an XSD dateTime with optional timezone.
type DateTime struct {
	inner time.Time
	hasTz bool
}

// CreateDateTime creates a DateTime from a time.Time.
func CreateDateTime(t time.Time, hasTz bool) DateTime {
	return DateTime{inner: t, hasTz: hasTz}
}

// ToGoTime converts to time.Time. If no timezone was specified in
// the original value, the time is returned in time.Local.
func (dt DateTime) ToGoTime() time.Time {
	if dt.hasTz {
		return dt.inner
	}
	return time.Date(
		dt.inner.Year(), dt.inner.Month(), dt.inner.Day(),
		dt.inner.Hour(), dt.inner.Minute(), dt.inner.Second(),
		dt.inner.Nanosecond(), time.Local,
	)
}

// IsZero reports whether the DateTime is the zero value.
func (dt DateTime) IsZero() bool {
	return dt.inner.IsZero()
}

func (dt DateTime) string() string {
	if dt.inner.IsZero() {
		return ""
	}
	s := dt.inner.Format(dateTimeLayout)
	if !dt.hasTz {
		return stripTzSuffix(s)
	}
	return s
}

func (dt DateTime) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	if dt.inner.IsZero() {
		return nil
	}
	return e.EncodeElement(dt.string(), start)
}

func (dt *DateTime) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var s string
	if err := d.DecodeElement(&s, &start); err != nil {
		return err
	}
	if s == "" {
		return nil
	}
	t, hasTz, err := parseDateTimeString(s, dateTimeLayout)
	if err != nil {
		return err
	}
	dt.inner = t
	dt.hasTz = hasTz
	return nil
}

func (dt DateTime) MarshalXMLAttr(name xml.Name) (xml.Attr, error) {
	if dt.inner.IsZero() {
		return xml.Attr{}, nil
	}
	return xml.Attr{Name: name, Value: dt.string()}, nil
}

func (dt *DateTime) UnmarshalXMLAttr(attr xml.Attr) error {
	if attr.Value == "" {
		return nil
	}
	t, hasTz, err := parseDateTimeString(attr.Value, dateTimeLayout)
	if err != nil {
		return err
	}
	dt.inner = t
	dt.hasTz = hasTz
	return nil
}

// Date represents an XSD date with optional timezone.
type Date struct {
	inner time.Time
	hasTz bool
}

// CreateDate creates a Date from a time.Time.
func CreateDate(t time.Time, hasTz bool) Date {
	return Date{inner: t, hasTz: hasTz}
}

// ToGoTime converts to time.Time with zeroed time components.
// If no timezone was specified, the time is returned in time.Local.
func (d Date) ToGoTime() time.Time {
	t := d.inner
	if !d.hasTz {
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// IsZero reports whether the Date is the zero value.
func (d Date) IsZero() bool {
	return d.inner.IsZero()
}

func (d Date) string() string {
	if d.inner.IsZero() {
		return ""
	}
	s := d.inner.Format(dateLayout)
	if !d.hasTz {
		return stripTzSuffix(s)
	}
	return s
}

func (d Date) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	if d.inner.IsZero() {
		return nil
	}
	return e.EncodeElement(d.string(), start)
}

func (d *Date) UnmarshalXML(dec *xml.Decoder, start xml.StartElement) error {
	var s string
	if err := dec.DecodeElement(&s, &start); err != nil {
		return err
	}
	if s == "" {
		return nil
	}
	t, hasTz, err := parseDateString(s, dateLayout)
	if err != nil {
		return err
	}
	d.inner = t
	d.hasTz = hasTz
	return nil
}

func (d Date) MarshalXMLAttr(name xml.Name) (xml.Attr, error) {
	if d.inner.IsZero() {
		return xml.Attr{}, nil
	}
	return xml.Attr{Name: name, Value: d.string()}, nil
}

func (d *Date) UnmarshalXMLAttr(attr xml.Attr) error {
	if attr.Value == "" {
		return nil
	}
	t, hasTz, err := parseDateString(attr.Value, dateLayout)
	if err != nil {
		return err
	}
	d.inner = t
	d.hasTz = hasTz
	return nil
}

// Time represents an XSD time with optional timezone.
type Time struct {
	inner time.Time
	hasTz bool
}

// CreateTime creates a Time value.
// If loc is nil, the time has no timezone.
func CreateTime(hour, min, sec, nsec int, loc *time.Location) Time {
	hasTz := loc != nil
	if loc == nil {
		loc = time.UTC
	}
	t := time.Date(0, 1, 1, hour, min, sec, nsec, loc)
	return Time{inner: t, hasTz: hasTz}
}

// Hour returns the hour component.
func (t Time) Hour() int { return t.inner.Hour() }

// Minute returns the minute component.
func (t Time) Minute() int { return t.inner.Minute() }

// Second returns the second component.
func (t Time) Second() int { return t.inner.Second() }

// Nanosecond returns the nanosecond component.
func (t Time) Nanosecond() int { return t.inner.Nanosecond() }

// Location returns the timezone, or nil if none was specified.
func (t Time) Location() *time.Location {
	if !t.hasTz {
		return nil
	}
	return t.inner.Location()
}

// IsZero reports whether the Time is the zero value.
func (t Time) IsZero() bool {
	return t.inner.IsZero()
}

func (t Time) string() string {
	if t.inner.IsZero() {
		return ""
	}
	s := t.inner.Format(timeLayout)
	if !t.hasTz {
		return stripTzSuffix(s)
	}
	return s
}

func (t Time) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	if t.inner.IsZero() {
		return nil
	}
	return e.EncodeElement(t.string(), start)
}

func (t *Time) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var s string
	if err := d.DecodeElement(&s, &start); err != nil {
		return err
	}
	if s == "" {
		return nil
	}
	return t.fromString(s)
}

func (t Time) MarshalXMLAttr(name xml.Name) (xml.Attr, error) {
	if t.inner.IsZero() {
		return xml.Attr{}, nil
	}
	return xml.Attr{Name: name, Value: t.string()}, nil
}

func (t *Time) UnmarshalXMLAttr(attr xml.Attr) error {
	if attr.Value == "" {
		return nil
	}
	return t.fromString(attr.Value)
}

func (t *Time) fromString(s string) error {
	hasTz := hasTzSuffix(s)
	if !hasTz {
		s += "Z"
	}
	parsed, err := time.Parse(timeLayout, s)
	if err != nil {
		return err
	}
	t.inner = parsed
	t.hasTz = hasTz
	return nil
}

// Duration is an XSD duration string (e.g. "P1Y2M3DT4H5M6S").
// XSD durations can express years and months which have no fixed
// length in seconds, so this is kept as a string.
type Duration string

// helpers

// parseDateTimeString parses an XSD dateTime string.
func parseDateTimeString(s, layout string) (time.Time, bool, error) {
	hasTz := false
	// Check time portion for TZ indicator.
	if idx := strings.Index(s, "T"); idx >= 0 {
		timePart := s[idx+1:]
		hasTz = hasTzSuffix(timePart)
	}
	if !hasTz {
		s += "Z"
	}
	t, err := time.Parse(layout, s)
	return t, hasTz, err
}

// parseDateString parses an XSD date string.
func parseDateString(s, layout string) (time.Time, bool, error) {
	// For date-only strings, TZ is indicated by Z or +/- after the date.
	// Date format is always YYYY-MM-DD, so check after position 10.
	hasTz := len(s) > 10
	if !hasTz {
		s += "Z"
	}
	t, err := time.Parse(layout, s)
	return t, hasTz, err
}

func hasTzSuffix(s string) bool {
	if s == "" {
		return false
	}
	return strings.HasSuffix(s, "Z") ||
		strings.ContainsAny(s[len(s)/2:], "+-")
}

func stripTzSuffix(s string) string {
	// Remove Z, +HH:MM, or -HH:MM suffix.
	for i := len(s) - 1; i >= 0; i-- {
		switch s[i] {
		case 'Z':
			return s[:i]
		case '+', '-':
			return s[:i]
		}
	}
	return s
}
