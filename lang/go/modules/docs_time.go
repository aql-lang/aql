package modules

func init() {
	registerDocs("aql:time-util", map[string]string{
		// unix epoch <-> Instant
		"unix":       "Unix seconds to an Instant.",
		"unix-ms":    "Unix milliseconds to an Instant.",
		"unix-ns":    "Unix nanoseconds to an Instant.",
		"to-unix":    "Instant to Unix seconds.",
		"to-unix-ms": "Instant to Unix milliseconds.",

		// current time (frozen clock in specs)
		"now":       "Current instant from the clock.",
		"now-local": "Current local DateTime from the clock.",
		"today":     "Current local date.",
		"today-utc": "Current date in UTC.",

		// Date field extraction
		"year":          "Calendar year of a Date.",
		"month":         "Month number (1-12) of a Date.",
		"day":           "Day of month of a Date.",
		"weekday":       "ISO weekday of a Date (Mon=1..Sun=7).",
		"weekday-name":  "Weekday name of a Date.",
		"month-name":    "Month name of a Date.",
		"year-day":      "Ordinal day of the year of a Date.",
		"iso-week":      "ISO-8601 week number of a Date.",
		"quarter":       "Calendar quarter (1-4) of a Date.",
		"days-in-month": "Number of days in the Date's month.",
		"days-in-year":  "Number of days in the Date's year.",
		"is-leap-year":  "Is the Date's year a leap year?",

		// Date formatting
		"to-iso":    "ISO-8601 string of a Date.",
		"to-string": "Default string form of a Date.",
		"format":    "Format a Date using a Go layout string.",

		// Date arithmetic
		"add-days":   "Shift a Date by n days.",
		"add-months": "Shift a Date by n months.",
		"add-years":  "Shift a Date by n years.",

		// duration constructors — calendar
		"years":   "n years as a CalendarDuration.",
		"months":  "n months as a CalendarDuration.",
		"weeks":   "n weeks as a duration (in days).",
		"days":    "n days as a duration.",
		"cal-dur": "Years/months/days to a CalendarDuration.",

		// duration constructors — clock
		"hours":   "n hours as a ClockDuration.",
		"minutes": "n minutes as a ClockDuration.",
		"seconds": "n seconds as a ClockDuration.",
		"ms":      "n milliseconds as a ClockDuration.",
		"us":      "n microseconds as a ClockDuration.",
		"ns":      "n nanoseconds as a ClockDuration.",

		// duration extraction
		"total-hours":   "ClockDuration as total hours (Float).",
		"total-minutes": "ClockDuration as total minutes (Float).",
		"total-seconds": "ClockDuration as total seconds (Float).",
		"total-ms":      "ClockDuration as total milliseconds.",
		"dur-years":     "Years field of a CalendarDuration.",
		"dur-months":    "Months field of a CalendarDuration.",
		"dur-days":      "Days field of a CalendarDuration.",
		"dur-sign":      "Sign (-1/0/1) of a duration.",

		// differences
		"until":   "Calendar span from one Date to another.",
		"since":   "Calendar span between two Dates (later, earlier).",
		"diff":    "Clock duration between two Instants.",
		"elapsed": "Clock duration elapsed since a past Instant.",

		// comparison & ordering
		"compare":    "Order two Dates as -1/0/1.",
		"is-before":  "Is the first Date before the second?",
		"is-after":   "Is the first Date after the second?",
		"is-equal":   "Are two Dates equal?",
		"is-between": "Is a Date within [start, end]?",
		"earliest":   "The earlier of two Dates.",
		"latest":     "The later of two Dates.",

		// rounding
		"start-of": "First instant of the given unit of a Date.",
		"end-of":   "Last instant of the given unit of a Date.",

		// arithmetic word extensions (transplanted onto core add/sub/div/mod at import)
		"add": "Date/DateTime/Instant plus a duration, or two same-leaf durations summed (word extension of core add).",
		"sub": "A point minus a duration, two same-leaf points differenced to a duration, or two durations subtracted (word extension of core sub).",
		"div": "ClockDuration divided by ClockDuration, a dimensionless Float ratio (word extension of core div).",
		"mod": "ClockDuration modulo ClockDuration, the remainder as a ClockDuration (word extension of core mod).",

		// conversions
		"to-date":        "Date part of a DateTime/Instant.",
		"to-datetime":    "Date to DateTime at midnight.",
		"to-time-of-day": "TimeOfDay part of a DateTime/Instant.",
		"to-instant":     "DateTime plus zone to an Instant.",
		"to-local":       "Instant plus zone to a local DateTime.",
		"to-utc":         "Instant to a UTC DateTime.",

		// timezones
		"tz":        "IANA zone name to a Timezone.",
		"tz-utc":    "The UTC timezone.",
		"tz-local":  "The host-local timezone.",
		"tz-name":   "Name of a Timezone.",
		"tz-offset": "Offset of an Instant in a Timezone.",
		"is-dst":    "Is DST active for an Instant in a zone?",

		// async / timer words
		"sleep":    "Pause for a duration, then continue.",
		"timeout":  "Schedule a body to run after a delay.",
		"interval": "Schedule a body to run repeatedly.",
		"cancel":   "Cancel a scheduled timer or interval.",
		"await":    "Run bodies in parallel and collect results.",
	})

	// Two things trip callers here and neither shows in a signature: the
	// INSTANT vs DATE split (a point on the timeline vs a calendar day —
	// most calendar words take a Date, so an Instant needs `to-date`
	// first), and the AQL swap convention on the comparison words. Every
	// line below was run to get its result.
	registerExamples("aql:time-util", map[string][]string{
		"now":       {`TimeUtil.now                                    ;# an Instant — a point on the timeline, always UTC`},
		"today-utc": {`TimeUtil.today-utc                              ;# a Date — a calendar day, no time of day`},
		"unix":      {`TimeUtil.unix 1700000000                        ;# 2023-11-14T22:13:20Z — seconds to an Instant`},
		"to-unix":   {`TimeUtil.to-unix (TimeUtil.unix 1700000000)     ;# 1700000000`},
		"to-date": {
			`def d (TimeUtil.to-date (TimeUtil.unix 1700000000))   ;# 2023-11-14 — Instant to Date`,
			`;# Needed before the calendar words: most of them take a Date.`,
		},
		"to-iso": {
			`def d (TimeUtil.to-date (TimeUtil.unix 1700000000))`,
			`TimeUtil.to-iso d                                ;# '2023-11-14'`,
		},
		"format": {
			`def d (TimeUtil.to-date (TimeUtil.unix 1700000000))`,
			`TimeUtil.format "2006-01-02" d                   ;# '2023-11-14'`,
			`;# The LAYOUT comes first, and it is Go's reference-time style`,
			`;# ("2006-01-02 15:04:05"), not strftime's %Y-%m-%d.`,
		},
		"year":         {`TimeUtil.year (TimeUtil.to-date (TimeUtil.unix 1700000000))          ;# 2023`},
		"weekday-name": {`TimeUtil.weekday-name (TimeUtil.to-date (TimeUtil.unix 1700000000))  ;# 'Tuesday'`},
		"add-days": {
			`def d (TimeUtil.to-date (TimeUtil.unix 1700000000))`,
			`TimeUtil.add-days 10 d                           ;# 2023-11-24 — the COUNT comes first`,
		},
		"start-of": {
			`def d (TimeUtil.to-date (TimeUtil.unix 1700000000))`,
			`TimeUtil.start-of "month" d                      ;# 2023-11-01`,
		},
		"end-of": {
			`def d (TimeUtil.to-date (TimeUtil.unix 1700000000))`,
			`TimeUtil.end-of "month" d                        ;# 2023-11-30`,
		},
		"is-before": {
			`def a (TimeUtil.to-date (TimeUtil.unix 1))`,
			`def b (TimeUtil.to-date (TimeUtil.unix 90000))`,
			`TimeUtil.is-before a b                           ;# false — reads "b is before a"`,
			`a TimeUtil.is-before b                           ;# true — the swap form reads forwards`,
		},
		"earliest": {
			`def a (TimeUtil.to-date (TimeUtil.unix 1))`,
			`def b (TimeUtil.to-date (TimeUtil.unix 90000))`,
			`TimeUtil.earliest a b                            ;# 1970-01-01`,
		},
		"add":           {`TimeUtil.add (TimeUtil.unix 1700000000) (TimeUtil.hours 2)   ;# 2023-11-15T00:13:20Z`},
		"diff":          {`TimeUtil.diff (TimeUtil.unix 1700003600) (TimeUtil.unix 1700000000)   ;# 1h0m0s — a Duration`},
		"total-minutes": {`TimeUtil.total-minutes (TimeUtil.hours 2)        ;# 120.0 — a Duration as a Float`},
		"tz":            {`TimeUtil.tz "America/New_York"                   ;# an IANA zone; TimeUtil.tz-utc for UTC`},
		"tz-offset":     {`TimeUtil.tz-offset (TimeUtil.tz "America/New_York") (TimeUtil.unix 1700000000)   ;# '-05:00'`},
		"await": {
			`TimeUtil.await [[1 add 2] [3 add 4]]             ;# [3 7] — run the bodies concurrently, collect in order`,
		},
		"sleep":    {`TimeUtil.sleep 250                               ;# block this actor for 250ms`},
		"timeout":  {`def t (TimeUtil.timeout 1000 [print "late"])     ;# a Timer; TimeUtil.cancel t stops it`},
		"interval": {`def i (TimeUtil.interval 1000 [print "tick"])     ;# repeats until TimeUtil.cancel i`},
	})
}
