# plan.md — Eco Terminal build plan

Living plan (AGENTS.md rule 4: follow in order, tick items, update this file if the plan turns out wrong).
Scope agreed with the owner: **P0 + P1 + one simulation**, plus **offline capture queue** (added 2026-10-20).
AI = OpenAI-compatible adapter via env (must degrade gracefully with no key). Commits carry **no AI attribution**.

Toolchain: Go 1.27.1, Flutter 3.44.6 / Dart 3.12.2, gcc. Verifiable here: backend + Linux desktop. NOT verifiable: Android, iOS, Windows, macOS, web (no SDK/OS/Chrome) — reported honestly, never claimed.

## Key decisions
- Monorepo: `backend/`, `apps/flutter_app/`, `docs/`, `datasets/`, `scripts/`, `tests/`.
- SQLite via `mattn/go-sqlite3` (CGO) + sqlite-vec + FTS5 + RTree (ncruces WASM route failed on Go 1.27; see `docs/architecture/vector-store.md`). Build tags `sqlite_fts5 sqlite_rtree` are mandatory (`backend/Makefile`).
- Hybrid retrieval works without an embedding key: FTS5 fallback; vectors when an EmbeddingProvider is configured. SQL does all counting; the LLM never produces numbers.
- Map: `flutter_map` + OSM tiles (MapLibre GL plugins lack Linux/Windows desktop support). Overlays (observations, regions) come from our own API.
- Flutter: Riverpod + go_router + dio. Auth: argon2id + opaque hashed session tokens. API: stdlib `net/http`, `/api/v1`, uniform envelope, OpenAPI = source of truth.
- Media: magic-byte validation, EXIF/GPS stripped by re-encoding, random filenames, disk storage. Sensitive species: public coordinates generalized.
- Regions: real Kastamonu boundaries only from a verified open source, else generated grid (stated honestly). No fake cadastre.
- Climate: Open-Meteo Historical API (ERA5-based, keyless) → deterministic delta-change drought model (SPI-style + Thornthwaite water balance). "Insufficient data" instead of guessing. LLM only narrates.
- RSS: only feeds actually fetched and verified; honest doc if few/none exist.
- **Offline capture queue:** wizard works without network; drafts + photos stored on device with real capture time/GPS; auto-sync with retry/backoff; idempotent via `client_ref` (unique per user) and media sha256; offline map-tile pack as a follow-up.

## Phases
### Phase 0 — foundation (lead) — DONE
- [x] sqlite-vec / FTS5 / RTree spike, driver decision
- [x] Monorepo scaffold, `config`, `storage` + migration runner, `0001_init.sql` (20+ tables)
- [x] `docs/api/openapi.yaml`, `docs/architecture/ARCHITECTURE.md`, `.env.example`, AGENTS.md filled in
- [x] README, `docs/DATABASE.md`, first commits pushed

### Phase 1 — parallel agents (own git worktrees/branches) — IN PROGRESS
- [ ] **A backend-core:** httpx, auth/roles, audit, observations + state machine, media, geographic (+ real Kastamonu regions), review, notifications, app wiring, `scripts/e2e.sh`, tests; includes `client_ref` idempotency (offline sync)
- [ ] **B flutter-foundation:** design system, API client + models, auth, responsive shell, map, profile, extension points
- [ ] **D1 intelligence services:** AI provider adapters, jobs worker, RAG (hybrid retrieval, citations, access filter), RSS ingest + scheduler
- [ ] **E1 simulation:** Open-Meteo ingestion, climate/drought model, scenario persistence, SIMULATION.md

### Phase 2 — after Phase 1 merge
- [ ] D2: HTTP handlers + wiring for `/ai/*`, `/feeds*`, `/admin/feeds*`
- [ ] E2: HTTP handlers + wiring for `/simulations*`, `/environment/measurements`
- [ ] C: Flutter observation wizard (6 steps) + **offline queue** + expert/review dashboard
- [ ] F: Flutter AI assistant tab, Official Updates tab, Simulation tab (what-if + comparison charts), analytics charts
- [ ] Offline map-tile pack (stretch)

### Phase 3 — integration (lead)
- [ ] Merge all branches, resolve conflicts, `go mod tidy`
- [ ] E2E: observation → review → map → AI query; RSS → RAG → cited answer; region → simulation → comparison
- [ ] Security probes: 401/403 matrix, fake magic bytes, sensitive-location leak, injection chunk
- [ ] Docs: ARCHITECTURE, DATABASE, DATA_SOURCES, SIMULATION, DEMO
- [ ] Final honest report: done / not done / not verifiable

## Deferred (explicitly out of this run unless time allows)
Photo-based species model (BioCLIP adapter slot only), PDF export, advanced notifications, extra simulation models, more data sources.

## Verification (what "done" is backed by)
```
cd backend && make vet && make test
cd apps/flutter_app && flutter analyze && flutter test && flutter build linux
./scripts/e2e.sh
```
