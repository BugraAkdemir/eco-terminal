package model

import "fmt"

// Delta is a simple "delta-change" perturbation of an observed reference period. It is a
// what-if experiment on the past, NOT a climate projection or a forecast.
type Delta struct {
	PrecipChangePct float64 // multiplies precipitation by (1 + PrecipChangePct/100); must be >= -100
	TempChangeC     float64 // added to temperature (degC)
}

// Validate checks that the perturbation is physically meaningful (precipitation cannot go negative).
func (d Delta) Validate() error {
	if IsMissing(d.PrecipChangePct) || IsMissing(d.TempChangeC) {
		return fmt.Errorf("delta contains NaN/Inf")
	}
	if d.PrecipChangePct < -100 {
		return fmt.Errorf("precip_change_pct %.2f < -100 would make precipitation negative", d.PrecipChangePct)
	}
	return nil
}

// Identity reports whether the delta changes nothing.
func (d Delta) Identity() bool { return d.PrecipChangePct == 0 && d.TempChangeC == 0 }

// PrecipFactor is 1 + PrecipChangePct/100.
func (d Delta) PrecipFactor() float64 { return 1 + d.PrecipChangePct/100 }

// ApplyDaily returns a copy of days with precipitation scaled and temperature shifted (Missing
// values stay Missing). With the identity delta the output is bit-for-bit equal to the input.
func (d Delta) ApplyDaily(days []DailyRecord) []DailyRecord {
	out := make([]DailyRecord, len(days))
	f := d.PrecipFactor()
	for i, r := range days {
		out[i] = DailyRecord{Date: r.Date, Precip: scale(r.Precip, f), Temp: shift(r.Temp, d.TempChangeC)}
	}
	return out
}

// ApplyMonthly returns scaled precipitation and shifted temperature monthly series. Because the
// perturbation is linear, ApplyMonthly(MonthlyAggregate(x)) equals MonthlyAggregate(ApplyDaily(x))
// (up to floating-point rounding).
func (d Delta) ApplyMonthly(precip, temp []float64) (p, t []float64) {
	f := d.PrecipFactor()
	p = make([]float64, len(precip))
	t = make([]float64, len(temp))
	for i, v := range precip {
		p[i] = scale(v, f)
	}
	for i, v := range temp {
		t[i] = shift(v, d.TempChangeC)
	}
	return
}

func scale(v, f float64) float64 {
	if IsMissing(v) {
		return v
	}
	return v * f
}

func shift(v, dt float64) float64 {
	if IsMissing(v) {
		return v
	}
	return v + dt
}
