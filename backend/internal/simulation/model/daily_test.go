package model

import (
	"math"
	"testing"
)

func TestYM(t *testing.T) {
	y, err := ParseYM("1991-01")
	if err != nil || y != (YM{1991, 1}) {
		t.Fatal(y, err)
	}
	if _, err := ParseYM("1991-13"); err == nil {
		t.Fatal("month 13 must fail")
	}
	if _, err := ParseYM("1991"); err == nil {
		t.Fatal("year only must fail")
	}
	if got := (YM{1991, 11}).Add(3); got != (YM{1992, 2}) {
		t.Fatal(got)
	}
	if got := (YM{1991, 2}).Add(-3); got != (YM{1990, 11}) {
		t.Fatal(got)
	}
	if (YM{2020, 2}).DaysIn() != 29 || (YM{2021, 2}).DaysIn() != 28 || (YM{2021, 12}).DaysIn() != 31 {
		t.Fatal("days in month")
	}
	if MonthsInclusive(YM{1991, 1}, YM{2020, 12}) != 360 || MonthsInclusive(YM{2020, 1}, YM{2019, 1}) != 0 {
		t.Fatal("months inclusive")
	}
	if (YM{2020, 3}).String() != "2020-03" {
		t.Fatal("string")
	}
}

func fullMonth(y, m int, p, tmp float64) []DailyRecord {
	var out []DailyRecord
	for d := 1; d <= (YM{y, m}).DaysIn(); d++ {
		out = append(out, Day(y, m, d, p, tmp))
	}
	return out
}

func TestMonthlyAggregate(t *testing.T) {
	// January: 31 days x 2 mm and 5 degC -> 62 mm, 5 degC. February 2020 (leap): 29 days x 1 mm.
	days := append(fullMonth(2020, 1, 2, 5), fullMonth(2020, 2, 1, 7)...)
	ms := MonthlyAggregate(days, YM{2020, 1}, YM{2020, 3}, 0.9)
	if len(ms) != 3 {
		t.Fatalf("want 3 months incl. empty March, got %d", len(ms))
	}
	near(t, "jan precip", ms[0].Precip, 62, 1e-12)
	near(t, "jan temp", ms[0].Temp, 5, 1e-12)
	near(t, "feb precip (29 days)", ms[1].Precip, 29, 1e-12)
	if ms[0].Days != 31 || ms[1].Days != 29 || ms[0].PrecipCoverage() != 1 {
		t.Fatalf("%+v", ms[:2])
	}
	if !IsMissing(ms[2].Precip) || !IsMissing(ms[2].Temp) || ms[2].Valid() {
		t.Fatal("March has no data: must be missing")
	}
	p, tc, both := MonthlyCoverage(ms)
	near(t, "coverage precip", p, 2.0/3, 1e-12)
	near(t, "coverage temp", tc, 2.0/3, 1e-12)
	near(t, "coverage both", both, 2.0/3, 1e-12)
}

func TestMonthlyAggregateMissingDays(t *testing.T) {
	// 2 of 31 days missing (coverage 29/31 = 0.935 >= 0.9): sum scaled by 31/29, so a constant
	// 1 mm/day still gives 31 mm. 4 missing (27/31 = 0.871 < 0.9): month is missing.
	mk := func(missing int) []DailyRecord {
		d := fullMonth(2021, 1, 1, 3)
		for i := 0; i < missing; i++ {
			d[i].Precip = Missing
		}
		return d
	}
	ms := MonthlyAggregate(mk(2), YM{2021, 1}, YM{2021, 1}, 0.9)
	near(t, "scaled", ms[0].Precip, 31, 1e-12)
	if ms[0].PrecipDays != 29 {
		t.Fatal(ms[0].PrecipDays)
	}
	near(t, "temp complete", ms[0].Temp, 3, 1e-12)
	ms = MonthlyAggregate(mk(4), YM{2021, 1}, YM{2021, 1}, 0.9)
	if !IsMissing(ms[0].Precip) {
		t.Fatal("27/31 < 0.9 must be missing")
	}
	if IsMissing(ms[0].Temp) {
		t.Fatal("temperature is independent of precipitation gaps")
	}
	// duplicates: last record wins; out-of-range days are ignored
	days := fullMonth(2021, 1, 1, 3)
	days = append(days, Day(2021, 1, 1, 10, 3), Day(2019, 5, 5, 99, 99))
	ms = MonthlyAggregate(days, YM{2021, 1}, YM{2021, 1}, 0.9)
	near(t, "dup last wins", ms[0].Precip, 30+10, 1e-12)
}

func TestClimatologyAndAnomaly(t *testing.T) {
	// three Januaries 10, 20, 30 -> mean 20, SAMPLE std 10 ; Februaries 5, 5, Missing
	start := YM{2000, 1}
	vals := make([]float64, 36)
	for i := range vals {
		vals[i] = Missing
	}
	vals[0], vals[12], vals[24] = 10, 20, 30
	vals[1], vals[13] = 5, 5
	c := MonthlyClimatology(vals, start)
	near(t, "jan mean", c.Mean[0], 20, 1e-12)
	near(t, "jan std", c.Std[0], 10, 1e-12)
	near(t, "feb mean", c.Mean[1], 5, 1e-12)
	if c.N[0] != 3 || c.N[1] != 2 || c.N[2] != 0 || !IsMissing(c.Mean[2]) {
		t.Fatalf("%+v", c)
	}
	if c.Complete(1) {
		t.Fatal("not complete: March has no data")
	}
	near(t, "percent of normal", PercentOfNormal(30, 20), 150, 1e-12)
	near(t, "z", ZScore(30, 20, 10), 1, 1e-12)
	if !IsMissing(PercentOfNormal(1, 0)) || !IsMissing(ZScore(1, 1, 0)) {
		t.Fatal("zero normal / std must give missing")
	}
	pa := PrecipAnomalyPercent(vals[:3], start, c)
	near(t, "series pct", pa[0], 50, 1e-12)
	za := PrecipAnomalyZ(vals[24:25], YM{2002, 1}, c)
	near(t, "series z", za[0], 1, 1e-12)
	m, n := MeanValid([]float64{1, Missing, 3})
	if m != 2 || n != 2 {
		t.Fatal(m, n)
	}
	if !math.IsNaN(StdValid([]float64{4})) {
		t.Fatal("std of one value")
	}
}

func TestDelta(t *testing.T) {
	d := Delta{PrecipChangePct: -20, TempChangeC: 2}
	days := []DailyRecord{Day(2020, 1, 1, 10, 5), Day(2020, 1, 2, Missing, Missing), Day(2020, 1, 3, 0, -3)}
	out := d.ApplyDaily(days)
	near(t, "precip", out[0].Precip, 8, 1e-12)
	near(t, "temp", out[0].Temp, 7, 1e-12)
	if !IsMissing(out[1].Precip) || !IsMissing(out[1].Temp) {
		t.Fatal("missing must stay missing")
	}
	if out[2].Precip != 0 || out[2].Temp != -1 {
		t.Fatal(out[2])
	}
	if days[0].Precip != 10 {
		t.Fatal("input must not be mutated")
	}
	// identity is bit-exact
	id := Delta{}
	if !id.Identity() {
		t.Fatal("identity")
	}
	for i, r := range id.ApplyDaily(days) {
		if r.Date != days[i].Date || (r.Precip != days[i].Precip && !IsMissing(r.Precip)) || (r.Temp != days[i].Temp && !IsMissing(r.Temp)) {
			t.Fatalf("identity changed day %d", i)
		}
	}
	// linearity: aggregating after the delta equals applying it to the aggregate
	all := append(fullMonth(2020, 1, 2, 5), fullMonth(2020, 2, 1, 7)...)
	a := MonthlyAggregate(d.ApplyDaily(all), YM{2020, 1}, YM{2020, 2}, 0.9)
	b := MonthlyAggregate(all, YM{2020, 1}, YM{2020, 2}, 0.9)
	pm, tm := d.ApplyMonthly(PrecipSeries(b), TempSeries(b))
	for i := range a {
		near(t, "linear precip", a[i].Precip, pm[i], 1e-9)
		near(t, "linear temp", a[i].Temp, tm[i], 1e-9)
	}
	if err := (Delta{PrecipChangePct: -100.1}).Validate(); err == nil {
		t.Fatal("-100.1% must be rejected")
	}
	if err := (Delta{PrecipChangePct: -100}).Validate(); err != nil {
		t.Fatal("-100% is allowed (no precipitation)")
	}
	if err := (Delta{PrecipChangePct: math.NaN()}).Validate(); err == nil {
		t.Fatal("NaN must be rejected")
	}
}
