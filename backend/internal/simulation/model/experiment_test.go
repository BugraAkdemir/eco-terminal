package model

import (
	"errors"
	"math"
	"math/rand"
	"testing"
	"time"
)

// synthDaily builds a deterministic synthetic climate for years y0..y1 (NOT real data; only used to
// exercise invariants): seasonal temperature (mean 9 degC, amplitude 11) with noise, and wet days
// (probability 0.35) with exponential amounts whose scale follows a seasonal cycle.
func synthDaily(y0, y1 int, seed int64) []DailyRecord {
	r := rand.New(rand.NewSource(seed))
	var out []DailyRecord
	for d := time.Date(y0, 1, 1, 0, 0, 0, 0, time.UTC); d.Year() <= y1; d = d.AddDate(0, 0, 1) {
		doy := float64(d.YearDay())
		temp := 9 - 11*math.Cos(2*math.Pi*(doy-15)/365) + r.NormFloat64()*2
		p := 0.0
		if r.Float64() < 0.35 {
			p = -math.Log(1-r.Float64()) * (4 + 2*math.Cos(2*math.Pi*(doy-100)/365))
		}
		out = append(out, DailyRecord{Date: d, Precip: p, Temp: temp})
	}
	return out
}

func run(t *testing.T, days []DailyRecord, p Params) Output {
	t.Helper()
	out, err := RunClimateDrought(Input{Daily: days, LatDeg: 41.4, From: YM{1991, 1}, To: YM{2020, 12}, Params: p})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func ind(t *testing.T, o Output, key string) Indicator {
	t.Helper()
	for _, i := range o.Indicators {
		if i.Key == key {
			return i
		}
	}
	t.Fatalf("indicator %q missing", key)
	return Indicator{}
}

func ser(t *testing.T, o Output, key string) Series {
	t.Helper()
	for _, s := range o.Series {
		if s.Key == key {
			return s
		}
	}
	t.Fatalf("series %q missing", key)
	return Series{}
}

func TestIdentityScenarioEqualsReference(t *testing.T) {
	days := synthDaily(1991, 2020, 1)
	for _, scale := range []int{1, 3, 6} {
		o := run(t, days, Params{Scale: scale})
		if o.Status != StatusOK {
			t.Fatalf("scale %d: %s %s", scale, o.Status, o.InsufficientReason)
		}
		for _, i := range o.Indicators {
			if i.Change != 0 && !(i.Key == "precip_pct_of_normal" || i.Key == "precip_anomaly_z_mean") {
				t.Fatalf("scale %d: indicator %s changed by %v under the identity scenario", scale, i.Key, i.Change)
			}
			if i.Key == "precip_pct_of_normal" && i.Scenario != 100 {
				t.Fatalf("pct of normal: %v", i.Scenario)
			}
		}
		for _, s := range o.Series {
			for _, p := range s.Points {
				if (p.Reference == nil) != (p.Scenario == nil) || (p.Reference != nil && *p.Reference != *p.Scenario) {
					t.Fatalf("series %s month %s: reference != scenario", s.Key, p.Month)
				}
			}
		}
		// the z-mean must also be identical (reference vs itself)
		z := ind(t, o, "precip_anomaly_z_mean")
		if z.Reference != z.Scenario {
			t.Fatalf("z-mean %v vs %v", z.Reference, z.Scenario)
		}
	}
}

func TestReferenceIsStandardizedByConstruction(t *testing.T) {
	o := run(t, synthDaily(1991, 2020, 2), Params{})
	spi := ind(t, o, "spi_mean")
	if math.Abs(spi.Reference) > 0.1 {
		t.Fatalf("mean SPI of the reference period should be ~0, got %v", spi.Reference)
	}
	f := ind(t, o, "spi_drought_fraction").Reference
	if f < 10 || f > 22 { // N(0,1): P(z<=-1) = 15.9 %
		t.Fatalf("reference drought fraction %v%%, want ~16%%", f)
	}
	f = ind(t, o, "wbsi_drought_fraction").Reference
	if f < 10 || f > 22 {
		t.Fatalf("reference wbsi drought fraction %v%%, want ~16%%", f)
	}
	if o.Coverage.Both != 1 || o.Coverage.Months != 360 {
		t.Fatalf("%+v", o.Coverage)
	}
}

func TestLessPrecipLowersIndicesMonotonically(t *testing.T) {
	days := synthDaily(1991, 2020, 3)
	prev := run(t, days, Params{PrecipChangePct: 0})
	for _, pct := range []float64{-10, -20, -40, -60} {
		o := run(t, days, Params{PrecipChangePct: pct})
		// pointwise: the scenario index is <= the reference index in every month
		for _, key := range []string{"spi", "wbsi", "water_balance", "precipitation"} {
			for _, pt := range ser(t, o, key).Points {
				if pt.Reference != nil && pt.Scenario != nil && *pt.Scenario > *pt.Reference+1e-9 {
					t.Fatalf("p=%v %s %s: scenario %v > reference %v", pct, key, pt.Month, *pt.Scenario, *pt.Reference)
				}
			}
		}
		// aggregate indicators move in the dry direction, and further with a larger cut
		for _, key := range []string{"spi_mean", "wbsi_mean", "water_balance_annual_mm", "precip_annual_mm"} {
			if ind(t, o, key).Change >= 0 {
				t.Fatalf("p=%v: %s change %v should be negative", pct, key, ind(t, o, key).Change)
			}
			if ind(t, o, key).Scenario > ind(t, prev, key).Scenario+1e-9 {
				t.Fatalf("p=%v: %s not monotone vs previous cut", pct, key)
			}
		}
		if ind(t, o, "spi_dry_months").Scenario < ind(t, prev, "spi_dry_months").Scenario {
			t.Fatalf("p=%v: dry month count decreased", pct)
		}
		if ind(t, o, "spi_dry_months").Change <= 0 {
			t.Fatalf("p=%v: expected more dry months", pct)
		}
		// PET is temperature only: unchanged by a precipitation scenario
		if ind(t, o, "pet_annual_mm").Change != 0 {
			t.Fatal("PET must not depend on precipitation")
		}
		prev = o
	}
	// exact scaling of the annual precipitation: -20 % => x0.8
	o := run(t, days, Params{PrecipChangePct: -20})
	p := ind(t, o, "precip_annual_mm")
	if math.Abs(p.Scenario-0.8*p.Reference) > 0.01 {
		t.Fatalf("annual precip %v vs 0.8 x %v", p.Scenario, p.Reference)
	}
	if v := ind(t, o, "precip_pct_of_normal").Scenario; math.Abs(v-80) > 0.01 {
		t.Fatalf("pct of normal %v", v)
	}
	// more precipitation => wetter
	w := run(t, days, Params{PrecipChangePct: 25})
	if ind(t, w, "spi_mean").Change <= 0 || ind(t, w, "water_balance_annual_mm").Change <= 0 {
		t.Fatal("more precipitation must raise SPI / water balance")
	}
}

func TestWarmingRaisesPETAndLowersWaterBalance(t *testing.T) {
	days := synthDaily(1991, 2020, 4)
	for _, dt := range []float64{0.5, 1, 2, 4, 6} {
		o := run(t, days, Params{TempChangeC: dt})
		for _, pt := range ser(t, o, "pet").Points {
			if pt.Scenario != nil && *pt.Scenario < *pt.Reference-1e-9 {
				t.Fatalf("dT=%v %s: PET fell from %v to %v", dt, pt.Month, *pt.Reference, *pt.Scenario)
			}
		}
		for _, pt := range ser(t, o, "water_balance").Points {
			if pt.Scenario != nil && *pt.Scenario > *pt.Reference+1e-9 {
				t.Fatalf("dT=%v %s: water balance rose", dt, pt.Month)
			}
		}
		if ind(t, o, "pet_annual_mm").Change <= 0 || ind(t, o, "temp_annual_c").Change <= dt-0.01 || ind(t, o, "temp_annual_c").Change >= dt+0.01 {
			t.Fatalf("dT=%v: %+v / %+v", dt, ind(t, o, "pet_annual_mm"), ind(t, o, "temp_annual_c"))
		}
		if ind(t, o, "wbsi_mean").Change >= 0 {
			t.Fatalf("dT=%v: wbsi_mean should fall", dt)
		}
		// precipitation untouched by a temperature-only scenario => SPI unchanged
		if ind(t, o, "spi_mean").Change != 0 || ind(t, o, "precip_annual_mm").Change != 0 {
			t.Fatal("temperature must not affect SPI / precipitation")
		}
	}
}

func TestInsufficientData(t *testing.T) {
	// reference period shorter than 10 years
	days := synthDaily(2010, 2018, 5)
	o, err := RunClimateDrought(Input{Daily: days, LatDeg: 41.4, From: YM{2010, 1}, To: YM{2018, 12}})
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != StatusInsufficientData || o.InsufficientReason == "" || o.Indicators != nil || o.Series != nil {
		t.Fatalf("short period: %+v", o.Status)
	}
	// exactly 10 years is accepted
	o, _ = RunClimateDrought(Input{Daily: synthDaily(2010, 2019, 5), LatDeg: 41.4, From: YM{2010, 1}, To: YM{2019, 12}, Params: Params{Scale: 6}})
	if o.Status != StatusOK {
		t.Fatalf("10 years scale 6 should be ok: %s", o.InsufficientReason)
	}
	// coverage below 90 %: drop every precipitation value of 15 % of months
	full := synthDaily(1991, 2020, 6)
	for i := range full {
		if (full[i].Date.Year()*12+int(full[i].Date.Month()))%7 == 0 { // 1/7 = 14.3 % of months wiped
			full[i].Precip = Missing
		}
	}
	o = run(t, full, Params{})
	if o.Status != StatusInsufficientData || o.Indicators != nil || o.Series != nil || o.Coverage.Both >= 0.9 {
		t.Fatalf("low coverage: %s %+v", o.Status, o.Coverage)
	}
	// 8 % of months missing is tolerated (coverage 92 %)
	ok := synthDaily(1991, 2020, 6)
	for i := range ok {
		if (ok[i].Date.Year()*12+int(ok[i].Date.Month()))%13 == 0 {
			ok[i].Precip = Missing
		}
	}
	o = run(t, ok, Params{})
	if o.Status != StatusOK || o.Coverage.Both < 0.9 || o.Coverage.Both > 0.95 {
		t.Fatalf("92%% coverage should work: %s %s %+v", o.Status, o.InsufficientReason, o.Coverage)
	}
	// no data at all
	o = run(t, nil, Params{})
	if o.Status != StatusInsufficientData || o.Coverage.Both != 0 {
		t.Fatal("no data")
	}
}

func TestInvalidParams(t *testing.T) {
	days := synthDaily(1991, 2020, 7)
	for _, p := range []Params{{Scale: 2}, {Scale: 12}, {PrecipChangePct: -101}, {TempChangeC: math.NaN()}} {
		_, err := RunClimateDrought(Input{Daily: days, LatDeg: 41, From: YM{1991, 1}, To: YM{2020, 12}, Params: p})
		if !errors.Is(err, ErrInvalidParams) {
			t.Fatalf("%+v: want ErrInvalidParams, got %v", p, err)
		}
	}
	if _, err := RunClimateDrought(Input{Daily: days, From: YM{2020, 1}, To: YM{1991, 1}}); !errors.Is(err, ErrInvalidParams) {
		t.Fatal("reversed period")
	}
}

func TestDeterministic(t *testing.T) {
	days := synthDaily(1991, 2020, 8)
	a := run(t, days, Params{PrecipChangePct: -20, TempChangeC: 2})
	b := run(t, days, Params{PrecipChangePct: -20, TempChangeC: 2})
	for i := range a.Indicators {
		if a.Indicators[i] != b.Indicators[i] {
			t.Fatalf("non-deterministic %s", a.Indicators[i].Key)
		}
	}
	if len(a.Assumptions) == 0 || len(a.Limitations) == 0 {
		t.Fatal("assumptions / limitations must be filled")
	}
}

// The scenario must be standardized with the REFERENCE distributions. If it were standardized
// against its own distribution the SPI signal of a pure precipitation scaling would vanish
// (gamma scale family) — this test documents that this is NOT what the experiment does.
func TestScenarioUsesReferenceBaseline(t *testing.T) {
	days := synthDaily(1991, 2020, 9)
	o := run(t, days, Params{PrecipChangePct: -30})
	if ind(t, o, "spi_mean").Scenario > -0.3 {
		t.Fatalf("a 30%% cut must show up clearly in SPI, got %v", ind(t, o, "spi_mean").Scenario)
	}
	// reference-periodicity sanity: dry-month fraction in the scenario is well above 16 %
	if ind(t, o, "spi_drought_fraction").Scenario < 30 {
		t.Fatalf("drought fraction %v", ind(t, o, "spi_drought_fraction").Scenario)
	}
}
