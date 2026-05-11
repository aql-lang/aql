package nativemod

import (
	"testing"
	"time"

	"github.com/metsitaba/voxgig-exp/lang/engine"
	"github.com/metsitaba/voxgig-exp/lang/native"
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
	r, err := engine.DefaultRegistry(native.Register)
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

// --- Duration Construction ---

func TestTimeDurYears(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, callTimeDot("years", engine.NewInteger(2)))
	cd, ok := result[0].AsCalDuration()
	if !ok || cd.Years != 2 || cd.Months != 0 || cd.Days != 0 {
		t.Errorf("2 years = %+v, want {2 0 0}", cd)
	}
}

func TestTimeDurMonths(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, callTimeDot("months", engine.NewInteger(6)))
	cd, ok := result[0].AsCalDuration()
	if !ok || cd.Months != 6 {
		t.Errorf("6 months = %+v, want {0 6 0}", cd)
	}
}

func TestTimeDurWeeks(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, callTimeDot("weeks", engine.NewInteger(2)))
	cd, ok := result[0].AsCalDuration()
	if !ok || cd.Days != 14 {
		t.Errorf("2 weeks = %+v, want {0 0 14}", cd)
	}
}

func TestTimeDurDays(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, callTimeDot("days", engine.NewInteger(30)))
	cd, ok := result[0].AsCalDuration()
	if !ok || cd.Days != 30 {
		t.Errorf("30 days = %+v, want {0 0 30}", cd)
	}
}

func TestTimeDurHours(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, callTimeDot("hours", engine.NewInteger(3)))
	d, ok := result[0].AsClkDuration()
	if !ok || d != 3*time.Hour {
		t.Errorf("3 hours = %v, want %v", d, 3*time.Hour)
	}
}

func TestTimeDurMinutes(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, callTimeDot("minutes", engine.NewInteger(90)))
	d, ok := result[0].AsClkDuration()
	if !ok || d != 90*time.Minute {
		t.Errorf("90 minutes = %v, want %v", d, 90*time.Minute)
	}
}

func TestTimeDurSeconds(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, callTimeDot("seconds", engine.NewInteger(30)))
	d, ok := result[0].AsClkDuration()
	if !ok || d != 30*time.Second {
		t.Errorf("30 seconds = %v, want %v", d, 30*time.Second)
	}
}

func TestTimeDurMs(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, callTimeDot("ms", engine.NewInteger(500)))
	d, ok := result[0].AsClkDuration()
	if !ok || d != 500*time.Millisecond {
		t.Errorf("500 ms = %v, want %v", d, 500*time.Millisecond)
	}
}

func TestTimeCalDur(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, callTimeDot("cal-dur", engine.NewInteger(1), engine.NewInteger(6), engine.NewInteger(15)))
	cd, ok := result[0].AsCalDuration()
	if !ok || cd.Years != 1 || cd.Months != 6 || cd.Days != 15 {
		t.Errorf("cal-dur 1 6 15 = %+v, want {1 6 15}", cd)
	}
}

func TestTimeDurationParseISO(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, callTimeDot("duration", engine.NewString("P1Y6M")))
	cd, ok := result[0].AsCalDuration()
	if !ok || cd.Years != 1 || cd.Months != 6 {
		t.Errorf("P1Y6M = %+v, want {1 6 0}", cd)
	}
}

func TestTimeDurationParseISOTime(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, callTimeDot("duration", engine.NewString("PT2H30M")))
	d, ok := result[0].AsClkDuration()
	if !ok || d != 2*time.Hour+30*time.Minute {
		t.Errorf("PT2H30M = %v, want %v", d, 2*time.Hour+30*time.Minute)
	}
}

// --- Duration Extraction ---

func TestTimeTotalHours(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, callTimeDot("total-hours", engine.NewClkDuration(90*time.Minute)))
	v, _ := result[0].AsNumber()
	if v != 1.5 {
		t.Errorf("total-hours(90min) = %v, want 1.5", v)
	}
}

func TestTimeTotalMinutes(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, callTimeDot("total-minutes", engine.NewClkDuration(2*time.Hour)))
	v, _ := result[0].AsNumber()
	if v != 120.0 {
		t.Errorf("total-minutes(2h) = %v, want 120", v)
	}
}

func TestTimeTotalSeconds(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, callTimeDot("total-seconds", engine.NewClkDuration(90*time.Second)))
	v, _ := result[0].AsNumber()
	if v != 90.0 {
		t.Errorf("total-seconds(90s) = %v, want 90", v)
	}
}

func TestTimeTotalMs(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, callTimeDot("total-ms", engine.NewClkDuration(2*time.Second+500*time.Millisecond)))
	v, _ := result[0].AsNumber()
	if v != 2500.0 {
		t.Errorf("total-ms(2.5s) = %v, want 2500", v)
	}
}

func TestTimeDurYearsExtract(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, callTimeDot("dur-years", engine.NewCalDuration(1, 6, 15)))
	v, _ := result[0].AsInteger()
	if v != 1 {
		t.Errorf("dur-years = %d, want 1", v)
	}
}

func TestTimeDurMonthsExtract(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, callTimeDot("dur-months", engine.NewCalDuration(1, 6, 15)))
	v, _ := result[0].AsInteger()
	if v != 6 {
		t.Errorf("dur-months = %d, want 6", v)
	}
}

func TestTimeDurDaysExtract(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, callTimeDot("dur-days", engine.NewCalDuration(1, 6, 15)))
	v, _ := result[0].AsInteger()
	if v != 15 {
		t.Errorf("dur-days = %d, want 15", v)
	}
}

func TestTimeDurSign(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, callTimeDot("dur-sign", engine.NewCalDuration(1, 0, 0)))
	v, _ := result[0].AsInteger()
	if v != 1 {
		t.Errorf("dur-sign(+) = %d, want 1", v)
	}
	result = runAQL(t, r, callTimeDot("dur-sign", engine.NewCalDuration(-1, 0, 0)))
	v, _ = result[0].AsInteger()
	if v != -1 {
		t.Errorf("dur-sign(-) = %d, want -1", v)
	}
	result = runAQL(t, r, callTimeDot("dur-sign", engine.NewCalDuration(0, 0, 0)))
	v, _ = result[0].AsInteger()
	if v != 0 {
		t.Errorf("dur-sign(0) = %d, want 0", v)
	}
}

// --- Arithmetic ---

func TestTimeUntil(t *testing.T) {
	r := timeRegistry(t)
	d1 := engine.NewDate(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	d2 := engine.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))
	result := runAQL(t, r, callTimeDot("until", d1, d2))
	cd, ok := result[0].AsCalDuration()
	if !ok || cd.Years != 0 || cd.Months != 2 || cd.Days != 14 {
		t.Errorf("until = %+v, want {0 2 14}", cd)
	}
}

func TestTimeSince(t *testing.T) {
	r := timeRegistry(t)
	d1 := engine.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))
	d2 := engine.NewDate(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	result := runAQL(t, r, callTimeDot("since", d1, d2))
	cd, ok := result[0].AsCalDuration()
	if !ok || cd.Years != 0 || cd.Months != 2 || cd.Days != 14 {
		t.Errorf("since = %+v, want {0 2 14}", cd)
	}
}

func TestTimeDiffInstants(t *testing.T) {
	r := timeRegistry(t)
	i1 := engine.NewInstant(time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC))
	i2 := engine.NewInstant(time.Date(2024, 1, 1, 12, 30, 0, 0, time.UTC))
	result := runAQL(t, r, callTimeDot("diff", i1, i2))
	d, ok := result[0].AsClkDuration()
	if !ok || d != 2*time.Hour+30*time.Minute {
		t.Errorf("diff = %v, want 2h30m", d)
	}
}

// --- Comparison extended ---

func TestTimeCompare(t *testing.T) {
	r := timeRegistry(t)
	d1 := engine.NewDate(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	d2 := engine.NewDate(time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC))

	result := runAQL(t, r, callTimeDot("compare", d1, d2))
	v, _ := result[0].AsInteger()
	if v != -1 {
		t.Errorf("compare(d1, d2) = %d, want -1", v)
	}

	result = runAQL(t, r, callTimeDot("compare", d1, d1))
	v, _ = result[0].AsInteger()
	if v != 0 {
		t.Errorf("compare(d1, d1) = %d, want 0", v)
	}
}

func TestTimeBetween(t *testing.T) {
	r := timeRegistry(t)
	d := engine.NewDate(time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC))
	start := engine.NewDate(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	end := engine.NewDate(time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC))

	result := runAQL(t, r, callTimeDot("between?", d, start, end))
	b, _ := result[0].AsBoolean()
	if !b {
		t.Error("expected between? = true")
	}
}

func TestTimeEarliest(t *testing.T) {
	r := timeRegistry(t)
	d1 := engine.NewDate(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	d2 := engine.NewDate(time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC))
	result := runAQL(t, r, callTimeDot("earliest", d1, d2))
	got := result[0].AsDate()
	if got.Year() != 2024 || got.Month() != 1 || got.Day() != 1 {
		t.Errorf("earliest = %v, want 2024-01-01", got)
	}
}

func TestTimeLatest(t *testing.T) {
	r := timeRegistry(t)
	d1 := engine.NewDate(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	d2 := engine.NewDate(time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC))
	result := runAQL(t, r, callTimeDot("latest", d1, d2))
	got := result[0].AsDate()
	if got.Year() != 2024 || got.Month() != 12 || got.Day() != 31 {
		t.Errorf("latest = %v, want 2024-12-31", got)
	}
}

// --- Conversion ---

func TestTimeToDate(t *testing.T) {
	r := timeRegistry(t)
	dt := engine.NewDateTime(time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC))
	result := runAQL(t, r, callTimeDot("to-date", dt))
	got := result[0].AsDate()
	if got.Hour() != 0 || got.Day() != 15 {
		t.Errorf("to-date = %v, want midnight 2024-03-15", got)
	}
}

func TestTimeToTimeOfDay(t *testing.T) {
	r := timeRegistry(t)
	dt := engine.NewDateTime(time.Date(2024, 3, 15, 14, 30, 0, 0, time.UTC))
	result := runAQL(t, r, callTimeDot("to-time-of-day", dt))
	tod := result[0].AsTimeOfDay()
	if tod != 14*time.Hour+30*time.Minute {
		t.Errorf("to-time-of-day = %v, want 14h30m", tod)
	}
}

func TestTimeToDatetime(t *testing.T) {
	r := timeRegistry(t)
	d := engine.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))
	result := runAQL(t, r, callTimeDot("to-datetime", d))
	if !result[0].VType.Matches(engine.TDateTime) {
		t.Errorf("to-datetime type = %s, want DateTime", result[0].VType)
	}
}

func TestTimeToInstant(t *testing.T) {
	r := timeRegistry(t)
	dt := engine.NewDateTime(time.Date(2024, 3, 15, 10, 0, 0, 0, time.UTC))
	tz := engine.NewTimezone(time.UTC)
	result := runAQL(t, r, callTimeDot("to-instant", dt, tz))
	if !result[0].VType.Matches(engine.TInstant) {
		t.Errorf("to-instant type = %s, want Instant", result[0].VType)
	}
}

func TestTimeToLocal(t *testing.T) {
	r := timeRegistry(t)
	ins := engine.NewInstant(time.Date(2024, 3, 15, 10, 0, 0, 0, time.UTC))
	tz := engine.NewTimezone(time.UTC)
	result := runAQL(t, r, callTimeDot("to-local", ins, tz))
	if !result[0].VType.Matches(engine.TDateTime) {
		t.Errorf("to-local type = %s, want DateTime", result[0].VType)
	}
}

func TestTimeToUtc(t *testing.T) {
	r := timeRegistry(t)
	ins := engine.NewInstant(time.Date(2024, 3, 15, 10, 0, 0, 0, time.UTC))
	result := runAQL(t, r, callTimeDot("to-utc", ins))
	if !result[0].VType.Matches(engine.TDateTime) {
		t.Errorf("to-utc type = %s, want DateTime", result[0].VType)
	}
}

// --- Rounding ---

func TestTimeStartOf(t *testing.T) {
	r := timeRegistry(t)
	d := engine.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))

	result := runAQL(t, r, callTimeDot("start-of", d, engine.NewString("month")))
	got := result[0].AsDate()
	if got.Day() != 1 || got.Month() != 3 {
		t.Errorf("start-of month = %v, want 2024-03-01", got)
	}

	result = runAQL(t, r, callTimeDot("start-of", d, engine.NewString("year")))
	got = result[0].AsDate()
	if got.Month() != 1 || got.Day() != 1 {
		t.Errorf("start-of year = %v, want 2024-01-01", got)
	}

	result = runAQL(t, r, callTimeDot("start-of", d, engine.NewString("quarter")))
	got = result[0].AsDate()
	if got.Month() != 1 || got.Day() != 1 {
		t.Errorf("start-of quarter = %v, want 2024-01-01", got)
	}
}

func TestTimeEndOf(t *testing.T) {
	r := timeRegistry(t)
	d := engine.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))

	result := runAQL(t, r, callTimeDot("end-of", d, engine.NewString("month")))
	got := result[0].AsDate()
	if got.Day() != 31 || got.Month() != 3 {
		t.Errorf("end-of month = %v, want 2024-03-31", got)
	}

	result = runAQL(t, r, callTimeDot("end-of", d, engine.NewString("year")))
	got = result[0].AsDate()
	if got.Month() != 12 || got.Day() != 31 {
		t.Errorf("end-of year = %v, want 2024-12-31", got)
	}
}

// --- Timezone ---

func TestTimeTzUtc(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, callTimeDot("tz-utc"))
	loc := result[0].AsTimezone()
	if loc == nil || loc.String() != "UTC" {
		t.Errorf("tz-utc = %v, want UTC", loc)
	}
}

func TestTimeTzName(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, callTimeDot("tz-name", engine.NewTimezone(time.UTC)))
	s, _ := result[0].AsString()
	if s != "UTC" {
		t.Errorf("tz-name = %q, want UTC", s)
	}
}

func TestTimeTzOffset(t *testing.T) {
	r := timeRegistry(t)
	ins := engine.NewInstant(time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC))
	tz := engine.NewTimezone(time.UTC)
	result := runAQL(t, r, callTimeDot("tz-offset", ins, tz))
	s, _ := result[0].AsString()
	if s != "+00:00" {
		t.Errorf("tz-offset UTC = %q, want +00:00", s)
	}
}

// --- Parsing ---

func TestTimeParseDate(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, callTimeDot("parse-date", engine.NewString("15/03/2024"), engine.NewString("02/01/2006")))
	got := result[0].AsDate()
	if got.Year() != 2024 || got.Month() != 3 || got.Day() != 15 {
		t.Errorf("parse-date = %v, want 2024-03-15", got)
	}
}

func TestTimeParseDatetime(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, callTimeDot("parse-datetime", engine.NewString("Mar 15, 2024 2:30PM"), engine.NewString("Jan 02, 2006 3:04PM")))
	got := result[0].AsDateTime()
	if got.Hour() != 14 || got.Minute() != 30 {
		t.Errorf("parse-datetime = %v, want 14:30", got)
	}
}

func TestTimeAutoDate(t *testing.T) {
	r := timeRegistry(t)
	tests := []struct {
		input string
		year  int
		month time.Month
		day   int
	}{
		{"2024-03-15", 2024, 3, 15},
		{"03/15/2024", 2024, 3, 15},
		{"March 15, 2024", 2024, 3, 15},
	}
	for _, tt := range tests {
		result := runAQL(t, r, callTimeDot("auto-date", engine.NewString(tt.input)))
		got := result[0].AsDate()
		if got.Year() != tt.year || got.Month() != tt.month || got.Day() != tt.day {
			t.Errorf("auto-date(%q) = %v, want %d-%02d-%02d", tt.input, got, tt.year, tt.month, tt.day)
		}
	}
}

// --- add/sub with temporal types (core engine) ---

func TestAddDateCalDuration(t *testing.T) {
	r, err := engine.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	e := engine.New(r)
	// "2024-01-31" date add 1 months → need date + CalDuration on stack
	d := engine.NewDate(time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC))
	cd := engine.NewCalDuration(0, 1, 0)
	result, err := e.Run([]engine.Value{cd, d, engine.NewWord("add")})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	got := result[0].AsDate()
	// Go normalizes: Jan 31 + 1 month = March 2
	want := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("date add CalDur = %v, want %v", got, want)
	}
}

func TestSubDateCalDuration(t *testing.T) {
	r, err := engine.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	e := engine.New(r)
	d := engine.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))
	cd := engine.NewCalDuration(0, 1, 0)
	result, err := e.Run([]engine.Value{cd, d, engine.NewWord("sub")})
	if err != nil {
		t.Fatalf("sub: %v", err)
	}
	got := result[0].AsDate()
	want := time.Date(2024, 2, 15, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("date sub CalDur = %v, want %v", got, want)
	}
}

func TestAddInstantClkDuration(t *testing.T) {
	r, err := engine.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	e := engine.New(r)
	ins := engine.NewInstant(time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC))
	dur := engine.NewClkDuration(30 * time.Minute)
	result, err := e.Run([]engine.Value{dur, ins, engine.NewWord("add")})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	got := result[0].AsInstant()
	if got.Minute() != 30 {
		t.Errorf("instant add 30min = %v, want 10:30", got)
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
