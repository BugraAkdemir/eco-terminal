# Region boundaries (Kastamonu province + 20 districts)

**Real administrative boundaries, not generated.** Downloaded and verified on **2026-10-20**.

| Item | Value |
|---|---|
| Provider | geoBoundaries (William & Mary geoLab) `gbOpen` release, itself derived from OpenStreetMap |
| Province | `TUR` `ADM1`, feature `Kastamonu` (ISO TR-37) |
| Districts | `TUR` `ADM2`, the 20 Kastamonu districts selected by name + bounding-box check (Merkez, Abana, Ağlı, Araç, Azdavay, Bozkurt, Cide, Çatalzeytin, Daday, Devrekani, Doğanyurt, Hanönü, İhsangazi, İnebolu, Küre, Pınarbaşı, Seydiler, Şenpazar, Taşköprü, Tosya) |
| API metadata | `https://www.geoboundaries.org/api/current/gbOpen/TUR/ADM1/` and `.../TUR/ADM2/` (build date 2023-12-12; ADM2 source data update 2023-01-19) |
| Downloaded files | `https://github.com/wmgeolab/geoBoundaries/raw/9469f09/releaseData/gbOpen/TUR/ADM1/geoBoundaries-TUR-ADM1.geojson` and `https://github.com/wmgeolab/geoBoundaries/raw/9469f09/releaseData/gbOpen/TUR/ADM2/geoBoundaries-TUR-ADM2.geojson` (full-resolution, 10 MB / 16 MB, not committed) |
| License | ADM2: Open Data Commons Open Database License 1.0 (ODbL), (c) OpenStreetMap contributors, https://www.openstreetmap.org/copyright. ADM1: listed by the API as CC BY-SA 2.0 (OSM-derived). Attribution is required: "(c) OpenStreetMap contributors, via geoBoundaries". |
| Retrieved | 2026-10-20 |

## Processing

`datasets/geographical/build_kastamonu_regions.py` (deterministic, stdlib only) selects the 21 features, simplifies every ring with
Douglas-Peucker at 0.0004 degrees (about 40 m) and rounds coordinates to 5 decimals. Result:
`datasets/geographical/kastamonu_regions.geojson` (82 KB, sha256 `e6b67282...e8c27`). A byte-identical copy is embedded into the
binary at `backend/internal/geographic/seed/kastamonu_regions.geojson` (go:embed cannot reach outside the Go module; a test fails if
the two copies diverge).

At first start (empty `geographic_regions`) the server inserts the province (`kind=province`) and the districts (`kind=district`,
`parent_id` = province). Each row stores
`source = "geoBoundaries gbOpen TUR ADM1/ADM2 v. 2023-12-12 (OpenStreetMap contributors, ODbL 1.0), simplified ~40 m; retrieved 2026-10-20"`
and `is_demo = 0`.

## Caveats (honest limits)

- Boundaries are 2023 OSM-derived data, simplified to ~40 m; they are fine for district-level counting but are NOT cadastral or legal
  boundaries. Points within tens of metres of a border may be assigned to the neighbouring district.
- Village (`köy/mahalle`) polygons are not seeded; `kind=village` is supported by the schema but no source was verified.
- No hex-grid fallback was needed (the download succeeded). Researchers/admins can add `custom` analysis polygons via `POST /regions`.
- Observation-to-region links use the exact point for official regions, but only the generalized public point for custom regions,
  so a small custom polygon cannot be used to locate sensitive species.
