package nativemod

import (
	"testing"
	"time"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

// timeRegistry returns a registry with the aql:time module loaded.
func timeRegistry(t *testing.T) *engine.Registry {
	t.Helper()
	r, err := engine.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if err := InstallTimeExports(r); err != nil {
		t.Fatal(err)
	}
	return r
}

// callTimeDot is a helper to call time.<word> with values on the stack.
func callTimeDot(word string, stackVals ...engine.Value) []engine.Value {
	var input []engine.Value
	input = append(input, stackVals...)
	input = append(input,
		engine.NewWord("("),
		engine.NewWord("time"), engine.NewWord("get"), engine.NewWord(word),
		engine.NewWord(")"),
	)
	return input
}

// --- Module structure ---

func TestTimeModuleExports(t *testing.T) {
	r, err := engine.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	desc, err := BuildTimeModule(r)
	if err != nil {
		t.Fatal(err)
	}
	timeExport, ok := desc.Exports["time"]
	if !ok {
		t.Fatal("expected 'time' export")
	}
	expected := []string{
		"date", "datetime", "instant", "time-of-day", "tz",
		"unix", "unix-ms", "unix-ns",
		"now-local", "today", "today-utc",
		"year", "month", "day", "weekday", "year-day",
		"weekday-name", "month-name", "iso-week", "quarter",
		"days-in-month", "days-in-year", "leap-year?",
		"to-unix", "to-unix-ms",
		"before?", "after?", "equal?",
		"to-string", "format", "to-iso",
		"add-days", "add-months", "add-years",
	}
	for _, name := range expected {
		if _, ok := timeExport.Get(name); !ok {
			t.Errorf("expected %q in time export map", name)
		}
	}
}

// --- Construction ---

func TestTimeDateParse(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, callTimeDot("date", engine.NewString("2024-03-15")))
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	d := result[0].AsDate()
	if d.Year() != 2024 || d.Month() != time.March || d.Day() != 15 {
		t.Errorf("expected 2024-03-15, got %v", d)
	}
}

func TestTimeDateTimeParse(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, callTimeDot("datetime", engine.NewString("2024-03-15T10:30:00")))
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	dt := result[0].AsDateTime()
	if dt.Year() != 2024 || dt.Hour() != 10 || dt.Minute() != 30 {
		t.Errorf("expected 2024-03-15T10:30:00, got %v", dt)
	}
}

func TestTimeInstantParse(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, callTimeDot("instant", engine.NewString("2024-03-15T10:30:00Z")))
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	ins := result[0].AsInstant()
	if ins.Year() != 2024 || ins.Hour() != 10 {
		t.Errorf("expected 2024-03-15T10:30:00Z, got %v", ins)
	}
}

func TestTimeTimeOfDayParse(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, callTimeDot("time-of-day", engine.NewString("14:30:00")))
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	tod := result[0].AsTimeOfDay()
	if tod != 14*time.Hour+30*time.Minute {
		t.Errorf("expected 14h30m, got %v", tod)
	}
}

func TestTimeTz(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, callTimeDot("tz", engine.NewString("UTC")))
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	loc := result[0].AsTimezone()
	if loc == nil || loc.String() != "UTC" {
		t.Errorf("expected UTC timezone, got %v", loc)
	}
}

func TestTimeUnix(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, callTimeDot("unix", engine.NewInteger(1710500000)))
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	ins := result[0].AsInstant()
	if ins.Unix() != 1710500000 {
		t.Errorf("expected unix 1710500000, got %d", ins.Unix())
	}
}

func TestTimeUnixMs(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, callTimeDot("unix-ms", engine.NewInteger(1710500000000)))
	ins := result[0].AsInstant()
	if ins.UnixMilli() != 1710500000000 {
		t.Errorf("expected unix-ms 1710500000000, got %d", ins.UnixMilli())
	}
}

// --- Current Time ---

func TestTimeNowLocal(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, callTimeDot("now-local"))
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if !result[0].VType.Matches(engine.TDateTime) {
		t.Errorf("expected DateTime type, got %s", result[0].VType)
	}
}

func TestTimeToday(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, callTimeDot("today"))
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	d := result[0].AsDate()
	if d.Year() < 2024 {
		t.Errorf("today year = %d, expected >= 2024", d.Year())
	}
}

func TestTimeTodayUtc(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, callTimeDot("today-utc"))
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	d := result[0].AsDate()
	if d.Year() < 2024 {
		t.Errorf("today-utc year = %d, expected >= 2024", d.Year())
	}
}

// --- Extraction ---

func TestTimeYear(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, callTimeDot("year", engine.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))))
	v, _ := result[0].AsInteger()
	if v != 2024 {
		t.Errorf("year = %d, want 2024", v)
	}
}

func TestTimeMonth(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, callTimeDot("month", engine.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))))
	v, _ := result[0].AsInteger()
	if v != 3 {
		t.Errorf("month = %d, want 3", v)
	}
}

func TestTimeDay(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, callTimeDot("day", engine.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))))
	v, _ := result[0].AsInteger()
	if v != 15 {
		t.Errorf("day = %d, want 15", v)
	}
}

func TestTimeWeekday(t *testing.T) {
	r := timeRegistry(t)
	// 2024-03-15 is Friday → ISO weekday 5
	result := runAQL(t, r, callTimeDot("weekday", engine.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))))
	v, _ := result[0].AsInteger()
	if v != 5 {
		t.Errorf("weekday = %d, want 5 (Friday)", v)
	}
}

func TestTimeYearDay(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, callTimeDot("year-day", engine.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))))
	v, _ := result[0].AsInteger()
	if v != 75 {
		t.Errorf("year-day = %d, want 75", v)
	}
}

func TestTimeWeekdayName(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, callTimeDot("weekday-name", engine.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))))
	s, _ := result[0].AsString()
	if s != "Friday" {
		t.Errorf("weekday-name = %q, want %q", s, "Friday")
	}
}

func TestTimeMonthName(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, callTimeDot("month-name", engine.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))))
	s, _ := result[0].AsString()
	if s != "March" {
		t.Errorf("month-name = %q, want %q", s, "March")
	}
}

func TestTimeIsoWeek(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, callTimeDot("iso-week", engine.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))))
	v, _ := result[0].AsInteger()
	if v != 11 {
		t.Errorf("iso-week = %d, want 11", v)
	}
}

func TestTimeQuarter(t *testing.T) {
	r := timeRegistry(t)
	tests := []struct {
		month time.Month
		want  int64
	}{
		{1, 1}, {3, 1}, {4, 2}, {6, 2}, {7, 3}, {9, 3}, {10, 4}, {12, 4},
	}
	for _, tt := range tests {
		result := runAQL(t, r, callTimeDot("quarter", engine.NewDate(time.Date(2024, tt.month, 1, 0, 0, 0, 0, time.UTC))))
		v, _ := result[0].AsInteger()
		if v != tt.want {
			t.Errorf("quarter(month=%d) = %d, want %d", tt.month, v, tt.want)
		}
	}
}

func TestTimeDaysInMonth(t *testing.T) {
	r := timeRegistry(t)
	tests := []struct {
		date string
		want int64
	}{
		{"2024-02-01", 29}, // leap year
		{"2023-02-01", 28},
		{"2024-01-15", 31},
		{"2024-04-10", 30},
	}
	for _, tt := range tests {
		d := engine.NewDate(parseDate(t, tt.date))
		result := runAQL(t, r, callTimeDot("days-in-month", d))
		v, _ := result[0].AsInteger()
		if v != tt.want {
			t.Errorf("days-in-month(%s) = %d, want %d", tt.date, v, tt.want)
		}
	}
}

func TestTimeDaysInYear(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, callTimeDot("days-in-year", engine.NewDate(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))))
	v, _ := result[0].AsInteger()
	if v != 366 {
		t.Errorf("days-in-year(2024) = %d, want 366", v)
	}
}

func TestTimeLeapYear(t *testing.T) {
	r := timeRegistry(t)
	tests := []struct {
		date string
		want bool
	}{
		{"2024-01-01", true},
		{"2023-01-01", false},
		{"2000-01-01", true},
		{"1900-01-01", false},
	}
	for _, tt := range tests {
		d := engine.NewDate(parseDate(t, tt.date))
		result := runAQL(t, r, callTimeDot("leap-year?", d))
		b, _ := result[0].AsBoolean()
		if b != tt.want {
			t.Errorf("leap-year?(%s) = %v, want %v", tt.date, b, tt.want)
		}
	}
}

func TestTimeToUnix(t *testing.T) {
	r := timeRegistry(t)
	ins := engine.NewInstant(time.Unix(1710500000, 0))
	result := runAQL(t, r, callTimeDot("to-unix", ins))
	v, _ := result[0].AsInteger()
	if v != 1710500000 {
		t.Errorf("to-unix = %d, want 1710500000", v)
	}
}

func TestTimeToUnixMs(t *testing.T) {
	r := timeRegistry(t)
	ins := engine.NewInstant(time.UnixMilli(1710500000123))
	result := runAQL(t, r, callTimeDot("to-unix-ms", ins))
	v, _ := result[0].AsInteger()
	if v != 1710500000123 {
		t.Errorf("to-unix-ms = %d, want 1710500000123", v)
	}
}

// --- Comparison ---

func TestTimeBeforeAfter(t *testing.T) {
	r := timeRegistry(t)
	d1 := engine.NewDate(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	d2 := engine.NewDate(time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC))

	result := runAQL(t, r, callTimeDot("before?", d1, d2))
	b, _ := result[0].AsBoolean()
	if !b {
		t.Error("expected 2024-01-01 before? 2024-12-31 = true")
	}

	result = runAQL(t, r, callTimeDot("after?", d1, d2))
	b, _ = result[0].AsBoolean()
	if b {
		t.Error("expected 2024-01-01 after? 2024-12-31 = false")
	}
}

func TestTimeEqual(t *testing.T) {
	r := timeRegistry(t)
	d := engine.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))
	result := runAQL(t, r, callTimeDot("equal?", d, d))
	b, _ := result[0].AsBoolean()
	if !b {
		t.Error("expected same date equal? = true")
	}
}

// --- Formatting ---

func TestTimeToString(t *testing.T) {
	r := timeRegistry(t)
	d := engine.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))
	result := runAQL(t, r, callTimeDot("to-string", d))
	s, _ := result[0].AsString()
	if s != "2024-03-15" {
		t.Errorf("to-string = %q, want %q", s, "2024-03-15")
	}
}

func TestTimeFormat(t *testing.T) {
	r := timeRegistry(t)
	d := engine.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))
	result := runAQL(t, r, callTimeDot("format", d, engine.NewString("02 Jan 2006")))
	s, _ := result[0].AsString()
	if s != "15 Mar 2024" {
		t.Errorf("format = %q, want %q", s, "15 Mar 2024")
	}
}

func TestTimeToIso(t *testing.T) {
	r := timeRegistry(t)
	d := engine.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))
	result := runAQL(t, r, callTimeDot("to-iso", d))
	s, _ := result[0].AsString()
	if s != "2024-03-15" {
		t.Errorf("to-iso = %q, want %q", s, "2024-03-15")
	}
}

// --- Legacy Arithmetic ---

func TestTimeAddDays(t *testing.T) {
	r := timeRegistry(t)
	d := engine.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))
	result := runAQL(t, r, callTimeDot("add-days", d, engine.NewInteger(10)))
	got := result[0].AsDate()
	want := time.Date(2024, 3, 25, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("add-days(2024-03-15, 10) = %v, want %v", got, want)
	}
}

func TestTimeAddMonths(t *testing.T) {
	r := timeRegistry(t)
	d := engine.NewDate(time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC))
	result := runAQL(t, r, callTimeDot("add-months", d, engine.NewInteger(1)))
	got := result[0].AsDate()
	want := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("add-months(2024-01-31, 1) = %v, want %v", got, want)
	}
}

func TestTimeAddYears(t *testing.T) {
	r := timeRegistry(t)
	d := engine.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))
	result := runAQL(t, r, callTimeDot("add-years", d, engine.NewInteger(2)))
	got := result[0].AsDate()
	want := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("add-years(2024-03-15, 2) = %v, want %v", got, want)
	}
}

// --- Standard now word (non-module) ---

func TestNowStandardWord(t *testing.T) {
	r, err := engine.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	e := engine.New(r)
	result, err := e.Run([]engine.Value{engine.NewWord("now")})
	if err != nil {
		t.Fatalf("now: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if !result[0].VType.Matches(engine.TInstant) {
		t.Errorf("now should return Instant, got %s", result[0].VType)
	}
}

// helper
func parseDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("parseDate(%q): %v", s, err)
	}
	return d
}
