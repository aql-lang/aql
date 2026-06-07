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
		"years":   "n years as a CalDuration.",
		"months":  "n months as a CalDuration.",
		"weeks":   "n weeks as a duration (in days).",
		"days":    "n days as a duration.",
		"cal-dur": "Years/months/days to a CalDuration.",

		// duration constructors — clock
		"hours":   "n hours as a ClkDuration.",
		"minutes": "n minutes as a ClkDuration.",
		"seconds": "n seconds as a ClkDuration.",
		"ms":      "n milliseconds as a ClkDuration.",
		"us":      "n microseconds as a ClkDuration.",
		"ns":      "n nanoseconds as a ClkDuration.",

		// duration extraction
		"total-hours":   "ClkDuration as total hours (Float).",
		"total-minutes": "ClkDuration as total minutes (Float).",
		"total-seconds": "ClkDuration as total seconds (Float).",
		"total-ms":      "ClkDuration as total milliseconds.",
		"dur-years":     "Years field of a CalDuration.",
		"dur-months":    "Months field of a CalDuration.",
		"dur-days":      "Days field of a CalDuration.",
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
}
