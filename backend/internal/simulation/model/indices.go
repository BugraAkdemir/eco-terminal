package model

import (
	"errors"
	"fmt"
	"math"
)

// IndexCDFLimit caps the cumulative probability away from 0 and 1 before the normal quantile is
// taken, so standardized values are limited to about +/-4.75.
const IndexCDFLimit = 1e-6

// DryThreshold: a month is "dry" (drought month) when its standardized index is <= -1.
const DryThreshold = -1.0

// Accumulate returns the k-month rolling sums of vals: out[i] = vals[i-k+1] + ... + vals[i]. The
// first k-1 entries and every window that contains a Missing value are Missing.
func Accumulate(vals []float64, k int) []float64 {
	out := make([]float64, len(vals))
	for i := range vals {
		out[i] = Missing
		if i < k-1 {
			continue
		}
		var s float64
		ok := true
		for j := i - k + 1; j <= i; j++ {
			if IsMissing(vals[j]) {
				ok = false
				break
			}
			s += vals[j]
		}
		if ok {
			out[i] = s
		}
	}
	return out
}

// cdfer is a fitted distribution.
type cdfer interface{ CDF(x float64) float64 }

// MonthlyStandardizer holds one fitted distribution per calendar month (index 0 = January) and maps
// accumulated values to standard-normal scores: z = Phi^-1(F_month(x)).
type MonthlyStandardizer struct {
	dists [12]cdfer
	// Fallbacks counts the calendar months for which the preferred distribution could not be
	// fitted and a normal distribution was used instead (only FitWBSI; see FitWBSI).
	Fallbacks int
}

// Standardize maps accumulated values (acc[0] belongs to month start) to z-scores using the FITTED
// (reference) distributions — never refitted — so a perturbed series is judged against the
// reference climate. Missing values and months without a fit give Missing.
func (s MonthlyStandardizer) Standardize(acc []float64, start YM) []float64 {
	out := make([]float64, len(acc))
	for i, x := range acc {
		out[i] = Missing
		d := s.dists[start.Add(i).Month-1]
		if d == nil || IsMissing(x) {
			continue
		}
		p := d.CDF(x)
		if p < IndexCDFLimit {
			p = IndexCDFLimit
		} else if p > 1-IndexCDFLimit {
			p = 1 - IndexCDFLimit
		}
		out[i] = NormalQuantile(p)
	}
	return out
}

// ErrFit is returned when a monthly standardizer cannot be fitted.
var ErrFit = errors.New("model: cannot fit standardization distribution")

func fitMonthly(acc []float64, start YM, minSamples int, fit func([]float64) (cdfer, bool), fallback func([]float64) (cdfer, bool)) (MonthlyStandardizer, error) {
	var groups [12][]float64
	for i, v := range acc {
		if IsMissing(v) {
			continue
		}
		m := start.Add(i).Month - 1
		groups[m] = append(groups[m], v)
	}
	var s MonthlyStandardizer
	for m := 0; m < 12; m++ {
		if len(groups[m]) < minSamples {
			return s, fmt.Errorf("%w: calendar month %d has %d usable values (< %d)", ErrFit, m+1, len(groups[m]), minSamples)
		}
		d, ok := fit(groups[m])
		if !ok && fallback != nil {
			d, ok = fallback(groups[m])
			if ok {
				s.Fallbacks++
			}
		}
		if !ok {
			return s, fmt.Errorf("%w: distribution fit failed for calendar month %d", ErrFit, m+1)
		}
		s.dists[m] = d
	}
	return s, nil
}

// FitSPI fits the SPI standardizer: one mixed (zero-mass + gamma, Thom estimator) distribution per
// calendar month to the accumulated precipitation `acc` of the reference period.
func FitSPI(acc []float64, start YM, minSamples int) (MonthlyStandardizer, error) {
	return fitMonthly(acc, start, minSamples, func(x []float64) (cdfer, bool) {
		g, ok := FitGammaThom(x)
		return g, ok
	}, nil)
}

// FitWBSI fits the water-balance standardizer: one 3-parameter log-logistic distribution (PWM)
// per calendar month to the accumulated climatic water balance D = P - PET of the reference period.
// The log-logistic needs a positively skewed sample (L-skewness in (0,1)). In calendar months where
// the sample is symmetric or negatively skewed (typical for summer months when PET dominates) the
// fit is impossible; for those months a normal distribution (sample mean and standard deviation)
// is used instead and counted in Fallbacks. This is a documented deviation from the SPEI procedure.
func FitWBSI(acc []float64, start YM, minSamples int) (MonthlyStandardizer, error) {
	return fitMonthly(acc, start, minSamples, func(x []float64) (cdfer, bool) {
		l, ok := FitLogLogisticPWM(x)
		return l, ok
	}, func(x []float64) (cdfer, bool) {
		m, n := MeanValid(x)
		sd := StdValid(x)
		if n < 2 || !(sd > 0) {
			return nil, false
		}
		return normalDist{m, sd}, true
	})
}

type normalDist struct{ mu, sigma float64 }

func (n normalDist) CDF(x float64) float64 { return NormalCDF((x - n.mu) / n.sigma) }

// WaterBalance returns D = P - PET per month (Missing if either is Missing).
func WaterBalance(p, pet []float64) []float64 {
	out := make([]float64, len(p))
	for i := range p {
		if IsMissing(p[i]) || IsMissing(pet[i]) {
			out[i] = Missing
		} else {
			out[i] = p[i] - pet[i]
		}
	}
	return out
}

// DryMonths counts the months with index <= DryThreshold among the months where idx is defined,
// and returns the number of defined months. The fraction is dry/defined (0 when none defined).
func DryMonths(idx []float64) (dry, defined int, fraction float64) {
	for _, v := range idx {
		if IsMissing(v) {
			continue
		}
		defined++
		if v <= DryThreshold {
			dry++
		}
	}
	if defined > 0 {
		fraction = float64(dry) / float64(defined)
	}
	return
}

// DryMonthsPaired counts dry months of a and b over the months where BOTH are defined, so that a
// reference and a scenario are compared over exactly the same months.
func DryMonthsPaired(a, b []float64) (dryA, dryB, defined int) {
	for i := range a {
		if IsMissing(a[i]) || IsMissing(b[i]) {
			continue
		}
		defined++
		if a[i] <= DryThreshold {
			dryA++
		}
		if b[i] <= DryThreshold {
			dryB++
		}
	}
	return
}

// MeanPaired returns the means of a and b over the months where both are defined.
func MeanPaired(a, b []float64) (ma, mb float64, n int) {
	var sa, sb float64
	for i := range a {
		if IsMissing(a[i]) || IsMissing(b[i]) {
			continue
		}
		sa += a[i]
		sb += b[i]
		n++
	}
	if n == 0 {
		return Missing, Missing, 0
	}
	return sa / float64(n), sb / float64(n), n
}

func round(v float64, places int) float64 {
	if IsMissing(v) {
		return v
	}
	p := math.Pow(10, float64(places))
	r := math.Round(v*p) / p
	if r == 0 { // avoid -0
		return 0
	}
	return r
}
