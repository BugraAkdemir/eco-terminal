package model

import (
	"math"
	"math/rand"
	"testing"
)

func TestNormalQuantileAndCDF(t *testing.T) {
	near(t, "cdf(1)", NormalCDF(1), 0.8413447460685429, 1e-14)
	near(t, "cdf(0)", NormalCDF(0), 0.5, 1e-15)
	near(t, "q(0.975)", NormalQuantile(0.975), 1.959963984540054, 1e-9)
	near(t, "q(0.5)", NormalQuantile(0.5), 0, 1e-12)
	near(t, "q(0.001)", NormalQuantile(0.001), -3.090232306167813, 1e-9)
	near(t, "q(0.8413447460685429)", NormalQuantile(0.8413447460685429), 1, 1e-9)
	for _, p := range []float64{1e-8, 1e-6, 0.01, 0.02, 0.3, 0.5, 0.7, 0.98, 0.999, 1 - 1e-7} {
		near(t, "roundtrip", NormalCDF(NormalQuantile(p)), p, 1e-12+p*1e-9)
	}
	if !math.IsInf(NormalQuantile(0), -1) || !math.IsInf(NormalQuantile(1), 1) || !math.IsNaN(NormalQuantile(1.5)) {
		t.Fatal("edge cases")
	}
}

func TestRegularizedGammaP(t *testing.T) {
	// closed forms: P(1,x)=1-e^-x ; P(1/2,x)=erf(sqrt x) ; P(2,x)=1-e^-x(1+x) ; P(3,5)=1-18.5 e^-5
	for _, x := range []float64{0.1, 0.5, 1, 2.9, 3.1, 10, 40} {
		near(t, "P(1,x)", RegularizedGammaP(1, x), 1-math.Exp(-x), 1e-13)
		near(t, "P(.5,x)", RegularizedGammaP(0.5, x), math.Erf(math.Sqrt(x)), 1e-13)
		near(t, "P(2,x)", RegularizedGammaP(2, x), 1-math.Exp(-x)*(1+x), 1e-13)
	}
	near(t, "P(3,5)", RegularizedGammaP(3, 5), 1-18.5*math.Exp(-5), 1e-13)
	if RegularizedGammaP(2, 0) != 0 || !math.IsNaN(RegularizedGammaP(-1, 1)) {
		t.Fatal("edge cases")
	}
	// chi-square(2 dof) = Gamma(1, 2): CDF(2) = 1 - e^-1 = 0.6321
	near(t, "chi2", GammaCDF(2, 1, 2), 1-math.Exp(-1), 1e-13)
}

// Hand calculation of the Thom estimator for {1, e, e^2}:
//
//	mean = (1+2.71828+7.38906)/3 = 3.70245 ; ln(mean) = 1.30900 ; mean(ln x) = (0+1+2)/3 = 1
//	A = 0.30900 ; shape = (1+sqrt(1+4A/3))/(4A) = (1+1.18827)/1.23598 = 1.77048 ; scale = mean/shape = 2.0913
func TestFitGammaThomHandCalculation(t *testing.T) {
	g, ok := FitGammaThom([]float64{1, math.E, math.E * math.E})
	if !ok {
		t.Fatal("fit failed")
	}
	near(t, "shape", g.Shape, 1.77048, 2e-4)
	near(t, "scale", g.Scale, 2.0913, 2e-3)
	if g.ZeroProb != 0 || g.N != 3 || g.NPositive != 3 {
		t.Fatalf("%+v", g)
	}
	// zeros are counted into q and excluded from the gamma fit
	g2, ok := FitGammaThom([]float64{0, 0, 1, math.E, math.E * math.E, Missing})
	if !ok {
		t.Fatal("fit with zeros failed")
	}
	near(t, "q", g2.ZeroProb, 0.4, 1e-12)
	near(t, "shape with zeros", g2.Shape, g.Shape, 1e-12)
	near(t, "H(0)=q", g2.CDF(0), 0.4, 1e-15)
	// H is q + (1-q)G: at the positive-part median it equals q + (1-q)/2
	near(t, "H(inf)", g2.CDF(1e9), 1, 1e-12)
	// degenerate samples
	if _, ok := FitGammaThom([]float64{5, 5, 5, 5}); ok {
		t.Fatal("identical values must not fit (A=0)")
	}
	if _, ok := FitGammaThom([]float64{0, 0, 0, 1, 2}); ok {
		t.Fatal("fewer than 3 positive values must not fit")
	}
}

func TestFitGammaThomRecoversParameters(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	xs := make([]float64, 20000)
	for i := range xs {
		xs[i] = 10 * (-math.Log(1-r.Float64()) - math.Log(1-r.Float64())) // Gamma(shape 2, scale 10)
	}
	g, ok := FitGammaThom(xs)
	if !ok {
		t.Fatal("fit failed")
	}
	near(t, "shape", g.Shape, 2, 0.1)
	near(t, "scale", g.Scale, 10, 0.5)
}

func TestFitLogLogisticRecoversParameters(t *testing.T) {
	// deterministic quantile grid of LogLogistic(alpha=50, beta=4, gamma=-20)
	truth := LogLogistic{Alpha: 50, Beta: 4, Gamma: -20}
	n := 1000
	xs := make([]float64, n)
	for i := range xs {
		xs[i] = truth.Quantile((float64(i) + 0.5) / float64(n))
	}
	// shuffle (fit must not depend on input order)
	rand.New(rand.NewSource(7)).Shuffle(n, func(i, j int) { xs[i], xs[j] = xs[j], xs[i] })
	f, ok := FitLogLogisticPWM(xs)
	if !ok {
		t.Fatal("fit failed")
	}
	near(t, "beta", f.Beta, 4, 0.2)
	near(t, "alpha", f.Alpha, 50, 2.5)
	near(t, "gamma", f.Gamma, -20, 2.5)
	// median of the log-logistic is gamma + alpha
	near(t, "F(median)", f.CDF(f.Gamma+f.Alpha), 0.5, 1e-12)
	near(t, "F(gamma)", f.CDF(f.Gamma-1), 0, 0)
	near(t, "quantile roundtrip", f.CDF(f.Quantile(0.8)), 0.8, 1e-12)
	if _, ok := FitLogLogisticPWM([]float64{1, 2, 3}); ok {
		t.Fatal("n<4 must not fit")
	}
}

func TestAccumulate(t *testing.T) {
	got := Accumulate([]float64{1, 2, 3, 4, 5}, 2)
	want := []float64{Missing, 3, 5, 7, 9}
	for i := range want {
		if IsMissing(want[i]) != IsMissing(got[i]) || (!IsMissing(want[i]) && got[i] != want[i]) {
			t.Fatalf("i=%d got %v want %v", i, got[i], want[i])
		}
	}
	got = Accumulate([]float64{1, Missing, 3, 4, 5}, 3)
	if !IsMissing(got[2]) || !IsMissing(got[3]) || got[4] != 12 {
		t.Fatalf("missing windows: %v", got)
	}
	got = Accumulate([]float64{4, 5}, 1)
	if got[0] != 4 || got[1] != 5 {
		t.Fatal("scale 1 is identity")
	}
}

func TestSPIStandardization(t *testing.T) {
	// 12 years x 12 months; every calendar month has 2 zero months of 10... build: Jan series
	// 0,0,1,2,3,4,5,6,7,8 (q=0.2), other months similar but shifted.
	start := YM{2000, 1}
	n := 10 * 12
	P := make([]float64, n)
	for i := range P {
		yr := i / 12
		if yr < 2 {
			P[i] = 0
		} else {
			P[i] = float64(yr-1) + float64(i%12)*0.1
		}
	}
	fit, err := FitSPI(Accumulate(P, 1), start, 8)
	if err != nil {
		t.Fatal(err)
	}
	z := fit.Standardize(P, start)
	// a zero value maps to Phi^-1(q) = Phi^-1(0.2) = -0.8416212
	near(t, "z(0)", z[0], -0.8416212335729143, 1e-9)
	near(t, "z(0) other month", z[5], -0.8416212335729143, 1e-9)
	// larger precipitation => larger index within a calendar month
	for yr := 3; yr < 10; yr++ {
		if !(z[yr*12] > z[(yr-1)*12]) {
			t.Fatalf("not monotone at year %d", yr)
		}
	}
	// refitting not allowed: standardizing an inflated series against the reference fit raises z
	P2 := make([]float64, n)
	for i := range P {
		P2[i] = P[i] * 1.5
	}
	z2 := fit.Standardize(P2, start)
	for i := range z {
		if z2[i] < z[i] {
			t.Fatalf("1.5x precipitation lowered index at %d", i)
		}
	}
	// but fitting the scaled series on ITSELF reproduces the original z exactly (gamma scale
	// family): this is why the scenario must be judged against the reference distribution.
	fit2, err := FitSPI(Accumulate(P2, 1), start, 8)
	if err != nil {
		t.Fatal(err)
	}
	z3 := fit2.Standardize(P2, start)
	for i := range z {
		if IsMissing(z[i]) != IsMissing(z3[i]) || (!IsMissing(z[i]) && math.Abs(z[i]-z3[i]) > 1e-9) {
			t.Fatalf("self-standardization changed z at %d: %v vs %v", i, z[i], z3[i])
		}
	}
	// too few samples => error
	if _, err := FitSPI(Accumulate(P[:5*12], 1), start, 8); err == nil {
		t.Fatal("want ErrFit for 5 years with min 8")
	}
}

func TestDryMonths(t *testing.T) {
	idx := []float64{0, -1, -1.5, -0.99, Missing, 2, -3}
	dry, def, frac := DryMonths(idx)
	if dry != 3 || def != 6 || math.Abs(frac-0.5) > 1e-12 {
		t.Fatalf("dry=%d defined=%d frac=%v", dry, def, frac)
	}
	a := []float64{-1, -2, 0, Missing}
	b := []float64{0, -2, -1, -5}
	da, db, n := DryMonthsPaired(a, b)
	if da != 2 || db != 2 || n != 3 {
		t.Fatalf("%d %d %d", da, db, n)
	}
}
