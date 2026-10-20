# RAG design (`backend/internal/rag`)

Geo-aware hybrid retrieval-augmented answering for `POST /ai/chat`. Works with **no AI key** (FTS5 + deterministic
answer); an OpenAI-compatible text model only rephrases what retrieval and SQL already established.

```
question ─▶ Understand (rules) ─▶ region/period ─▶ ComputeFacts (SQL) ─┐
        └▶ Search (access-filtered FTS5 [+ sqlite-vec], RRF) ──────────┴▶ Answer
                                                   generative (text provider)  | retrieval_only (deterministic)
```

## Tables
`rag_documents` (one row per source record: observation / rss_article / manual|scientific|region_report),
`rag_document_chunks` (`id` AUTOINCREMENT = rowid of both indexes), `rag_chunks_fts` (FTS5, external content,
`unicode61 remove_diacritics 2`), `rag_chunks_vec` (sqlite-vec `vec0 ... distance_metric=cosine`, created at runtime),
`embedding_index_metadata` (model + dimension of the current vec table). No schema changes were needed.

## Indexing (`index.go`, `chunk.go`)
| Source | visibility | owner_user_id | Notes |
|---|---|---|---|
| observation `verified` / `resolved` | `public` | - | **public coordinates only** (`public_lat/lon`) |
| observation `submitted` / `ai_processed` / `under_review` / `needs_information` | `staff` | owner | owner sees own, reviewer/admin see all |
| observation `draft` / `rejected` / soft-deleted / missing | **removed from index** | | |
| RSS article (source enabled) | `public` | - | removed when the source is disabled |
| manual/scientific/region_report | `public` | - | `IndexManualDocument` |

- `is_sensitive` observations: place names and `region_id` are generalized to district/province (no village), only
  public coordinates are stored, and species are never broken down in SQL facts (totals still count them).
- Document text is a Turkish template (title, type, category in tr+en, species [verified vs "kullanıcı tahmini"], description,
  place, date, status, demo flag). Demo rows say so in the text.
- Chunker: deterministic, sentence-aware (abbreviations, initials, numbers/dates don't split), ~800 chars, ~120 chars overlap,
  long sentences split at word boundaries. Chunks after the first are prefixed with the title.
- `content_hash` over all indexed fields: unchanged documents are skipped. Changed documents have their chunks **replaced
  wholesale** (delete old chunks -> `trg_chunks_ad` removes FTS rows -> insert new -> `trg_chunks_ai` adds FTS rows), which is why
  the external-content FTS5 needs no UPDATE trigger. `ReindexAll(force)` also runs FTS5 `'rebuild'`.
- Embeddings are computed **before** the write transaction (no lock held during network I/O). If embedding fails the document is
  still indexed lexically; `EmbedBackfill` (job `embed_backfill`) fills missing vectors later. If the configured model or
  dimension differs from `embedding_index_metadata`, `EnsureVectorIndex` drops/recreates `rag_chunks_vec` (vectors then re-created by backfill).

## Retrieval and the access boundary (`retrieve.go`)
Both candidate queries embed the same filter, evaluated **before ranking and LIMIT**:

```
visibility = 'public'
OR (visibility = 'verified_only' AND caller is signed in)
OR (visibility = 'staff'  AND (caller is reviewer/admin OR owner_user_id = caller))
OR (visibility = 'owner'  AND owner_user_id = caller)
AND region ∈ subtree(region_id) [documents.region_id OR observation_region_links OR rss_article_regions]
AND (doc_date IS NULL OR date(doc_date) BETWEEN from AND to)
```
- Lexical: FTS5 `MATCH` + `bm25()` joined to chunks and documents with the filter in `WHERE`.
- Vector: `SELECT rowid FROM rag_chunks_vec WHERE embedding MATCH ? AND k = ? AND rowid IN (SELECT id FROM cand)` where `cand` is the
  filtered chunk set - KNN is constrained to allowed chunks, it is not post-filtered.
- Fusion: reciprocal rank fusion (`k=60`), at most 2 chunks per document, then **hydration re-applies the access filter**
  (defence in depth). A restricted chunk is therefore never returned, regardless of query text; the query text only reaches
  FTS5 as double-quoted terms (no operators) and as an embedding.
- Turkish query handling: Turkish lower-casing, stopwords, per-term OR group with `ı/i` variants (unicode61 folds `İ/I` to `i` but keeps `ı`)
  and a suffix-stripped **prefix** variant (`ayıların` -> `ayı*`). Diacritics (`ç ğ ö ş ü`) are folded by the tokenizer.
- Vector search is optional: no embedding provider, query-embedding error or vec error => lexical only (a limitation is reported).

## Question understanding (`understand.go`) and facts (`facts.go`)
Rule-based (Turkish + English): aggregate intent (`kaç`, `sayısı`, `toplam`, `how many`...), trend (`aylık`, `artış`...), top species
(`en çok`), "here" references (`burada`, `bu bölgede`, `here`), periods (`son iki yılda`, `son 30 gün`, `geçen yıl`, `bu ay`, `2024`),
category words, leftover subject terms (candidate species names). A place named in the question (`Tosya'da`) resolves to a region; an explicit
`region_id` wins. "Here" without any region => `needs_region=true` and no counting/search.

Facts are computed by SQL over **verified/resolved, non-deleted** observations in the region subtree and period (`observed_at`):
total, distinct observers, demo count, category breakdown, top species / species-term matches (sensitive excluded), monthly series (trend).
Staff additionally get the pending-records count as a *separate* fact. Every answer with facts carries limitations: unverified excluded,
"observation density != population change", period used, region used, data gaps (empty months), and "no records != absence".
A subject term that matches no verified species name does **not** filter the total (it is reported instead) so the answer can never silently be 0.

## Answer assembly (`answer.go`, `prompt.go`)
- `sources[]`: retrieved chunks numbered `1..k` (type observation / rss_article / scientific; `manual` maps to `scientific`), then each SQL
  fact as a `sql_fact` source so facts are citable too. `id` is the plain number (`"1"`), the text uses `[1]`.
- **generative** (text provider configured and there is something to cite): static system prompt (Turkish rules; never contains user/retrieved text),
  user message with `[SORU]`, `[VERİLER]` (SQL facts), `[KAYNAKLAR]` (each source inside `<<<SOURCE n | type | title | date>>> ... <<<END SOURCE n>>>`),
  `[GEÇMİŞ]`. Untrusted text is stripped of control characters, `<<<`/`>>>` runs and our section headers, and capped (1200 chars per chunk).
  History turns (including client-supplied "assistant" turns) are treated as untrusted data, never as chat roles.
- Post-checks: `ValidateCitations` removes `[n]` markers that do not match a source (reported as a limitation);
  `UngroundedNumbers` flags numbers in the text that appear in no fact/source/question (reported as a limitation).
  Generation failure falls back to retrieval-only with a limitation.
- **retrieval_only**: deterministic Turkish/English summary: facts (with `[n]`) then top sources with snippets.

## Prompt-injection posture
RSS items and user observation text are untrusted. They (1) never enter the system role, (2) are delimited and sanitized,
(3) cannot influence retrieval filters (filters are built from the authenticated caller + request fields only), and (4) cannot change numbers
(numbers come from SQL; ungrounded numbers are flagged). Tests: `TestPromptKeepsMaliciousChunkInsideDataBlock`,
`TestAccessFilterCannotBeOverriddenByQueryText`, `TestInjectedDocumentCannotWidenRetrieval`.

## Public API for wiring (phase 2)
`rag.New(db, cfg, providers)`; `Answer(ctx, ChatRequest, Caller) (ChatAnswer, error)` (errors: `ErrEmptyQuestion`, `ErrQuestionTooLong`, `ErrRegionNotFound` -> 400/404);
`IndexObservation`, `IndexArticle`, `IndexManualDocument`, `RemoveDocument`, `ReindexAll`, `EmbedBackfill`, `EnsureVectorIndex`, `Search`.
`Caller{UserID, Role}` must come from the authenticated session, never from the request body.

## HTTP and process wiring

`internal/app/intelligence_module.go` assembles providers, RAG, RSS and the durable jobs worker.
Authenticated `/ai/providers/status`, `/ai/chat` and `/ai/analyze-observation` use the shared session middleware.
The RAG caller is built only from `auth.UserFrom`; client-supplied role/id fields are never trusted.
Chat works without provider configuration through `retrieval_only`, SQL facts and cited FTS sources.
History is limited to 20 user/assistant turns of 2000 characters; explicit dates are validated and normalized to UTC calendar days.
Photo analysis requires owner/reviewer/admin and configured vision, then atomically queues a pending analysis and an audit entry.
The worker writes suggestions and only advances submitted records to ai_processed, with history and audit in the same transaction.

Public `/feeds` and `/feeds/articles` hide disabled sources. Admin endpoints add a URL only after SSRF-guarded RSS/Atom verification,
or queue an asynchronous sync (empty body means all enabled sources). Feed creation and sync requests are audited atomically.
At startup only the committed verified source list is seeded into an empty table; jobs index missing legacy/demo documents and backfill embeddings.
`ECO_RSS_ENABLED=false` stops periodic fetches; explicit admin sync remains available. All workers stop on context cancellation.
The optional simulation narrator shares the text provider; the simulation service still computes and validates every indicator independently.
