// Package model contains the PURE, deterministic climate/drought mathematics of Eco Terminal:
// no database, no network, no clock, no LLM. Every function maps numbers to numbers, so the
// whole package can be unit-tested with hand-verifiable values (see docs/simulations/SIMULATION.md).
//
// Missing data is represented by math.NaN() throughout (see Missing / IsMissing).
package model

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// Missing is the marker for "no value" in every series of this package.
var Missing = math.NaN()

// IsMissing reports whether v is the missing-data marker.
func IsMissing(v float64) bool { return math.IsNaN(v) || math.IsInf(v, 0) }

// YM is a calendar month (Month is 1..12).
type YM struct{ Year, Month int }

// ParseYM parses "YYYY-MM".
func ParseYM(s string) (YM, error) {
	t, err := time.Parse("2006-01", s)
	if err != nil {
		return YM{}, fmt.Errorf("invalid month %q (want YYYY-MM)", s)
	}
	return YM{t.Year(), int(t.Month())}, nil
}

func (y YM) String() string { return fmt.Sprintf("%04d-%02d", y.Year, y.Month) }

// Index is a monotonically increasing integer for the month (Jan of year 0 == 0).
func (y YM) Index() int { return y.Year*12 + y.Month - 1 }

// YMFromIndex inverts Index.
func YMFromIndex(i int) YM { return YM{i / 12, i%12 + 1} }

// Add returns the month n months later (n may be negative).
func (y YM) Add(n int) YM { return YMFromIndex(y.Index() + n) }

// DaysIn is the number of days of the month in the proleptic Gregorian calendar.
func (y YM) DaysIn() int {
	return time.Date(y.Year, time.Month(y.Month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// MonthsInclusive is the number of months from a to b inclusive (0 if b is before a).
func MonthsInclusive(a, b YM) int {
	n := b.Index() - a.Index() + 1
	if n < 0 {
		return 0
	}
	return n
}

// DailyRecord is one day of observed (reanalysis) climate. Precip in mm/day, Temp = daily mean 2 m air
// temperature in degC. Either may be Missing. Date is interpreted as a UTC calendar date.
type DailyRecord struct {
	Date   time.Time
	Precip float64
	Temp   float64
}

// Day builds a DailyRecord (convenience for tests and callers).
func Day(y, m, d int, precip, temp float64) DailyRecord {
	return DailyRecord{Date: time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC), Precip: precip, Temp: temp}
}

// Monthly is the aggregation of the daily records of one calendar month.
type Monthly struct {
	YM YM
	// Precip is the monthly total in mm, Temp the monthly mean in degC. Each is Missing when the
	// share of valid days is below the coverage threshold.
	Precip, Temp float64
	// PrecipDays/TempDays: number of valid daily values found; Days: days in the month.
	PrecipDays, TempDays, Days int
}

// PrecipCoverage is PrecipDays/Days.
func (m Monthly) PrecipCoverage() float64 { return float64(m.PrecipDays) / float64(m.Days) }

// TempCoverage is TempDays/Days.
func (m Monthly) TempCoverage() float64 { return float64(m.TempDays) / float64(m.Days) }

// Valid is true if both precipitation and temperature are available.
func (m Monthly) Valid() bool { return !IsMissing(m.Precip) && !IsMissing(m.Temp) }

// MonthlyAggregate aggregates daily records into months from..to (inclusive); every month in the
// range appears in the result (in order), also when no day exists for it.
//
// Rules (documented in SIMULATION.md):
//   - a month's precipitation is valid when at least minDayCoverage of its days have a value;
//   - the monthly total of a valid-but-incomplete month is the sum of the valid days scaled by
//     daysInMonth/validDays (so a month missing 2 of 30 days is not biased low);
//   - the monthly temperature is the mean of the valid days (same coverage rule, no scaling);
//   - days outside from..to are ignored; if a date occurs more than once the last record wins.
func MonthlyAggregate(days []DailyRecord, from, to YM, minDayCoverage float64) []Monthly {
	n := MonthsInclusive(from, to)
	out := make([]Monthly, n)
	type acc struct {
		psum, tsum float64
		pn, tn     int
	}
	accs := make([]acc, n)
	seen := make(map[int]DailyRecord, len(days))
	keys := make([]int, 0, len(days))
	for _, d := range days {
		y, m, dd := d.Date.UTC().Date()
		k := y*10000 + int(m)*100 + dd
		if _, ok := seen[k]; !ok {
			keys = append(keys, k)
		}
		seen[k] = d
	}
	sort.Ints(keys) // deterministic summation order
	for _, k := range keys {
		d := seen[k]
		y, m, _ := d.Date.UTC().Date()
		i := YM{y, int(m)}.Index() - from.Index()
		if i < 0 || i >= n {
			continue
		}
		if !IsMissing(d.Precip) {
			accs[i].psum += d.Precip
			accs[i].pn++
		}
		if !IsMissing(d.Temp) {
			accs[i].tsum += d.Temp
			accs[i].tn++
		}
	}
	for i := 0; i < n; i++ {
		ym := from.Add(i)
		dim := ym.DaysIn()
		m := Monthly{YM: ym, Precip: Missing, Temp: Missing, PrecipDays: accs[i].pn, TempDays: accs[i].tn, Days: dim}
		if accs[i].pn > 0 && float64(accs[i].pn)/float64(dim) >= minDayCoverage {
			m.Precip = accs[i].psum * float64(dim) / float64(accs[i].pn)
		}
		if accs[i].tn > 0 && float64(accs[i].tn)/float64(dim) >= minDayCoverage {
			m.Temp = accs[i].tsum / float64(accs[i].tn)
		}
		out[i] = m
	}
	return out
}

// MonthlyCoverage returns the fraction of months with valid precipitation, valid temperature and
// with both (all in 0..1; 0 for an empty slice).
func MonthlyCoverage(ms []Monthly) (precip, temp, both float64) {
	if len(ms) == 0 {
		return 0, 0, 0
	}
	var p, t, b int
	for _, m := range ms {
		if !IsMissing(m.Precip) {
			p++
		}
		if !IsMissing(m.Temp) {
			t++
		}
		if m.Valid() {
			b++
		}
	}
	n := float64(len(ms))
	return float64(p) / n, float64(t) / n, float64(b) / n
}

// PrecipSeries / TempSeries extract the monthly value vectors (Missing preserved).
func PrecipSeries(ms []Monthly) []float64 {
	out := make([]float64, len(ms))
	for i, m := range ms {
		out[i] = m.Precip
	}
	return out
}

func TempSeries(ms []Monthly) []float64 {
	out := make([]float64, len(ms))
	for i, m := range ms {
		out[i] = m.Temp
	}
	return out
}
