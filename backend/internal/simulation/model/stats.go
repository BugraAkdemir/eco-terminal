package model

import "math"

// MeanValid returns the mean of the non-missing values and how many there were (NaN, 0 if none).
func MeanValid(xs []float64) (mean float64, n int) {
	var s float64
	for _, x := range xs {
		if IsMissing(x) {
			continue
		}
		s += x
		n++
	}
	if n == 0 {
		return Missing, 0
	}
	return s / float64(n), n
}

// StdValid returns the SAMPLE standard deviation (n-1 denominator) of the non-missing values;
// NaN if fewer than 2 values.
func StdValid(xs []float64) float64 {
	m, n := MeanValid(xs)
	if n < 2 {
		return Missing
	}
	var ss float64
	for _, x := range xs {
		if IsMissing(x) {
			continue
		}
		d := x - m
		ss += d * d
	}
	return math.Sqrt(ss / float64(n-1))
}

// Climatology holds the month-of-year (index 0 = January) mean, sample standard deviation and
// sample size of a monthly series over a reference period.
type Climatology struct {
	Mean [12]float64
	Std  [12]float64
	N    [12]int
}

// MonthlyClimatology computes the per-calendar-month statistics of vals, where vals[0] belongs to
// month start and every following element to the next month. Missing values are skipped.
// A calendar month with no data gets NaN mean/std and N=0.
func MonthlyClimatology(vals []float64, start YM) Climatology {
	var groups [12][]float64
	for i, v := range vals {
		m := start.Add(i).Month - 1
		groups[m] = append(groups[m], v)
	}
	var c Climatology
	for m := 0; m < 12; m++ {
		c.Mean[m], c.N[m] = MeanValid(groups[m])
		c.Std[m] = StdValid(groups[m])
	}
	return c
}

// Complete reports whether every calendar month has at least minN values.
func (c Climatology) Complete(minN int) bool {
	for m := 0; m < 12; m++ {
		if c.N[m] < minN {
			return false
		}
	}
	return true
}

// PercentOfNormal returns 100*x/normal (NaN if normal <= 0 or either is missing).
func PercentOfNormal(x, normal float64) float64 {
	if IsMissing(x) || IsMissing(normal) || normal <= 0 {
		return Missing
	}
	return 100 * x / normal
}

// ZScore returns (x-mean)/std (NaN if std <= 0 or any input is missing).
func ZScore(x, mean, std float64) float64 {
	if IsMissing(x) || IsMissing(mean) || IsMissing(std) || std <= 0 {
		return Missing
	}
	return (x - mean) / std
}

// PrecipAnomalyPercent returns, per month, 100*P/normal_of_that_calendar_month.
func PrecipAnomalyPercent(vals []float64, start YM, clim Climatology) []float64 {
	out := make([]float64, len(vals))
	for i, v := range vals {
		out[i] = PercentOfNormal(v, clim.Mean[start.Add(i).Month-1])
	}
	return out
}

// PrecipAnomalyZ returns, per month, the z-score of v against the climatology (the supplied
// climatology — not the series' own — so a perturbed series is compared with the reference normal).
func PrecipAnomalyZ(vals []float64, start YM, clim Climatology) []float64 {
	out := make([]float64, len(vals))
	for i, v := range vals {
		m := start.Add(i).Month - 1
		out[i] = ZScore(v, clim.Mean[m], clim.Std[m])
	}
	return out
}
