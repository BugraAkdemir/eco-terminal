# DATABASE.md

SQLite (WAL, foreign keys ON, busy_timeout 5s, `BEGIN IMMEDIATE` writers) via `mattn/go-sqlite3` with FTS5, RTree and sqlite-vec.
Schema source of truth: `backend/internal/storage/migrations/*.sql` (embedded in the binary, applied at startup, recorded in `schema_migrations`).
Conventions: ids are UUIDv4 TEXT; timestamps RFC3339 UTC TEXT; booleans 0/1; JSON in `*_json` columns.

## Table groups
| Group | Tables |
|---|---|
| Identity | `roles`, `users`, `user_profiles`, `sessions` (only sha256 of the token is stored) |
| Geography | `geographic_regions` (+ `regions_rtree`, kept in sync by triggers), `observation_region_links` |
| Observations | `observations` (+ `observations_rtree` over **public** coordinates), `observation_media`, `observation_ai_analysis`, `observation_reviews`, `observation_status_history` |
| Official feeds | `rss_sources`, `rss_articles` (source URL, published_at and ingested_at stored separately), `rss_article_regions` |
| Environment | `environmental_datasets`, `environmental_measurements` |
| RAG | `rag_documents` (visibility + owner for access control), `rag_document_chunks`, `rag_chunks_fts` (FTS5, external content), `embedding_index_metadata`, `rag_chunks_vec` (sqlite-vec `vec0`, **created at runtime** with the embedding dimension) |
| Async work | `jobs` (durable queue: AI analysis, indexing, feed sync) |
| Simulation | `simulation_scenarios`, `simulation_results` |
| Platform | `notifications`, `audit_logs` |

## Indexes worth knowing
- Spatial: RTree on region bboxes and on observation public coordinates (bbox queries), then exact point-in-polygon in Go.
- `observations(status, created_at)`, `(user_id, created_at)`, `(category, observed_at)`, `(primary_region_id)`.
- `environmental_measurements(region_id, measurement_type, ts)` and a UNIQUE key making ingestion idempotent.
- `rag_chunks_fts` for BM25; `rag_chunks_vec` for KNN (rowid == `rag_document_chunks.id`).

## Privacy-relevant columns
`observations.lat/lon` are exact (owner/reviewer/admin only); `public_lat/public_lon` are what everybody else gets
(rounded to 0.1° when `is_sensitive`). Photos live on disk (`ECO_STORAGE_DIR`), EXIF stripped; only metadata is in `observation_media`.

## Migrations
Add `NNNN_description.sql` (lexicographic order). Never edit an applied migration; add a new one. The runner executes each file in a transaction.
