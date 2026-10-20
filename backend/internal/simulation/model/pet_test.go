package model

import (
	"math"
	"testing"
)

func near(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if math.IsNaN(got) || math.Abs(got-want) > tol {
		t.Fatalf("%s: got %.6f want %.6f (tol %g)", name, got, want, tol)
	}
}

// Published worked example: Landtek Limited, Project 21260, "Appendix H: Thornthwaite Method For
// Calculating Evapotranspiration" (Hamilton RBG, Ontario climate data; spreadsheet PDF dated
// 2024-09-06, https://www.oakville.ca/getmedia/dc0b12c5-2ebb-43e2-b8c3-4f8dcf07adb7/da-131212-WaterBalance2-S1.pdf).
// Monthly mean temperatures (degC) Jan..Dec; the document reports
//
//	heat index I = 41.3085 ; exponent a = 1.148655
//	unadjusted PET (cm): Mar 0.1415, Apr 2.9806, May 6.1294, Jun 9.1774, Jul 10.9266,
//	                     Aug 10.3014, Sep 7.7426, Oct 4.4173, Nov 1.5863 (T<=0 months = 0)
//	adjusted PET  (cm): Mar 0.1585, Apr 3.3383, May 7.7231, Jun 11.7470, Jul 14.0953,
//	                     Aug 12.3617, Sep 8.0523, Oct 4.1965, Nov 1.2849 ; total 629.58 mm/yr
//
// (Hamilton is at about 43.3 N.) Our day length uses FAO-56 equations while the spreadsheet uses
// its own day-length correction, so the adjusted values are compared with a 4 % tolerance (largest observed deviation 2.8 %, October); the
// unadjusted values and I, a must match to 1e-6.
var hamiltonT = [12]float64{-4.7, -3.9, 0.5, 7.1, 13.3, 18.9, 22, 20.9, 16.3, 10, 4.1, -1.4}

func TestThornthwaitePublishedExample(t *testing.T) {
	// The spreadsheet prints I = 41.3085 although its own nine monthly indices sum to 41.3393
	// (0.0306+1.7005+4.3982+7.4873+9.4230+8.7189+5.9843+2.8560+0.7405); the printed PET values
	// were computed with 41.3085. We therefore (a) check HeatIndex against the sum of the printed
	// monthly indices and (b) reproduce the printed PET values with the printed I and a.
	near(t, "I = sum of published monthly indices", HeatIndex(hamiltonT), 41.3393, 2e-3)
	near(t, "a(published I)", ThornthwaiteExponent(41.3084717), 1.148654797, 1e-6)

	th := NewThornthwaite(hamiltonT, 43.3)
	th.I, th.A = 41.3084717, ThornthwaiteExponent(41.3084717)
	unadjCm := []float64{0, 0, 0.141489475, 2.980613536, 6.129446549, 9.177357679, 10.92657209, 10.30139518, 7.74263507, 4.417316126, 1.586283476, 0}
	adjCm := []float64{0, 0, 0.158468212, 3.33828716, 7.723102652, 11.74701783, 14.095278, 12.36167422, 8.052340473, 4.196450319, 1.284889615, 0}
	var total float64
	for m := 0; m < 12; m++ {
		near(t, "unadjusted month", th.Unadjusted(hamiltonT[m])/10, unadjCm[m], 1e-6)
		got := th.PET(hamiltonT[m], YM{2001, m + 1}) / 10
		total += got
		if adjCm[m] == 0 {
			if got != 0 {
				t.Fatalf("month %d should be 0, got %v", m+1, got)
			}
			continue
		}
		if m == 2 {
			// The published March correction factor (1.12) equals its April factor and is not
			// consistent with ~11.9 h March daylight at 43 N (expected ~1.02); we do not
			// reproduce this apparent spreadsheet error. Check the physical factor instead.
			near(t, "March factor", got/(th.Unadjusted(hamiltonT[m])/10), 1.02, 0.02)
			continue
		}
		if math.Abs(got-adjCm[m])/adjCm[m] > 0.04 {
			t.Fatalf("adjusted month %d: got %.4f cm want %.4f cm (>4%%)", m+1, got, adjCm[m])
		}
	}
	// total differs from the published 62.958 cm only through the March entry (+0.016 cm)
	if math.Abs(total-(62.958-0.158468212+0.1414895*1.02)) > 0.02*62.958 {
		t.Fatalf("annual adjusted PET %.3f cm, published 62.958 cm", total)
	}
}

// Hand calculation, all 12 months at 10 degC:
//
//	i = (10/5)^1.514 = 2.85601 ; I = 12 i = 34.2721
//	a = 6.75e-7*I^3 - 7.71e-5*I^2 + 1.792e-2*I + 0.49239 = 1.04316
//	PET_unadj = 16*(100/34.2721)^1.04316 = 16*(2.91785)^1.04316 = 48.91 mm
func TestThornthwaiteHandCalculation(t *testing.T) {
	var T [12]float64
	for i := range T {
		T[i] = 10
	}
	near(t, "I", HeatIndex(T), 12*math.Pow(2, 1.514), 1e-9)
	near(t, "I numeric", HeatIndex(T), 34.2721, 1e-3)
	near(t, "a", ThornthwaiteExponent(HeatIndex(T)), 1.04316, 1e-4)
	th := NewThornthwaite(T, 45)
	near(t, "unadj", th.Unadjusted(10), 48.91, 0.05)
	ym := YM{2021, 6}
	near(t, "adjusted", th.PET(10, ym), th.Unadjusted(10)*MeanDaylightHours(45, ym)/12, 1e-9) // June has 30 days: d/30 = 1
}

func TestThornthwaiteBranches(t *testing.T) {
	var T [12]float64
	for i := range T {
		T[i] = 15
	}
	th := NewThornthwaite(T, 40)
	if th.Unadjusted(0) != 0 || th.Unadjusted(-5) != 0 {
		t.Fatal("T<=0 must give 0")
	}
	// T >= 26.5 uses the quadratic: -415.85 + 32.24*30 - 0.43*900 = 164.35
	near(t, "hot branch", th.Unadjusted(30), 164.35, 1e-9)
	if !IsMissing(th.Unadjusted(Missing)) {
		t.Fatal("missing must propagate")
	}
	// all-frozen climate: I=0 -> PET 0, no NaN
	var cold [12]float64
	for i := range cold {
		cold[i] = -3
	}
	if v := NewThornthwaite(cold, 60).Unadjusted(5); v != 0 {
		t.Fatalf("I=0 should give 0, got %v", v)
	}
}

func TestDaylightHours(t *testing.T) {
	near(t, "equator", DaylightHours(0, 100), 12, 1e-9)
	// FAO-56 Example 8: latitude 20 S, 3 September (J=246): N = 11.7 h (Allen et al. 1998).
	near(t, "FAO-56 ex.8", DaylightHours(-20, 246), 11.7, 0.1)
	near(t, "41N summer", DaylightHours(41, 172), 14.95, 0.1)
	near(t, "41N winter", DaylightHours(41, 355), 9.05, 0.1)
	near(t, "polar day", DaylightHours(80, 172), 24, 1e-9)
	near(t, "polar night", DaylightHours(80, 355), 0, 1e-9)
}
