# Climate data source: Open-Meteo Historical Weather API (ERA5)

Used by `backend/internal/simulation/climate` and the `climate_drought_v1` model.
This page records what was **actually verified** (call, date, result) and what was only read from the provider's pages.

## Dataset record

| Field | Value |
|---|---|
| `environmental_datasets.name` | `open-meteo-era5-daily` |
| Source | Open-Meteo Historical Weather API (`https://archive-api.open-meteo.com/v1/archive`), which serves the ECMWF **ERA5** reanalysis produced by the Copernicus Climate Change Service (C3S) |
| Model pinned | `models=era5` (see "Why `models=era5`") |
| Variables | `precipitation_sum` (mm/day) -> `measurement_type=precipitation`, unit `mm`; `temperature_2m_mean` (degC) -> `temperature_mean`, unit `degC` |
| Temporal resolution | daily, UTC days (`timezone=UTC`); coverage 1940-present; the archive lags real time by about 5 days (the code never requests the last 6 days) |
| Spatial resolution | ERA5 grid, **0.25 deg (about 25 km)**. The API answers with the single nearest grid cell; for Kastamonu (41.38N, 33.78E) that is **41.50N, 33.75E**, DEM elevation reported 778 m. This is one grid cell, **not** a province/region average and **not** station data. ERA5-Land (0.1 deg, about 11 km) exists on the API but did not return precipitation in our test (below), so it is not used |
| License | Open-Meteo API data: **CC BY 4.0**. The free API is for **non-commercial use only** (see "Use restrictions") |
| Attribution (required) | "Weather data by Open-Meteo.com" (link https://open-meteo.com/), link to the licence, indicate changes. Copernicus acknowledgement given by Open-Meteo for reanalysis: "Generated using Copernicus Climate Change Service information". Our UI/API therefore returns the licence in `data_sources[].license` and the docs/Flutter screens must show the attribution next to any chart built from this data |
| `retrieved_at` | set to the time of the latest download (committed snapshot: **2026-10-20T10:42:46Z**) |

## What was verified, 2026-10-20 (live calls with curl)

All calls: `GET https://archive-api.open-meteo.com/v1/archive?latitude=41.38&longitude=33.78&...&daily=precipitation_sum,temperature_2m_mean&timezone=UTC`

| Call | Result |
|---|---|
| default model, 1991-01-01..1991-01-05 | HTTP 200, grid 41.3708N 33.75E, elevation 778 m, daily_units `mm` / `°C`, 5 values each |
| `models=era5`, 1991-01-01..1991-01-03 | HTTP 200, grid **41.5N 33.75E**; precip `[0, 0, 0.8]` mm, temp `[2.8, 4.5, 2.5]` degC |
| `models=era5_land`, 1991-01-01..1991-01-03 and 2015-03-01..2015-03-06 | HTTP 200, grid 41.4N 33.8E, temperature present (e.g. `[3.2, 4.2, 1.2]`) but **`precipitation_sum` all `null`** |
| default (`best_match`) | temperature equals the ERA5-Land values (grid 41.37N), precipitation equals the ERA5 values: i.e. **best_match silently mixes two grids** |
| `models=era5`, 2020-01-01..2020-12-31 | HTTP 200, 366 days |
| `models=era5`, **1991-01-01..2020-12-31 in one request** | HTTP 200, **248,930 bytes**: the API accepts a 30-year daily request in one call. We still fetch **one calendar year per request** (about 8 KB) so that a failure loses little and the ledger can resume |
| bulk download for the snapshot | 35 yearly requests 1991..2025, 1 s apart, about 36 s total, no 429; result 12,784 days, 0 missing values in both variables |

Not tested on purpose: provoking a 429 (we do not hammer a free service). 429/5xx handling, `Retry-After`, and quota detection are covered by unit tests against a fake server.

### Why `models=era5`
With the default `best_match`, temperature and precipitation come from **different reanalyses/grids** (ERA5-Land vs ERA5) and the choice is the provider's internal logic, which can change. A water-balance index (P - PET) should use one consistent source, and results should be reproducible, so the model is pinned to `era5` (both variables from the same 0.25 deg grid). `Fetcher.Model` can be changed if the provider adds a consistent higher-resolution precipitation model.

## Terms, as read from the provider's pages (2026-10-20)

Read via page fetch (summarised by a tool, not by a lawyer); re-read before any commercial or large-scale use:
- https://open-meteo.com/en/terms: API data under CC BY 4.0. "You may only use the free API services for non-commercial purposes." Free limits: **600 calls/minute, 5,000/hour, 10,000/day, 300,000/month**, per IP/application, no uptime guarantee. Considered **non-commercial**: private or non-profit sites without subscriptions or ads, public research at public institutions, educational content. Considered **commercial**: apps/websites with subscriptions or ads, commercial products, undisclosed research at commercial entities. Commercial use needs a paid plan (or self-hosting).
- https://open-meteo.com/en/pricing: a request normally counts as one call, but requests for more than 2 weeks of data for one location or more than 10 variables are counted as several (fractional) calls, e.g. 4 weeks = 3 calls. The exact formula is not published, so we budget conservatively: a full 35-year download is 35 yearly requests (the weighted cost was not measured; at 35 requests it is expected to stay well below the 10,000/day limit) and happens **once per region** (the ledger stops repeats).
- https://open-meteo.com/en/licence: attribution requirements quoted above; Copernicus reanalysis acknowledged.
- https://open-meteo.com/en/docs/historical-weather-api: ERA5 0.25 deg hourly 1940-present, 5-day delay; ERA5-Land 0.1 deg 1950-present; `models` options.

## Use restrictions for Eco Terminal

1. **Non-commercial only on the free API.** A public, ad-free, non-profit/research deployment fits the stated non-commercial examples; the operator must decide for their own deployment. A commercial deployment needs a paid Open-Meteo plan or a self-hosted/own ERA5 source.
2. **Attribute** wherever charts/indicators derived from this data are shown.
3. **Politeness**: built-in minimum 1 s between requests, exponential back-off on 5xx/network errors, `Retry-After` honoured on 429, no retry when the reason mentions a daily/hourly/monthly quota (surfaced as `climate.ErrQuota`), `User-Agent: EcoTerminal/1.0`.
4. **Not station data, not official.** ERA5 is a model-based reanalysis. Do not present it as a measurement of the Turkish State Meteorological Service (MGM) or as an official statistic.
5. ERA5 temperatures of a 25 km cell differ from a mountain-town station (cell elevation and mean terrain differ); absolute values (for example the 1991-2020 ERA5 mean temperature of this cell, 11.6 degC) must not be quoted as the city's climate normal. The model uses them only as a consistent baseline for relative comparison.

## Offline snapshot

`datasets/environmental/kastamonu_era5_daily_1991_2025.json` (about 96 KB): a **genuine, one-time download** (not synthetic) of the series above for the Kastamonu city centre request (41.38N, 33.78E) -> ERA5 cell 41.50N 33.75E, 1991-01-01..2025-12-31, 12,784 days, retrieved 2026-10-20T10:42:46Z by:

```
cd backend && go run -tags "sqlite_fts5 sqlite_rtree" ./internal/simulation/cmd/climatesnap \
  -lat 41.38 -lon 33.78 -place "Kastamonu (city centre)" -from 1991-01-01 -to 2025-12-31 \
  -out ../datasets/environmental/kastamonu_era5_daily_1991_2025.json
```
The file carries `source`, `license`, `attribution`, `model`, `request`, `grid`, `retrieved_at`. Sanity statistics of the file (computed 2026-10-20): 1991-2020 mean annual precipitation 603 mm, mean temperature 11.6 degC; the first days (1991-01-01..03) equal the live API values quoted above.

`climate.Store` uses it **before** the network when a region's centroid falls into the same 0.25 deg ERA5 cell (`climate.SameCell`) as the snapshot; regions in other cells are fetched from the API. The default Kastamonu province centroid lies in the same cell as the city centre request only if it rounds to the same grid point; this is checked at runtime, not assumed.

## Storage

`environmental_measurements` rows: `dataset_id` = `open-meteo-era5-daily`, `region_id`, `ts` = `YYYY-MM-DD`, `measurement_type` in {`precipitation`, `temperature_mean`}, `unit` {`mm`, `degC`}, `spatial_resolution` = "ERA5 0.25deg grid cell (41.50N, 33.75E)", `quality_flag` = `ok`. Upserts use the table's `UNIQUE (dataset_id, region_id, ts, measurement_type)`; days the provider reports as `null` get no row. Migration `0020_simulation_climate_fetches.sql` adds the ledger `climate_fetch_ranges` (which date ranges were ingested per region and from where: `api` or `snapshot`), so `EnsureRegionData` fetches only missing ranges and never re-requests provider nulls.
