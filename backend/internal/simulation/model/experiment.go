package model

import (
	"errors"
	"fmt"
	"strings"
)

// Thresholds of the climate_drought_v1 experiment. They are constants (not request parameters) on
// purpose: results must be comparable between runs. Documented in docs/simulations/SIMULATION.md.
const (
	// MinReferenceMonths: shortest accepted reference period (10 years). WMO recommends 30-year
	// normals; shorter baselines are accepted down to 10 years but flagged in the limitations.
	MinReferenceMonths = 120
	// MinMonthCoverage: share of reference months that must have both precipitation and temperature.
	MinMonthCoverage = 0.90
	// MinDayCoverage: share of a month's days that must have a value for the month to count.
	MinDayCoverage = 0.90
	// MinFitSamples: usable accumulated values required per calendar month to fit a distribution.
	MinFitSamples = 8
	// DefaultScale is the accumulation window (months) of the standardized indices.
	DefaultScale = 3
)

// Status values of an experiment.
const (
	StatusOK               = "ok"
	StatusInsufficientData = "insufficient_data"
	StatusError            = "error"
)

// Params are the user-chosen scenario parameters.
type Params struct {
	PrecipChangePct float64 `json:"precip_change_pct"`
	TempChangeC     float64 `json:"temp_change_c"`
	// Scale is the accumulation window in months of the SPI / water-balance index (1, 3 or 6).
	Scale int `json:"scale"`
}

// Input is everything the pure experiment needs.
type Input struct {
	Daily    []DailyRecord // observed reference-period daily series
	LatDeg   float64       // latitude of the region centroid (for day length)
	From, To YM            // reference period, inclusive
	Params   Params
}

// Indicator compares one quantity in the reference period and in the perturbed scenario.
// Change = Scenario - Reference in the indicator's own unit (percentage points for "%" units).
type Indicator struct {
	Key       string  `json:"key"`
	Label     string  `json:"label"`
	Unit      string  `json:"unit"`
	Reference float64 `json:"reference"`
	Scenario  float64 `json:"scenario"`
	Change    float64 `json:"change"`
}

// Point is one month of a chart series; nil means "no value for that month".
type Point struct {
	Month     string   `json:"month"`
	Reference *float64 `json:"reference"`
	Scenario  *float64 `json:"scenario"`
}

// Series is a monthly reference-vs-scenario series for charts.
type Series struct {
	Key    string  `json:"key"`
	Label  string  `json:"label"`
	Unit   string  `json:"unit"`
	Points []Point `json:"points"`
}

// Coverage describes how complete the reference data was.
type Coverage struct {
	Months      int     `json:"months"`       // months in the reference period
	ValidMonths int     `json:"valid_months"` // months with both precipitation and temperature
	Precip      float64 `json:"precip"`       // fraction of months with valid precipitation
	Temp        float64 `json:"temp"`         // fraction of months with valid temperature
	Both        float64 `json:"both"`         // fraction with both (== ValidMonths/Months)
	IndexMonths int     `json:"index_months"` // months for which the standardized indices exist
	// WBSIFallbackMonths: calendar months (0..12) where the water-balance index used a normal
	// distribution because a log-logistic could not be fitted (see FitWBSI).
	WBSIFallbackMonths int     `json:"wbsi_fallback_months"`
	Years              float64 `json:"years"` // Months/12
}

// Output is the complete result of a pure run. When Status != "ok" there are NO numbers
// (Indicators and Series are nil).
type Output struct {
	Status             string      `json:"status"`
	InsufficientReason string      `json:"insufficient_reason,omitempty"`
	Indicators         []Indicator `json:"indicators,omitempty"`
	Series             []Series    `json:"series,omitempty"`
	Coverage           Coverage    `json:"coverage"`
	Assumptions        []string    `json:"assumptions,omitempty"`
	Limitations        []string    `json:"limitations,omitempty"`
}

func insufficient(cov Coverage, format string, args ...any) Output {
	return Output{Status: StatusInsufficientData, InsufficientReason: fmt.Sprintf(format, args...), Coverage: cov,
		Limitations: LimitationsTR()}
}

// ErrInvalidParams is returned for parameters the experiment refuses (not a data problem).
var ErrInvalidParams = errors.New("model: invalid parameters")

// RunClimateDrought executes the climate_drought_v1 experiment. It is a pure function.
//
// Procedure (details and equations in docs/simulations/SIMULATION.md):
//  1. aggregate the observed daily series to months and check data sufficiency;
//  2. build the scenario by delta change (precip x (1+p/100), temp + dT) of the SAME months;
//  3. Thornthwaite PET for reference and scenario (heat index from each own climatology);
//  4. water balance D = P - PET;
//  5. fit SPI (gamma) and water-balance (log-logistic) distributions on the REFERENCE series only,
//     and apply those fixed distributions to both reference and scenario series — standardizing the
//     scenario against its own distribution would cancel exactly the signal being studied;
//  6. derive indicators and chart series.
func RunClimateDrought(in Input) (Output, error) {
	p := in.Params
	if p.Scale == 0 {
		p.Scale = DefaultScale
	}
	if p.Scale != 1 && p.Scale != 3 && p.Scale != 6 {
		return Output{}, fmt.Errorf("%w: scale must be 1, 3 or 6", ErrInvalidParams)
	}
	delta := Delta{PrecipChangePct: p.PrecipChangePct, TempChangeC: p.TempChangeC}
	if err := delta.Validate(); err != nil {
		return Output{}, fmt.Errorf("%w: %v", ErrInvalidParams, err)
	}
	if in.To.Index() < in.From.Index() {
		return Output{}, fmt.Errorf("%w: reference_to is before reference_from", ErrInvalidParams)
	}

	nMonths := MonthsInclusive(in.From, in.To)
	cov := Coverage{Months: nMonths, Years: float64(nMonths) / 12}
	if nMonths < MinReferenceMonths {
		return insufficient(cov, "Referans dönemi %d ay (%.1f yıl); güvenilir bir taban çizgisi için en az %d ay (10 yıl) gerekir.",
			nMonths, cov.Years, MinReferenceMonths), nil
	}

	monthly := MonthlyAggregate(in.Daily, in.From, in.To, MinDayCoverage)
	cov.Precip, cov.Temp, cov.Both = MonthlyCoverage(monthly)
	for _, m := range monthly {
		if m.Valid() {
			cov.ValidMonths++
		}
	}
	if cov.Both < MinMonthCoverage {
		return insufficient(cov, "Referans dönemindeki aylık veri kapsaması %%%.0f (yağış %%%.0f, sıcaklık %%%.0f); en az %%%.0f gerekir.",
			100*cov.Both, 100*cov.Precip, 100*cov.Temp, 100*MinMonthCoverage), nil
	}

	// Only months with BOTH variables are used: otherwise a missing temperature would silently
	// leave precipitation in the baseline while PET/water balance exclude the month.
	P := make([]float64, nMonths)
	T := make([]float64, nMonths)
	for i, m := range monthly {
		if m.Valid() {
			P[i], T[i] = m.Precip, m.Temp
		} else {
			P[i], T[i] = Missing, Missing
		}
	}
	Ps, Ts := delta.ApplyMonthly(P, T)

	climP := MonthlyClimatology(P, in.From)
	climT := MonthlyClimatology(T, in.From)
	if !climP.Complete(2) || !climT.Complete(2) {
		return insufficient(cov, "Referans döneminde en az bir takvim ayı için yeterli (en az 2 yıllık) veri yok; iklim normali hesaplanamadı."), nil
	}
	climPs := MonthlyClimatology(Ps, in.From)
	climTs := MonthlyClimatology(Ts, in.From)

	thRef := NewThornthwaite(climT.Mean, in.LatDeg)
	thScn := NewThornthwaite(climTs.Mean, in.LatDeg)
	PET := thRef.PETSeries(T, in.From)
	PETs := thScn.PETSeries(Ts, in.From)
	D := WaterBalance(P, PET)
	Ds := WaterBalance(Ps, PETs)

	k := p.Scale
	spiFit, err := FitSPI(Accumulate(P, k), in.From, MinFitSamples)
	if err != nil {
		return insufficient(cov, "SPI dağılımı uydurulamadı: %v.", trimErr(err)), nil
	}
	wbsiFit, err := FitWBSI(Accumulate(D, k), in.From, MinFitSamples)
	if err != nil {
		return insufficient(cov, "Su dengesi dağılımı uydurulamadı: %v.", trimErr(err)), nil
	}
	cov.WBSIFallbackMonths = wbsiFit.Fallbacks
	spi := spiFit.Standardize(Accumulate(P, k), in.From)
	spis := spiFit.Standardize(Accumulate(Ps, k), in.From)
	wbsi := wbsiFit.Standardize(Accumulate(D, k), in.From)
	wbsis := wbsiFit.Standardize(Accumulate(Ds, k), in.From)
	for _, v := range spi {
		if !IsMissing(v) {
			cov.IndexMonths++
		}
	}

	climPET := MonthlyClimatology(PET, in.From)
	climPETs := MonthlyClimatology(PETs, in.From)
	climD := MonthlyClimatology(D, in.From)
	climDs := MonthlyClimatology(Ds, in.From)

	zRef := PrecipAnomalyZ(P, in.From, climP)
	zScn := PrecipAnomalyZ(Ps, in.From, climP) // reference normal, NOT the scenario's own
	zr, _ := MeanValid(zRef)
	zs, _ := MeanValid(zScn)

	var ind []Indicator
	add := func(key, label, unit string, ref, scn float64) {
		ind = append(ind, Indicator{Key: key, Label: label, Unit: unit,
			Reference: round(ref, 2), Scenario: round(scn, 2), Change: round(scn-ref, 2)})
	}
	sum12 := func(a [12]float64) float64 {
		var s float64
		for _, v := range a {
			s += v
		}
		return s
	}
	annP, annPs := sum12(climP.Mean), sum12(climPs.Mean)
	add("precip_annual_mm", "Yıllık ortalama yağış", "mm/yıl", annP, annPs)
	add("temp_annual_c", "Yıllık ortalama sıcaklık", "°C", avg12(climT.Mean), avg12(climTs.Mean))
	add("pet_annual_mm", "Yıllık potansiyel evapotranspirasyon (Thornthwaite)", "mm/yıl", sum12(climPET.Mean), sum12(climPETs.Mean))
	add("water_balance_annual_mm", "Yıllık su dengesi (yağış − PET)", "mm/yıl", sum12(climD.Mean), sum12(climDs.Mean))
	add("precip_pct_of_normal", "Yağış (referans normalin yüzdesi)", "%", 100, PercentOfNormal(annPs, annP))
	add("precip_anomaly_z_mean", "Ortalama aylık yağış anomalisi (z)", "z", zr, zs)

	sk := fmt.Sprintf("-%d", k)
	sa, sb, _ := MeanPaired(spi, spis)
	add("spi_mean", "SPI"+sk+" ortalaması", "SPI", sa, sb)
	dr, ds, def := DryMonthsPaired(spi, spis)
	add("spi_dry_months", "Kurak ay sayısı (SPI"+sk+" ≤ −1)", "ay", float64(dr), float64(ds))
	add("spi_drought_fraction", "Kurak ay oranı (SPI"+sk+" ≤ −1)", "%", pct(dr, def), pct(ds, def))
	wa, wb, _ := MeanPaired(wbsi, wbsis)
	add("wbsi_mean", "Su dengesi standart indeksi"+sk+" ortalaması", "z", wa, wb)
	dr, ds, def = DryMonthsPaired(wbsi, wbsis)
	add("wbsi_dry_months", "Kurak ay sayısı (su dengesi indeksi"+sk+" ≤ −1)", "ay", float64(dr), float64(ds))
	add("wbsi_drought_fraction", "Kurak ay oranı (su dengesi indeksi"+sk+" ≤ −1)", "%", pct(dr, def), pct(ds, def))

	seasons := []struct {
		key, label string
		months     [3]int // 1-based
	}{
		{"djf", "Kış (Ara–Şub)", [3]int{12, 1, 2}},
		{"mam", "İlkbahar (Mar–May)", [3]int{3, 4, 5}},
		{"jja", "Yaz (Haz–Ağu)", [3]int{6, 7, 8}},
		{"son", "Sonbahar (Eyl–Kas)", [3]int{9, 10, 11}},
	}
	for _, s := range seasons {
		add("precip_"+s.key+"_mm", s.label+" yağışı", "mm/mevsim", seasonSum(climP.Mean, s.months), seasonSum(climPs.Mean, s.months))
	}
	for _, s := range seasons {
		add("water_balance_"+s.key+"_mm", s.label+" su dengesi", "mm/mevsim", seasonSum(climD.Mean, s.months), seasonSum(climDs.Mean, s.months))
	}

	mk := func(key, label, unit string, ref, scn []float64, places int) Series {
		pts := make([]Point, len(ref))
		for i := range ref {
			pts[i] = Point{Month: in.From.Add(i).String(), Reference: ptr(ref[i], places), Scenario: ptr(scn[i], places)}
		}
		return Series{Key: key, Label: label, Unit: unit, Points: pts}
	}
	series := []Series{
		mk("precipitation", "Aylık yağış", "mm", P, Ps, 3),
		mk("temperature", "Aylık ortalama sıcaklık", "°C", T, Ts, 3),
		mk("pet", "Aylık PET (Thornthwaite)", "mm", PET, PETs, 3),
		mk("water_balance", "Aylık su dengesi (P − PET)", "mm", D, Ds, 3),
		mk("spi", "SPI"+sk, "SPI", spi, spis, 3),
		mk("wbsi", "Su dengesi standart indeksi"+sk, "z", wbsi, wbsis, 3),
	}

	return Output{
		Status:      StatusOK,
		Indicators:  ind,
		Series:      series,
		Coverage:    cov,
		Assumptions: AssumptionsTR(in, p, cov),
		Limitations: LimitationsTR(),
	}, nil
}

func trimErr(err error) string { return strings.TrimPrefix(err.Error(), ErrFit.Error()+": ") }

func ptr(v float64, places int) *float64 {
	if IsMissing(v) {
		return nil
	}
	r := round(v, places)
	return &r
}

func pct(n, d int) float64 {
	if d == 0 {
		return 0
	}
	return 100 * float64(n) / float64(d)
}

func avg12(a [12]float64) float64 {
	var s float64
	for _, v := range a {
		s += v
	}
	return s / 12
}

func seasonSum(a [12]float64, months [3]int) float64 {
	return a[months[0]-1] + a[months[1]-1] + a[months[2]-1]
}
