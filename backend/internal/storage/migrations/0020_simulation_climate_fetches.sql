-- Agent E1 (simulation): ledger of the date ranges already ingested per (dataset, region), so that
-- climate.Store.EnsureRegionData fetches only what is missing and never re-requests days the
-- source reports as null. Additive; does not touch any 0001 table.
CREATE TABLE climate_fetch_ranges (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    dataset_id TEXT NOT NULL REFERENCES environmental_datasets(id) ON DELETE CASCADE,
    region_id  TEXT NOT NULL REFERENCES geographic_regions(id) ON DELETE CASCADE,
    from_date  TEXT NOT NULL,                 -- YYYY-MM-DD inclusive
    to_date    TEXT NOT NULL,                 -- YYYY-MM-DD inclusive
    source     TEXT NOT NULL DEFAULT 'api' CHECK (source IN ('api','snapshot')),
    grid_lat   REAL,                          -- grid cell the provider answered with
    grid_lon   REAL,
    fetched_at TEXT NOT NULL,
    UNIQUE (dataset_id, region_id, from_date, to_date)
);
CREATE INDEX idx_climate_fetch_region ON climate_fetch_ranges(dataset_id, region_id, from_date);
