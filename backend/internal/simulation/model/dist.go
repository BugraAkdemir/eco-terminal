package model

import (
	"math"
	"sort"
)

// ───────────── standard normal ─────────────

// NormalCDF is the standard normal cumulative distribution function.
func NormalCDF(z float64) float64 { return 0.5 * math.Erfc(-z/math.Sqrt2) }

// NormalQuantile is the inverse of NormalCDF (Acklam's rational approximation followed by one
// Halley refinement step; absolute error < 1e-12 for 1e-10 < p < 1-1e-10). p must be in (0,1);
// 0 and 1 map to -Inf/+Inf.
func NormalQuantile(p float64) float64 {
	if math.IsNaN(p) || p < 0 || p > 1 {
		return math.NaN()
	}
	if p == 0 {
		return math.Inf(-1)
	}
	if p == 1 {
		return math.Inf(1)
	}
	a := [...]float64{-3.969683028665376e+01, 2.209460984245205e+02, -2.759285104469687e+02, 1.383577518672690e+02, -3.066479806614716e+01, 2.506628277459239e+00}
	b := [...]float64{-5.447609879822406e+01, 1.615858368580409e+02, -1.556989798598866e+02, 6.680131188771972e+01, -1.328068155288572e+01}
	c := [...]float64{-7.784894002430293e-03, -3.223964580411365e-01, -2.400758277161838e+00, -2.549732539343734e+00, 4.374664141464968e+00, 2.938163982698783e+00}
	d := [...]float64{7.784695709041462e-03, 3.224671290700398e-01, 2.445134137142996e+00, 3.754408661907416e+00}
	const plow = 0.02425
	var x float64
	switch {
	case p < plow:
		q := math.Sqrt(-2 * math.Log(p))
		x = (((((c[0]*q+c[1])*q+c[2])*q+c[3])*q+c[4])*q + c[5]) / ((((d[0]*q+d[1])*q+d[2])*q+d[3])*q + 1)
	case p <= 1-plow:
		q := p - 0.5
		r := q * q
		x = (((((a[0]*r+a[1])*r+a[2])*r+a[3])*r+a[4])*r + a[5]) * q / (((((b[0]*r+b[1])*r+b[2])*r+b[3])*r+b[4])*r + 1)
	default:
		q := math.Sqrt(-2 * math.Log(1-p))
		x = -(((((c[0]*q+c[1])*q+c[2])*q+c[3])*q+c[4])*q + c[5]) / ((((d[0]*q+d[1])*q+d[2])*q+d[3])*q + 1)
	}
	// one step of Halley's method
	e := NormalCDF(x) - p
	u := e * math.Sqrt(2*math.Pi) * math.Exp(x*x/2)
	return x - u/(1+x*u/2)
}

// ───────────── gamma distribution (SPI) ─────────────

// RegularizedGammaP is the regularized lower incomplete gamma function P(a, x).
func RegularizedGammaP(a, x float64) float64 {
	if a <= 0 || x < 0 || math.IsNaN(a) || math.IsNaN(x) {
		return math.NaN()
	}
	if x == 0 {
		return 0
	}
	lg, _ := math.Lgamma(a)
	if x < a+1 { // series
		ap, sum, del := a, 1/a, 1/a
		for n := 0; n < 1000; n++ {
			ap++
			del *= x / ap
			sum += del
			if math.Abs(del) < math.Abs(sum)*1e-16 {
				break
			}
		}
		return sum * math.Exp(-x+a*math.Log(x)-lg)
	}
	// continued fraction (modified Lentz) for Q(a,x)
	const tiny = 1e-300
	b := x + 1 - a
	c := 1 / tiny
	d := 1 / b
	h := d
	for i := 1; i < 1000; i++ {
		an := -float64(i) * (float64(i) - a)
		b += 2
		d = an*d + b
		if math.Abs(d) < tiny {
			d = tiny
		}
		c = b + an/c
		if math.Abs(c) < tiny {
			c = tiny
		}
		d = 1 / d
		del := d * c
		h *= del
		if math.Abs(del-1) < 1e-16 {
			break
		}
	}
	return 1 - math.Exp(-x+a*math.Log(x)-lg)*h
}

// GammaCDF is the CDF of Gamma(shape, scale) at x.
func GammaCDF(x, shape, scale float64) float64 {
	if x <= 0 {
		return 0
	}
	return RegularizedGammaP(shape, x/scale)
}

// GammaFit is a mixed distribution for precipitation accumulations: a point mass at zero
// (probability ZeroProb) plus a gamma distribution for the positive values.
type GammaFit struct {
	Shape, Scale float64
	ZeroProb     float64 // q = (#zeros)/n
	N, NPositive int
}

// FitGammaThom fits Gamma(shape, scale) to the strictly positive values of xs with the Thom (1958)
// closed-form approximation of the maximum-likelihood estimator:
//
//	A     = ln(mean) - mean(ln x)
//	shape = (1 + sqrt(1 + 4A/3)) / (4A)
//	scale = mean / shape
//
// and records the share of zeros q. ok=false when it cannot be fitted (fewer than 3 positive
// values, or A <= 0, i.e. all positive values identical). Missing values are ignored.
func FitGammaThom(xs []float64) (GammaFit, bool) {
	var sum, sumLog float64
	var n, pos int
	for _, x := range xs {
		if IsMissing(x) || x < 0 {
			continue
		}
		n++
		if x > 0 {
			pos++
			sum += x
			sumLog += math.Log(x)
		}
	}
	if pos < 3 {
		return GammaFit{}, false
	}
	mean := sum / float64(pos)
	A := math.Log(mean) - sumLog/float64(pos)
	if !(A > 0) {
		return GammaFit{}, false
	}
	shape := (1 + math.Sqrt(1+4*A/3)) / (4 * A)
	return GammaFit{Shape: shape, Scale: mean / shape, ZeroProb: float64(n-pos) / float64(n), N: n, NPositive: pos}, true
}

// CDF is H(x) = q + (1-q) G(x) (McKee et al. 1993); H(0) = q.
func (g GammaFit) CDF(x float64) float64 {
	if x <= 0 {
		return g.ZeroProb
	}
	return g.ZeroProb + (1-g.ZeroProb)*GammaCDF(x, g.Shape, g.Scale)
}

// ───────────── log-logistic distribution (water-balance index) ─────────────

// LogLogistic is the 3-parameter log-logistic distribution F(x) = [1 + (Alpha/(x-Gamma))^Beta]^-1,
// defined for x > Gamma.
type LogLogistic struct{ Alpha, Beta, Gamma float64 }

// FitLogLogisticPWM fits the 3-parameter log-logistic by probability-weighted moments, following
// the parameterisation of Singh et al. (1993) used by Vicente-Serrano et al. (2010):
//
//	beta  = (2 w1 - w0) / (6 w1 - w0 - 6 w2)
//	alpha = (w0 - 2 w1) beta / (G(1+1/beta) G(1-1/beta))
//	gamma = w0 - alpha G(1+1/beta) G(1-1/beta)
//
// with w_s = E[X (1-F)^s]. The w_s are obtained from the UNBIASED PWM estimators
// b_r = E[X F^r] (Landwehr et al. 1979): b0 = mean, b1 = (1/n) sum (i-1)/(n-1) x(i),
// b2 = (1/n) sum (i-1)(i-2)/((n-1)(n-2)) x(i) on the ascending order statistics, via
// w0 = b0, w1 = b0 - b1, w2 = b0 - 2 b1 + b2.
// (Vicente-Serrano et al. use plotting-position PWMs; results differ slightly. See SIMULATION.md.)
// ok=false when n < 4, or when the fit is degenerate (beta <= 1 or non-finite parameters).
func FitLogLogisticPWM(xs []float64) (LogLogistic, bool) {
	v := make([]float64, 0, len(xs))
	for _, x := range xs {
		if !IsMissing(x) {
			v = append(v, x)
		}
	}
	n := len(v)
	if n < 4 {
		return LogLogistic{}, false
	}
	sort.Float64s(v)
	fn := float64(n)
	var b0, b1, b2 float64
	for i := 1; i <= n; i++ {
		fi := float64(i)
		b0 += v[i-1]
		b1 += (fi - 1) / (fn - 1) * v[i-1]
		b2 += (fi - 1) * (fi - 2) / ((fn - 1) * (fn - 2)) * v[i-1]
	}
	b0, b1, b2 = b0/fn, b1/fn, b2/fn
	w0, w1, w2 := b0, b0-b1, b0-2*b1+b2
	den := 6*w1 - w0 - 6*w2
	if den == 0 {
		return LogLogistic{}, false
	}
	beta := (2*w1 - w0) / den
	if !(beta > 1) || math.IsInf(beta, 0) {
		return LogLogistic{}, false
	}
	g := math.Gamma(1+1/beta) * math.Gamma(1-1/beta)
	alpha := (w0 - 2*w1) * beta / g
	gamma := w0 - alpha*g
	if !(alpha > 0) || math.IsNaN(gamma) || math.IsInf(gamma, 0) || math.IsInf(alpha, 0) {
		return LogLogistic{}, false
	}
	return LogLogistic{Alpha: alpha, Beta: beta, Gamma: gamma}, true
}

// CDF returns F(x); 0 for x <= Gamma (outside the support).
func (l LogLogistic) CDF(x float64) float64 {
	if x <= l.Gamma {
		return 0
	}
	return 1 / (1 + math.Pow(l.Alpha/(x-l.Gamma), l.Beta))
}

// Quantile is the inverse CDF, for p in (0,1).
func (l LogLogistic) Quantile(p float64) float64 {
	return l.Gamma + l.Alpha*math.Pow(p/(1-p), 1/l.Beta)
}
