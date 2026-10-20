# httpx / auth / app: how to wire a handler

Everything lives under `/api/v1`; responses use the envelope from `docs/api/openapi.yaml`.
Handlers are plain `http.HandlerFunc`s registered on the shared Go 1.22 `ServeMux`.

## Plugging in a module (ai, rag, rss, simulation, ...)

Implement `app.Module` (and optionally `app.Worker`) and add ONE line in
`backend/internal/app/extensions.go`:

```go
type Module interface { Register(mux *http.ServeMux, deps app.Deps) }
type Worker interface { Start(ctx context.Context) error; Stop() } // Start must not block

mods = append(mods, ai.NewModule(d))          // a Module that is also a Worker is started/stopped automatically
workers = append(workers, jobs.NewWorker(d))  // pure background worker
```

`app.Deps` gives you `DB *sql.DB`, `Config config.Config`, `Logger *slog.Logger`, `Auth *auth.Service`,
`Geo *geographic.Service`, `Observations *observations.Service`, `Require(roles...)`, `Strict` (rate limiter).

## Writing a handler

```go
func (m *module) Register(mux *http.ServeMux, d app.Deps) {
    // public (optional auth): auth.UserFrom works because the whole mux is wrapped by the auth middleware
    mux.HandleFunc("GET /api/v1/feeds", m.list)
    // any logged-in user / specific roles (admin always passes; reviewer and researcher are siblings)
    mux.Handle("POST /api/v1/ai/chat", auth.RequireFunc(m.chat))
    mux.Handle("POST /api/v1/simulations/run", auth.RequireFunc(m.run, auth.RoleResearcher))
    mux.Handle("GET /api/v1/admin/feeds", d.Require(auth.RoleAdmin)(http.HandlerFunc(m.adminList)))
}

func (m *module) list(w http.ResponseWriter, r *http.Request) {
    viewer, loggedIn := auth.UserFrom(r.Context())      // *auth.User{ID, Email, Role, Status, ...}
    page, err := httpx.ParsePage(r)                      // ?page=&page_size= (default 50, max 200)
    if err != nil { httpx.WriteError(w, err); return }
    var in struct{ Message string `json:"message"` }
    if err := httpx.DecodeJSON(w, r, &in); err != nil { httpx.WriteError(w, err); return } // 1 MiB cap
    httpx.WriteJSONMeta(w, 200, items, page.Meta(total)) // {"data":..., "meta":{page,page_size,total}}
    httpx.WriteJSON(w, 201, obj)                         // {"data": obj}
    httpx.NoContent(w)                                   // 204
    httpx.WriteError(w, httpx.NotFound("..."))           // {"error":{"code","message","details"}}
}
```

Error constructors: `Validation(msg, details)` 400, `Unauthorized` 401, `Forbidden` 403, `NotFound` 404,
`Conflict` 409, `TooLarge` 413, `UnsupportedMedia` 415, `RateLimited` 429, `AINotConfigured` 503,
`Internal` 500. Any non-`*httpx.Error` passed to `WriteError` is logged and returned as 500 `internal`
(its text is never sent to clients). Query helpers: `QueryBool`, `ParseTimeParam` (RFC3339 or YYYY-MM-DD).

## Roles and visibility helpers

- `auth.RoleCitizen|RoleResearcher|RoleReviewer|RoleAdmin`; `auth.HasRole(u, roles...)`; `auth.IsStaff(u)` (reviewer or admin).
- `observations.CanView(viewer, ownerID, status)` / `observations.Privileged(viewer, ownerID)`: use these (do not
  reimplement) whenever you expose observation-derived data (RAG retrieval must filter with the same rules BEFORE ranking).
  Public coordinates are `public_lat/public_lon`; exact `lat/lon` only for owner/reviewer/admin.
- Audit every state change inside the same transaction: `audit.Log(ctx, tx, audit.Entry{Actor: auth.ActorFrom(r), Action: "feed.sync", EntityType: "feed", EntityID: id, After: ...})`.
- IDs `storage.NewID()`, timestamps `storage.Now()`.

## Jobs queued by the core (you consume them)

- `analyze_observation` payload `{"observation_id": id}` (only when AI is configured; created 20 s after the
  observation, and again after media uploads when none is queued). Move a record `submitted -> ai_processed` with
  `observations.Service.Transition(ctx, tx, TransitionRequest{ObservationID, To: "ai_processed", Kind: observations.ActorAI})`.
  The state machine refuses any other AI-caused transition.
- `index_document` payload `{"source_type":"observation","source_id": id}` on every create / edit / status change /
  media change / delete / sensitivity change. Re-read the record and (re)index it or remove it from the index when it
  is not `verified`/`resolved` any more (or was deleted).

## Middleware order (internal/app)

`RequestID -> AccessLog -> Recover -> CORS -> general rate limit (not for GET /media/*) -> body limit -> optional auth -> JSON 404/405 fallback -> mux`.
`/auth/register` and `/auth/login` additionally use the stricter per-IP limiter. The client IP is the TCP peer
address (forwarding headers are NOT trusted).
