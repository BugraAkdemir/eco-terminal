# Simulation: `climate_drought_v1`

Package: `backend/internal/simulation` (service), `.../model` (pure maths), `.../climate` (data).
Data: [docs/data_sources/climate.md](../data_sources/climate.md). Contract: `docs/api/openapi.yaml` (`/simulations*`, `/environment/measurements`).

**Golden rule (AGENTS.md):** every number comes from this deterministic code. An LLM (optional `Explainer`) may only put the already-computed numbers into words; it never computes, and any narrative containing a number that was not in its prompt is discarded.

## What this is, and what it is not

A **delta-change experiment on the past**. The user picks a reference period of observed (reanalysis) climate, e.g. 1991-2020, and a perturbation: precipitation x (1 + p/100), temperature + dT. The model recomputes water-related indicators for that same period with the perturbation and shows reference vs scenario.

It is **not** a climate projection, forecast or prediction. It contains no climate-model output, no emission scenario, no change of rainfall variability or seasonality, and **no ecological response**: it says nothing about species, habitats or populations. "What if each of those 30 years had been 20 % drier and 2 degC warmer" is the whole claim.

## Pipeline

1. **Data**: daily precipitation (mm) and mean temperature (degC) of the ERA5 cell nearest to the region centroid (`climate.Store.EnsureRegionData`, then read from `environmental_measurements`).
2. **Monthly aggregation** (`model.MonthlyAggregate`): precipitation = sum, temperature = mean. A month is valid when >= 90 % of its days have a value; a valid month with gaps is scaled by days_in_month / valid_days (precipitation only). Otherwise the month is missing. Only months with both variables enter the baseline.
3. **Sufficiency checks** (constants in `model/experiment.go`, not user-adjustable):
   - reference period < 120 months (10 years) -> `insufficient_data`;
   - months with both variables < 90 % of the period -> `insufficient_data`;
   - a calendar month with < 2 years of data, or < 8 usable accumulated values for a distribution fit -> `insufficient_data`;
   - no data and the provider cannot be reached/quota exhausted -> `insufficient_data` with the (Turkish) reason that the download failed.
   An insufficient result has **no indicators and no series**, only the reason, coverage and limitations.
4. **Scenario** (`model.Delta`): P_s = P x (1 + p/100) (p >= -100), T_s = T + dT, applied to the same months (linear, so applying to daily or monthly values is identical). With p = 0 and dT = 0 the scenario equals the reference **bit for bit** (tested).
5. **PET** (Thornthwaite), **water balance** D = P - PET, **SPI** and **water-balance standardized index**, accumulated over a window of k = 1, 3 (default) or 6 months.
6. **Indicators** and chart **series** (reference vs scenario).

## Equations (plain text)

### Thornthwaite PET (Thornthwaite 1948; Thornthwaite & Mather 1957)
- monthly heat term: i = (T / 5)^1.514 for T > 0, else 0
- annual heat index: I = sum over the 12 calendar months of i, computed from the **long-term mean monthly temperatures** of the climate in question (reference climatology for the reference series; the shifted climatology for the scenario, so the scenario's I and a rise with warming)
- exponent: a = 6.75e-7 I^3 - 7.71e-5 I^2 + 1.792e-2 I + 0.49239
- unadjusted PET (mm per 30-day month of 12 h days): PET0 = 16 (10 T / I)^a for 0 < T < 26.5; PET0 = -415.85 + 32.24 T - 0.43 T^2 for T >= 26.5; 0 for T <= 0
- adjusted: PET = PET0 x (N / 12) x (d / 30), N = mean daylight hours of the month at the region centroid latitude, d = days in month
- day length (FAO-56): delta = 0.409 sin(2 pi J / 365 - 1.39); ws = arccos(-tan(phi) tan(delta)); N = 24 ws / pi, averaged over the days of the month.

### Anomalies and climatology
- climatology: per calendar month mean and **sample** standard deviation (n-1) over the reference period
- precipitation anomaly: % of normal = 100 P / mean_month; z = (P - mean_month) / sd_month. For the scenario both use the **reference** normal, never the scenario's own.

### SPI-style index (`spi_*`)
McKee et al. (1993) procedure, per calendar month and window k:
1. accumulate k-month precipitation totals (a window with any missing month is missing);
2. fit, on the **reference** values of that calendar month, a gamma distribution to the positive totals with the **Thom (1958)** closed-form approximation of the maximum-likelihood estimator: A = ln(mean) - mean(ln x); shape = (1 + sqrt(1 + 4A/3)) / (4A); scale = mean / shape; and the share of zeros q;
3. H(x) = q + (1 - q) G(x), with G the gamma CDF (H(0) = q);
4. SPI = Phi^-1(H(x)) (standard normal quantile), H clipped to [1e-6, 1 - 1e-6] (|SPI| <= about 4.75).
Not verified against the R `SPEI`/`SPI` packages; the algorithm is the textbook one.

### Water-balance standardized index (`wbsi_*`) - **not** called SPEI
Follows the idea of the SPEI (Vicente-Serrano et al. 2010) with these exact choices:
1. D = P - PET (Thornthwaite PET), accumulated over k months;
2. per calendar month, fit a 3-parameter **log-logistic** F(x) = [1 + (alpha / (x - gamma))^beta]^-1 to the **reference** values by probability-weighted moments: unbiased PWM estimators b0, b1, b2 (Landwehr 1979), w0 = b0, w1 = b0 - b1, w2 = b0 - 2 b1 + b2; beta = (2 w1 - w0) / (6 w1 - w0 - 6 w2); alpha = (w0 - 2 w1) beta / (Gamma(1 + 1/beta) Gamma(1 - 1/beta)); gamma = w0 - alpha Gamma(1 + 1/beta) Gamma(1 - 1/beta);
3. index = Phi^-1(F(x)), clipped as above.
**Deviations from the standard SPEI, stated honestly:** (a) unbiased PWMs instead of the paper's plotting-position PWMs; (b) the log-logistic requires positive skew (L-skewness in (0,1)); in calendar months where the reference sample is symmetric or negatively skewed (typical in summer, when PET dominates) it cannot be fitted, so a **normal distribution** (sample mean and sd) is used for that month. The number of such months is reported in `assumptions` and `coverage.wbsi_fallback_months` (4 of 12 for the Kastamonu 1991-2020 run); (c) Thornthwaite's I comes from the climatology, not per year; (d) no cross-check against the R package was done. Therefore: "water-balance standardized index", `wbsi_*`.

### Same-baseline rule (why the scenario shows a signal)
The distributions of SPI and WBSI are fitted **once, on the reference series**, and applied unchanged to both reference and scenario. If the scenario were standardized against its own distribution, a pure precipitation scaling would cancel out exactly (the gamma family is closed under scaling: scale x c, same shape -> identical SPI). `TestSPIStandardization` proves this cancellation numerically and `TestScenarioUsesReferenceBaseline` / `TestLessPrecipLowersIndicesMonotonically` prove the baseline-fixed behaviour.
Consequence: **reference** indicators are standardized against themselves: mean SPI = 0 and about 16 % of months with index <= -1 by construction (14.5 % in the Kastamonu run because of finite sample/fit). Only the **shift** in the scenario is information.

### Drought months
Dry month: index <= -1. `*_dry_months` counts months, `*_drought_fraction` is the percentage, both over the months where reference **and** scenario indices exist (same months).

## Indicators (`SimulationResult.indicators[]`, `change` = scenario - reference in the unit shown)

| key | unit | how to read it |
|---|---|---|
| `precip_annual_mm` | mm/yr | sum over calendar months of the mean monthly precipitation |
| `temp_annual_c` | degC | mean of the 12 monthly mean temperatures |
| `pet_annual_mm` | mm/yr | Thornthwaite PET; temperature-only, see limitations |
| `water_balance_annual_mm` | mm/yr | P - PET; negative = PET exceeds precipitation on average (does not mean the region is dry: soil storage, snow, runoff are not modelled) |
| `precip_pct_of_normal` | % | reference is 100 by definition; scenario = 100 x annual scenario P / reference P (change in percentage points) |
| `precip_anomaly_z_mean` | z | mean monthly z-score of precipitation against the **reference** normal (reference ~ 0) |
| `spi_mean` | SPI | mean standardized precipitation index (window k); < 0 drier than the reference |
| `spi_dry_months`, `spi_drought_fraction` | months, % | months with SPI <= -1 |
| `wbsi_mean`, `wbsi_dry_months`, `wbsi_drought_fraction` | z, months, % | same for the water-balance standardized index |
| `precip_{djf,mam,jja,son}_mm` | mm/season | mean seasonal precipitation (DJF = Dec-Feb, ...) |
| `water_balance_{djf,mam,jja,son}_mm` | mm/season | mean seasonal P - PET |

`series[]` (monthly, `reference` and `scenario`, `null` where a month has no value): `precipitation`, `temperature`, `pet`, `water_balance`, `spi`, `wbsi`.
Values are rounded to 2 decimals (indicators) / 3 decimals (series).

## Example (real data, Kastamonu ERA5 cell, reference 1991-01..2020-12, precipitation -20 %, +2 degC, window 3)

| indicator | reference | scenario | change |
|---|---|---|---|
| annual precipitation (mm) | 603.11 | 482.49 | -120.62 |
| annual mean temperature (degC) | 11.59 | 13.59 | +2.00 |
| annual PET Thornthwaite (mm) | 689.69 | 761.43 | +71.74 |
| annual water balance P - PET (mm) | -86.58 | -278.94 | -192.36 |
| SPI-3 mean | 0.00 | -0.73 | -0.73 |
| months with SPI-3 <= -1 (of 358) | 52 | 122 | +70 |
| months with water-balance index <= -1 | 57 | 186 | +129 |
| summer (JJA) water balance (mm) | -254.88 | -311.71 | -56.82 |

(Reproduced by `TestRunKastamonuWithRealSnapshotAndPersistRoundTrip -v`.) These ERA5 absolute values are for a 25 km grid cell and are not the city's station climate.

## Assumptions and limits - read before using a number

- **Not a forecast.** Delta change keeps the observed year-to-year sequence, variability and seasonality; real climate change alters these. Choose p and dT as exploratory "what-if" values, not as predictions; no probability is attached to them.
- **Grid vs region.** One ERA5 0.25 deg cell (about 25 km) per region, nearest to the centroid. Kastamonu is mountainous (Ilgaz, Küre); elevation, rain shadow and local convection are averaged out. Large regions are represented by one point. Station data (MGM) is not used.
- **Reanalysis, not observation.** ERA5 precipitation is model-derived and known to be biased in complex terrain; ERA5 daily mean temperature of the cell differs from a station at 780 m.
- **Thornthwaite** uses only temperature and latitude. It ignores radiation, humidity, wind and CO2/vegetation effects. Compared with Penman-Monteith it tends to under-estimate PET in arid and over-estimate in humid climates, and it is known to **over-state the PET increase under warming** (so water-balance deterioration in warming scenarios is probably exaggerated). The heat index I is recomputed for the warmed climate as the method prescribes; this partly offsets the temperature effect and is a documented modelling choice. Published check: the method reproduces a textbook worked example (below).
- **Short baselines.** WMO recommends 30-year normals. 10 years is the enforced minimum; 10-20 years give unstable gamma/log-logistic fits (a calendar-month fit uses 8+ values).
- **Drought definition.** SPI/WBSI measure anomalies relative to the chosen reference period; "dry" always means "dry relative to that period". They do not represent soil moisture, hydrological or agricultural drought, snow or groundwater.
- **No uncertainty bands.** Results are one deterministic computation; they carry no confidence interval. Differences of a few hundredths of an index unit are not meaningful.
- **No ecological claim.** Nothing here measures species or population response. Citizen-observation counts elsewhere in the platform must not be combined with these indicators to claim population trends.

The Turkish `assumptions[]` and `limitations[]` returned by the API repeat the points above (Turkish is the primary UI language).

## Data-handling rules
- No data and no provider -> `insufficient_data`, never fabricated numbers.
- A provider outage during a run is not an error: the run uses whatever is stored; if that is not enough the reason says the download failed.
- Each run persists a scenario (`simulation_scenarios`, `params_json`) and a result (`simulation_results.result_json`; narrative in its own column). Re-running creates a new scenario; names are free text (default generated).

## Extending: `Model` registry
`simulation.Model` (`Name`, `Normalize(RunRequest) (Params, error)`, `Run(ctx, Env, Params) (Outcome, error)`) + `Registry`. `climate_drought_v1` is registered by `NewService`; add e.g. a habitat-fragmentation or water-stress model by implementing the interface and calling `Registry.Register`. Service, persistence, `Compare` and the HTTP layer stay unchanged (`TestRegistryExtensibility`).

## Narrative (optional)
`Service.Explainer` (interface `Explain(ctx, prompt string) (string, error)`) is called only for `explain=true`, status `ok`. The prompt (`BuildExplainPrompt`) lists every indicator with its computed values and forbids new numbers, rounding/derivation, forecasts and ecological claims; it fences the data in `<VERI>...</VERI>`. The answer is dropped (result still returned, `narrative: null`) if the explainer is nil, errors, panics, times out (default 45 s), is empty, or contains a number not found in the prompt (a quoted prompt number rounded to 0/1 decimals is allowed). The narrative never modifies `indicators`, `series`, `params`.

## Test evidence (`cd backend && make vet && make test`)
- Thornthwaite vs a published worked example (Landtek Ltd, Project 21260, Appendix H, Hamilton climate data): unadjusted PET reproduced to 1e-6 with the printed I and a; adjusted PET within 4 % (largest 2.8 %, October) with FAO-56 day length. Two inconsistencies of that spreadsheet are documented in `pet_test.go` (its printed I = 41.3085 vs the 41.339 sum of its own monthly indices; its March correction factor 1.12 equal to April's).
- Closed forms: incomplete gamma vs `1 - e^-x`, `erf`, `1 - e^-x (1 + x)`; normal quantile vs 1.959964; Thom estimator vs a hand calculation ({1, e, e^2} -> shape 1.7705, scale 2.0913); log-logistic parameter recovery from a quantile grid; zero-precipitation mapping Phi^-1(q) = -0.8416 for q = 0.2.
- Invariants: p = 0, dT = 0 -> scenario == reference exactly; less precipitation -> SPI, WBSI, water balance never higher (pointwise) and dry-month count never lower, monotone in the size of the cut; more warming -> PET never lower, water balance never higher, precipitation indicators untouched; scenario standardized with reference fit (and the self-fit cancellation proof).
- Data/service: insufficient-data paths (short period, low coverage, provider outage, no provider) all return reasons and no numbers; idempotent ingestion against an `httptest` Open-Meteo double that serves slices of the real download; retry/back-off/429/quota handling; fetch ledger (only missing years requested); persistence round trip; narrative isolation (numbers identical with/without explainer, hallucinated number dropped, failing/panicking/hanging explainer harmless).
