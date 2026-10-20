# Eco Terminal — Architecture brief (shared contract for all contributors/agents)

Truth sources: `docs/api/openapi.yaml` (HTTP contract), `backend/internal/storage/migrations/*.sql` (schema).
Change the contract first, then both sides. Never invent endpoints/fields that are not in the OpenAPI file — add them there.

## Topology
Flutter app(s) ──HTTPS/JSON──▶ Go API (`backend/cmd/server`) ──▶ SQLite file (WAL; FTS5, RTree, sqlite-vec) + media dir on disk.
Background workers inside the Go process: job queue (`jobs` table), RSS scheduler, climate-data ingestion. No other services.

## Go backend layout (`backend/`, module `github.com/BugraAkdemir/eco-terminal/backend`)
Build/test always with tags: `go test -tags "sqlite_fts5 sqlite_rtree" -race ./...` (see `backend/Makefile`).
Stdlib `net/http` ServeMux (Go 1.22+ patterns like `GET /api/v1/observations/{id}`), `database/sql`, no ORM, no web framework.

| Package | Owner/purpose |
|---|---|
| `internal/config`, `internal/storage` | DONE (foundation): env config; `storage.Open/Migrate/NewID/Now`. |
| `internal/httpx` | JSON envelope helpers, error codes, middleware (request id, logging, CORS, rate limit, panic recovery). |
| `internal/auth` | users, argon2id, sessions (hashed opaque tokens), role middleware, `/auth/*`, `/admin/users`. |
| `internal/audit` | `audit.Log(ctx, db/tx, entry)` used by all state-changing code. |
| `internal/observations` | CRUD, reference numbers, status state machine, history, notifications hooks, sensitive-location generalization. |
| `internal/media` | upload validation (magic bytes), EXIF/GPS strip by re-encoding, random names, thumbnails, `/media/{id}`. |
| `internal/geographic` | regions (GeoJSON), point-in-polygon, bbox via RTree, region analytics, Kastamonu region seeding. |
| `internal/review` | queue + decisions (uses observations state machine). |
| `internal/ai` | provider interfaces + OpenAI-compatible adapter + status. No secrets in responses. |
| `internal/rag` | chunking, FTS5 + sqlite-vec index, hybrid retrieval, access filter, answer assembly with citations. |
| `internal/rss` | feed fetch/parse/sanitize/dedup, scheduler, region tagging. |
| `internal/simulation` | pure deterministic math (no DB, no LLM) + climate data service + API. |
| `internal/jobs` | durable worker over the `jobs` table. |
| `internal/app` | wiring: builds dependencies, registers routes, starts workers. `cmd/server/main.go` only calls it. |

Rules: handlers thin; business logic in services with interfaces; every state change writes `audit_logs`;
all IDs `storage.NewID()`, all timestamps `storage.Now()`; SQL parameterized only; timestamps UTC RFC3339.
Tests next to code (`*_test.go`), using `storage.OpenTemp(t.TempDir())` and `httptest`.

## Observation lifecycle (enforced in `internal/observations`)
Statuses: draft, submitted, ai_processed, under_review, verified, needs_information, rejected, resolved.
Allowed transitions (actor):
- draft → submitted (owner)
- submitted → ai_processed (system/AI) ; submitted|ai_processed → under_review (reviewer `start_review`)
- submitted|ai_processed|under_review → verified | rejected | needs_information (reviewer)
- needs_information → under_review (owner submits answer via PATCH `submit:true`)
- verified → resolved (reviewer; mainly for `issue` records)
Anything else → 409 `conflict`. AI never moves a record to verified/rejected. Every transition: row in
`observation_status_history`, `audit_logs` entry, notification to owner (for reviewer-caused transitions).
Visibility: non-staff see only `verified`/`resolved` records of others; owners see all of their own (not soft-deleted).
Sensitive records (`is_sensitive=1`): `public_lat/lon` rounded to 0.1° (~11 km) for non-owner/non-staff.

## Security baseline
argon2id; token = 32 random bytes, only sha256 stored; per-IP rate limit (stricter on /auth/*); request body limits;
CORS from config; no secrets in logs; AI keys server-side only; RAG retrieval filters by `rag_documents.visibility`
and caller role BEFORE ranking; external text (RSS, user text) is untrusted data inside prompts (delimited, never instructions).

## Climate/data honesty rules
LLM never produces numbers (counts come from SQL, indicators from `internal/simulation`).
Declining citizen sightings never imply declining populations (sampling bias warnings in analytics).
Demo/seed rows carry `is_demo=1` and the UI labels them. No fabricated official data (feeds, parcels).

## Flutter app layout (`apps/flutter_app/lib/`)
`core/` (theme/tokens, api client, config, errors, router, l10n strings tr+en), `features/<name>/{data,domain,presentation}`
for: authentication, maps, observations, intelligence, ai_assistant, official_feeds, simulations, expert_dashboard,
user_profile; `shared_widgets/`. State: Riverpod; routing: go_router; HTTP: dio. Map: flutter_map.
Responsive breakpoints: <700 mobile (bottom nav), 700–1100 tablet, >1100 desktop (resizable left panel + map).
Default API base URL `http://localhost:8080/api/v1`, overridable via `--dart-define=API_BASE_URL=...`.
Design tokens (spec §18): bg #07131E, surface #0D2232, sidebar #092B45, green #36C879, teal #22B8A8, warning #F1AA4C, critical #E66B68, text #F2F7F7 / #A9BCC5.
