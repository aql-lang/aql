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
		"date", "now", "today",
		"year", "month", "day", "weekday", "year-day",
		"before?", "after?", "equal?",
		"to-string", "format",
		"add-days", "add-months", "add-years",
		"days-in-month", "leap-year?",
	}
	for _, name := range expected {
		if _, ok := timeExport.Get(name); !ok {
			t.Errorf("expected %q in time export map", name)
		}
	}
}

// --- Construction: time.date ---

func TestTimeDateParse(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, []engine.Value{
		engine.NewString("2024-03-15"),
		engine.NewWord("("), engine.NewWord("time"), engine.NewWord("get"), engine.NewWord("date"), engine.NewWord(")"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	d := result[0].AsDate()
	if d.Year() != 2024 || d.Month() != time.March || d.Day() != 15 {
		t.Errorf("expected 2024-03-15, got %v", d)
	}
}

// --- Extraction ---

func TestTimeYear(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, []engine.Value{
		engine.NewString("2024-03-15"),
		engine.NewWord("("), engine.NewWord("time"), engine.NewWord("get"), engine.NewWord("date"), engine.NewWord(")"),
		engine.NewWord("("), engine.NewWord("time"), engine.NewWord("get"), engine.NewWord("year"), engine.NewWord(")"),
	})
	if result[0].AsInteger() != 2024 {
		t.Errorf("year = %v, want 2024", result[0])
	}
}

func TestTimeMonth(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, []engine.Value{
		engine.NewString("2024-03-15"),
		engine.NewWord("("), engine.NewWord("time"), engine.NewWord("get"), engine.NewWord("date"), engine.NewWord(")"),
		engine.NewWord("("), engine.NewWord("time"), engine.NewWord("get"), engine.NewWord("month"), engine.NewWord(")"),
	})
	if result[0].AsInteger() != 3 {
		t.Errorf("month = %v, want 3", result[0])
	}
}

func TestTimeDay(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, []engine.Value{
		engine.NewString("2024-03-15"),
		engine.NewWord("("), engine.NewWord("time"), engine.NewWord("get"), engine.NewWord("date"), engine.NewWord(")"),
		engine.NewWord("("), engine.NewWord("time"), engine.NewWord("get"), engine.NewWord("day"), engine.NewWord(")"),
	})
	if result[0].AsInteger() != 15 {
		t.Errorf("day = %v, want 15", result[0])
	}
}

func TestTimeWeekday(t *testing.T) {
	r := timeRegistry(t)
	// 2024-03-15 is a Friday → ISO weekday 5
	result := runAQL(t, r, []engine.Value{
		engine.NewString("2024-03-15"),
		engine.NewWord("("), engine.NewWord("time"), engine.NewWord("get"), engine.NewWord("date"), engine.NewWord(")"),
		engine.NewWord("("), engine.NewWord("time"), engine.NewWord("get"), engine.NewWord("weekday"), engine.NewWord(")"),
	})
	if result[0].AsInteger() != 5 {
		t.Errorf("weekday = %v, want 5 (Friday)", result[0])
	}
}

func TestTimeYearDay(t *testing.T) {
	r := timeRegistry(t)
	// 2024-03-15: Jan=31, Feb=29 (leap), Mar 1-15 = 15 → 31+29+15 = 75
	result := runAQL(t, r, []engine.Value{
		engine.NewString("2024-03-15"),
		engine.NewWord("("), engine.NewWord("time"), engine.NewWord("get"), engine.NewWord("date"), engine.NewWord(")"),
		engine.NewWord("("), engine.NewWord("time"), engine.NewWord("get"), engine.NewWord("year-day"), engine.NewWord(")"),
	})
	if result[0].AsInteger() != 75 {
		t.Errorf("year-day = %v, want 75", result[0])
	}
}

// --- Comparison ---

func TestTimeBeforeAfter(t *testing.T) {
	r := timeRegistry(t)
	d1 := engine.NewDate(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	d2 := engine.NewDate(time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC))

	// d1 d2 time.before?
	result := runAQL(t, r, []engine.Value{
		d1, d2,
		engine.NewWord("("), engine.NewWord("time"), engine.NewWord("get"), engine.NewWord("before?"), engine.NewWord(")"),
	})
	if !result[0].AsBoolean() {
		t.Error("expected 2024-01-01 before? 2024-12-31 = true")
	}

	// d1 d2 time.after?
	result = runAQL(t, r, []engine.Value{
		d1, d2,
		engine.NewWord("("), engine.NewWord("time"), engine.NewWord("get"), engine.NewWord("after?"), engine.NewWord(")"),
	})
	if result[0].AsBoolean() {
		t.Error("expected 2024-01-01 after? 2024-12-31 = false")
	}
}

func TestTimeEqual(t *testing.T) {
	r := timeRegistry(t)
	d := engine.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))
	result := runAQL(t, r, []engine.Value{
		d, d,
		engine.NewWord("("), engine.NewWord("time"), engine.NewWord("get"), engine.NewWord("equal?"), engine.NewWord(")"),
	})
	if !result[0].AsBoolean() {
		t.Error("expected same date equal? = true")
	}
}

// --- Formatting ---

func TestTimeToString(t *testing.T) {
	r := timeRegistry(t)
	d := engine.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))
	result := runAQL(t, r, []engine.Value{
		d,
		engine.NewWord("("), engine.NewWord("time"), engine.NewWord("get"), engine.NewWord("to-string"), engine.NewWord(")"),
	})
	if result[0].AsString() != "2024-03-15" {
		t.Errorf("to-string = %q, want %q", result[0].AsString(), "2024-03-15")
	}
}

func TestTimeFormat(t *testing.T) {
	r := timeRegistry(t)
	d := engine.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))
	result := runAQL(t, r, []engine.Value{
		d, engine.NewString("02 Jan 2006"),
		engine.NewWord("("), engine.NewWord("time"), engine.NewWord("get"), engine.NewWord("format"), engine.NewWord(")"),
	})
	if result[0].AsString() != "15 Mar 2024" {
		t.Errorf("format = %q, want %q", result[0].AsString(), "15 Mar 2024")
	}
}

// --- Arithmetic ---

func TestTimeAddDays(t *testing.T) {
	r := timeRegistry(t)
	d := engine.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))
	result := runAQL(t, r, []engine.Value{
		d, engine.NewInteger(10),
		engine.NewWord("("), engine.NewWord("time"), engine.NewWord("get"), engine.NewWord("add-days"), engine.NewWord(")"),
	})
	got := result[0].AsDate()
	want := time.Date(2024, 3, 25, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("add-days(2024-03-15, 10) = %v, want %v", got, want)
	}
}

func TestTimeAddMonths(t *testing.T) {
	r := timeRegistry(t)
	// 2024-01-31 + 1 month → 2024-03-02 or 2024-02-29 (Go's AddDate normalizes)
	d := engine.NewDate(time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC))
	result := runAQL(t, r, []engine.Value{
		d, engine.NewInteger(1),
		engine.NewWord("("), engine.NewWord("time"), engine.NewWord("get"), engine.NewWord("add-months"), engine.NewWord(")"),
	})
	got := result[0].AsDate()
	// Go normalizes: Jan 31 + 1 month = March 2 (Feb has 29 days in 2024)
	want := time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("add-months(2024-01-31, 1) = %v, want %v", got, want)
	}
}

func TestTimeAddYears(t *testing.T) {
	r := timeRegistry(t)
	d := engine.NewDate(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC))
	result := runAQL(t, r, []engine.Value{
		d, engine.NewInteger(2),
		engine.NewWord("("), engine.NewWord("time"), engine.NewWord("get"), engine.NewWord("add-years"), engine.NewWord(")"),
	})
	got := result[0].AsDate()
	want := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("add-years(2024-03-15, 2) = %v, want %v", got, want)
	}
}

// --- Info ---

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
		result := runAQL(t, r, []engine.Value{
			d,
			engine.NewWord("("), engine.NewWord("time"), engine.NewWord("get"), engine.NewWord("days-in-month"), engine.NewWord(")"),
		})
		if result[0].AsInteger() != tt.want {
			t.Errorf("days-in-month(%s) = %d, want %d", tt.date, result[0].AsInteger(), tt.want)
		}
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
		result := runAQL(t, r, []engine.Value{
			d,
			engine.NewWord("("), engine.NewWord("time"), engine.NewWord("get"), engine.NewWord("leap-year?"), engine.NewWord(")"),
		})
		if result[0].AsBoolean() != tt.want {
			t.Errorf("leap-year?(%s) = %v, want %v", tt.date, result[0].AsBoolean(), tt.want)
		}
	}
}

// --- Now/Today produce valid dates ---

func TestTimeNow(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("("), engine.NewWord("time"), engine.NewWord("get"), engine.NewWord("now"), engine.NewWord(")"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	d := result[0].AsDate()
	if d.Year() < 2024 {
		t.Errorf("now year = %d, expected >= 2024", d.Year())
	}
}

func TestTimeToday(t *testing.T) {
	r := timeRegistry(t)
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("("), engine.NewWord("time"), engine.NewWord("get"), engine.NewWord("today"), engine.NewWord(")"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	d := result[0].AsDate()
	if d.Year() < 2024 {
		t.Errorf("today year = %d, expected >= 2024", d.Year())
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
