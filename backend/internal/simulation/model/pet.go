package model

import (
	"math"
	"time"
)

// DaylightHours returns the astronomical day length N (hours) for latitude latDeg (degrees, north
// positive) and day of year doy (1..366), using the FAO-56 relations (Allen et al. 1998, eq. 24-25, 34):
//
//	delta = 0.409 sin(2 pi J / 365 - 1.39)          solar declination (rad)
//	ws    = arccos(-tan(phi) tan(delta))            sunset hour angle (rad)
//	N     = 24 ws / pi
//
// Polar day / polar night are handled by clamping the arccos argument (N = 24 or 0).
func DaylightHours(latDeg float64, doy int) float64 {
	phi := latDeg * math.Pi / 180
	delta := 0.409 * math.Sin(2*math.Pi*float64(doy)/365-1.39)
	x := -math.Tan(phi) * math.Tan(delta)
	if x > 1 {
		x = 1
	} else if x < -1 {
		x = -1
	}
	return 24 * math.Acos(x) / math.Pi
}

// MeanDaylightHours is the mean of DaylightHours over all days of the month.
func MeanDaylightHours(latDeg float64, ym YM) float64 {
	n := ym.DaysIn()
	var s float64
	for d := 1; d <= n; d++ {
		doy := time.Date(ym.Year, time.Month(ym.Month), d, 0, 0, 0, 0, time.UTC).YearDay()
		s += DaylightHours(latDeg, doy)
	}
	return s / float64(n)
}

// HeatIndex is Thornthwaite's annual heat index I = sum over the 12 calendar months of (T/5)^1.514,
// where months with T <= 0 degC contribute 0. monthlyMeanT are the (long-term) mean temperatures of
// January..December in degC; a Missing month contributes 0.
func HeatIndex(monthlyMeanT [12]float64) float64 {
	var I float64
	for _, t := range monthlyMeanT {
		if IsMissing(t) || t <= 0 {
			continue
		}
		I += math.Pow(t/5, 1.514)
	}
	return I
}

// ThornthwaiteExponent is the cubic a(I) = 6.75e-7 I^3 - 7.71e-5 I^2 + 1.792e-2 I + 0.49239.
func ThornthwaiteExponent(I float64) float64 {
	return 6.75e-7*I*I*I - 7.71e-5*I*I + 1.792e-2*I + 0.49239
}

// Thornthwaite computes monthly potential evapotranspiration with the Thornthwaite (1948) method
// as given in Thornthwaite & Mather (1957). It is a temperature-only index; see SIMULATION.md for
// its known limitations compared with Penman-Monteith.
type Thornthwaite struct {
	LatDeg float64
	I      float64 // annual heat index of the climate the instance represents
	A      float64 // exponent a(I)
}

// NewThornthwaite builds the PET function for a climate whose long-term mean monthly temperatures
// (Jan..Dec, degC) are given. The heat index I is taken from this climatology, not from a single
// year, as in Thornthwaite's original method.
func NewThornthwaite(monthlyMeanT [12]float64, latDeg float64) Thornthwaite {
	I := HeatIndex(monthlyMeanT)
	return Thornthwaite{LatDeg: latDeg, I: I, A: ThornthwaiteExponent(I)}
}

// Unadjusted is the PET (mm/month) of a standard month of 30 days with 12 h days:
//
//	T <= 0         : 0
//	0 < T < 26.5   : 16 (10 T / I)^a
//	T >= 26.5      : -415.85 + 32.24 T - 0.43 T^2
func (t Thornthwaite) Unadjusted(T float64) float64 {
	switch {
	case IsMissing(T):
		return Missing
	case T <= 0:
		return 0
	case T >= 26.5:
		return -415.85 + 32.24*T - 0.43*T*T
	}
	if t.I <= 0 {
		return 0
	}
	return 16 * math.Pow(10*T/t.I, t.A)
}

// PET is the adjusted monthly PET (mm) for mean monthly temperature T in the given month:
// Unadjusted(T) * (N/12) * (d/30) with N = mean daylight hours and d = days in the month.
func (t Thornthwaite) PET(T float64, ym YM) float64 {
	u := t.Unadjusted(T)
	if IsMissing(u) {
		return Missing
	}
	return u * (MeanDaylightHours(t.LatDeg, ym) / 12) * (float64(ym.DaysIn()) / 30)
}

// PETSeries returns PET for a monthly temperature series starting at start (Missing preserved).
func (t Thornthwaite) PETSeries(temps []float64, start YM) []float64 {
	out := make([]float64, len(temps))
	for i, T := range temps {
		out[i] = t.PET(T, start.Add(i))
	}
	return out
}
