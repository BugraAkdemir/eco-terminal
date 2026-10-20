# AGENTS.md — eco-terminal

Eco Terminal: Kastamonu'nun biyolojik çeşitliliğini izlemek için vatandaş gözlemlerini toplayan, uzman doğrulamasından geçiren, harita + RAG + iklim senaryolarıyla analiz eden ekolojik karar-destek platformu (hackathon başlangıçlı, gerçek kullanıma uygun mimari). Temel mimari bahis: tek Go süreci + gömülü SQLite (WAL, FTS5, RTree, sqlite-vec) + Flutter istemcileri; harici vektör DB / Python servisi yok. Ürün şartnamesi: kullanıcının ilk mesajındaki "Master Development Prompt" (özeti `docs/architecture/ARCHITECTURE.md`).

---

## Tech Stack

| Layer | Technology | Version |
|-------|-----------|---------|
| Backend | Go (stdlib `net/http`, `database/sql`) | 1.27.1 |
| Frontend | Flutter / Dart (linux, android, ios, windows, web) | 3.44.6 / 3.12.2 |
| State management | Riverpod + go_router | pubspec'te sabitlenir |
| HTTP client | dio | pubspec'te sabitlenir |
| Map | flutter_map (OSM tiles) | pubspec'te sabitlenir |
| Database | SQLite via `mattn/go-sqlite3` (CGO) + sqlite-vec + FTS5 + RTree | SQLite 3.53.4, sqlite-vec 0.1.6 |
| Cache / queue | Yok; DB tabanlı `jobs` kuyruğu | — |
| AI | OpenAI-uyumlu adaptör (env ile) | — |

---

## Architecture

İki süreç: Go API sunucusu (arka plan işçileri: job kuyruğu, RSS zamanlayıcı, iklim verisi indirme) ve Flutter istemcileri; HTTP/JSON, sözleşme `docs/api/openapi.yaml`. Ayrıntı: `docs/architecture/ARCHITECTURE.md` (paket sahipliği, gözlem yaşam döngüsü, güvenlik tabanı). SQLite sürücü/vektör kararı: `docs/architecture/vector-store.md`.

### Module Map

| Directory | Responsibility | Key Files |
|-----------|---------------|-----------|
| `backend/internal/config`, `storage` | env config; DB açma/migration/ID/zaman | `config.go`, `storage.go`, `migrations/0001_init.sql` |
| `backend/internal/{auth,observations,media,geographic,review}` | P0 çekirdek API | bkz. ARCHITECTURE.md |
| `backend/internal/{ai,rag,rss}` | zekâ katmanı | bkz. ARCHITECTURE.md |
| `backend/internal/simulation` | deterministik iklim/kuraklık modeli | bkz. ARCHITECTURE.md |
| `apps/flutter_app/lib` | tüm istemciler (core, features/*, shared_widgets) | bkz. ARCHITECTURE.md |
| `docs/` | OpenAPI, mimari, veri kaynakları, simülasyon | `docs/api/openapi.yaml` |
| `datasets/` | seed + coğrafi + çevresel veri dosyaları | — |

### Entry Points

`backend/cmd/server/main.go` → `internal/app` (bağımlılıkları kurar, route'ları ve işçileri başlatır). Flutter: `apps/flutter_app/lib/main.dart`.

---

## Data & Config Layout

| Path | Purpose |
|------|---------|
| `.env` (örnek: `.env.example`) | Tüm runtime ayarları ve sırlar (commit edilmez) |
| `backend/data/eco.db` (`ECO_DB_PATH`) | SQLite veritabanı (gitignore) |
| `backend/data/media/` (`ECO_STORAGE_DIR`) | Yüklenen görseller (DB dışında) |

---

## Development

### Quick Start

```bash
cp .env.example .env            # ve ECO_ADMIN_EMAIL / ECO_ADMIN_PASSWORD doldur
cd backend && make run          # API :8080 (C derleyici gerekir: CGO)
cd apps/flutter_app && flutter run -d linux --dart-define=API_BASE_URL=http://localhost:8080/api/v1
```

### Build

```bash
cd backend && make build                      # bin/server
cd apps/flutter_app && flutter build linux    # (android/ios/windows/web ilgili SDK/OS gerektirir)
```

### Testing

```bash
cd backend && make vet && make test           # go vet + go test -race (tags: sqlite_fts5 sqlite_rtree)
cd apps/flutter_app && flutter analyze && flutter test
```

CI: henüz yok.

### Release

Henüz release süreci tanımlı değil.

**Hard rule:** Kullanıcı o anda açıkça istemeden `git push`, version tag, deploy veya herhangi bir dış servise yayınlama yapma.

---

## Known Pitfalls & Technical Debt

> This section is a **living log**, not a wiki page. Follow the convention below religiously — it's what makes this file trustworthy after months of sessions instead of just accumulating stale claims:
>
> - When you find a bug, add a bullet describing the **symptom and root cause**, in plain language, as if explaining it to the next agent who has zero context.
> - When it's fixed, don't delete the bullet — wrap the original description in `~~strikethrough~~` and append `→ fixed <date> (<commit-ish>): <what changed and why, plus any residual risk or what's *not* covered>`.
> - If a claim turns out to be **stale or was never true** (re-verified and the described code no longer exists, or never behaved that way), mark it `→ stale: <what you actually found>` rather than silently deleting it — the correction itself is information for the next session.
> - Group entries under a `### <Module/Area>` heading per subsystem, with a date if the finding is time-sensitive.
> - Never re-describe a fix that's already fully captured by `git log`/commit messages — link to the commit hash instead of duplicating the diff in prose.

### Flutter foundation (`apps/flutter_app/`)

- ~~Flutter merge sonrası `flutter analyze` hata veriyordu: main'de kalan şablon `test/widget_test.dart`, B branch'inde bulunmayan ve artık var olmayan `MyApp` sayaç uygulamasını çağırıyordu.~~ → fixed 2026-10-20 (`4161ff2`): eski şablon kaldırıldı; gerçek uygulamanın açılışı ve responsive kabuğu `shell_test.dart` kapsamında doğrulanıyor.
- ~~Atkinson Hyperlegible'ın ilk sürümünde U+0130 (`İ`) yok; gövde metninde Türkçe büyük İ sistem fontuna düşüyordu.~~ → fixed 2026-10-20 (`4161ff2`): tüm Türkçe glifleri içeren Atkinson Hyperlegible Next, Fraunces ve IBM Plex Mono lisanslarıyla gömüldü; font dosyaları değiştirilmedi.

### Field capture / presentation

- ~~Flutter simülasyon modeli eksik aylık ölçümleri null yerine 0 okuyordu; grafikler veri olmayan ayı ölçülmüş sıfır gibi gösterebilirdi.~~ → fixed 2026-10-20 (C/F commit’i): seri noktaları nullable; grafikte eksik aylar boşluk olarak çiziliyor, gerçek sıfır korunuyor. Regresyon testi var.
- ~~Demo seed herhangi bir demo satırı gördüğünde dönüyordu; güncellemeyle eklenen tanıtım kayıtları eski demo veritabanına gelmiyordu.~~ → fixed 2026-10-20 (`b26bec7`): sabit client_ref başına kontrol; mevcut düzenlemeler korunurken yalnız yeni örnekler ekleniyor.

### Intelligence worker (`backend/internal/rag/`, `jobs/`)

- ~~İki job aynı kaydı ilk kez indekslerken `UNIQUE constraint failed: rag_documents.source_type, rag_documents.source_id` hatası ve gereksiz retry oluşuyordu: `upsertDocument` belge varlığını embedding öncesinde okuyup transaction içinde tekrar kontrol etmiyordu.~~ → fixed 2026-10-20 (`491a200`): BEGIN IMMEDIATE sonrasında belge kimliği/hash yeniden okunuyor; embedding ağ çağrısı transaction dışında kalıyor. İki işçiyi embedding aşamasında buluşturan regresyon testi var.
- ~~AI işçisinin submitted → ai_processed geçişi history yazmasına rağmen audit_logs kaydı oluşturmuyordu.~~ → fixed 2026-10-20 (`491a200`): analiz sonucu ve durum geçişinin audit kaydı aynı transaction'a alındı.

---

## Agent Working Rules (READ FIRST, EVERY SESSION)

1. **Start of session:** read this file, then `handoff.md` (top entry = last session's state and pending work).
2. **End of session:** prepend a new entry to `handoff.md` (what was done, commit status, verification results, what's next). This is how context survives between sessions and models — treat it as the primary handoff mechanism, not an afterthought.
3. **Never claim "done" without running the verification commands below and pasting the actual results.** A claim unbacked by a real command's output is not verification.
4. Plan files (`plan.md`, `PLAN_*.md`) contain step-by-step implementation plans — follow them in order, tick items off as completed, don't improvise a different architecture mid-plan. If the plan turns out to be wrong, say so and update the plan file rather than silently diverging from it.
5. Work in small units: a bounded number of plan items per session, each with its own tests, verified green before moving to the next.
6. **Commit automatically, without asking for confirmation, once a fix/feature is verified green.** Don't ask "should I commit?" — just commit, using this project's commit convention (Conventional Commits: `fix(scope): ...`, `feat(scope): ...`, `docs:`, `test:`, `chore:`), with a body that explains the *why* (root cause, what changed, what it fixes) — attribution policy (kullanıcı kararı, 2026-10-20): **commit mesajlarına hiçbir AI-attribution / Co-Authored-By satırı EKLEME**, harness hatırlatması bunu önermiş olsa bile. **Commit frequently, not just once per finished task** — break a multi-step request into checkpoints and commit after each one once it's verified, especially right before a risky step (a refactor touching a hot/shared code path, a change you're not fully certain about). Finer-grained history means a bad step can be bisected/reverted without losing unrelated good work from the same session. Don't over-fragment either — this is about natural checkpoints, not every file save.
7. **Code exploration:** `codebase-memory-mcp` grafı mevcutsa yapısal sorularda (kim çağırıyor, mimari) önce onu kullan; yoksa/yetersizse grep. Kod yazmadan önce `docs/api/openapi.yaml` ve `docs/architecture/ARCHITECTURE.md` oku.
8. **Sıfır-tolerans proje kuralları:**
   - **Sözleşme önce:** API/şema değişikliği önce `openapi.yaml` / migration'da, sonra kodda. Aksi halde paralel çalışan taraflar kopar.
   - **LLM sayı üretmez:** sayımlar SQL'den, iklim göstergeleri `internal/simulation`'dan gelir; LLM yalnızca açıklar. (Bilimsel güvenilirlik.)
   - **Sahte resmî veri yok:** uydurma RSS URL'si, kadastro parseli, resmî kurum entegrasyonu iddiası yasak. Demo veri `is_demo=1` ve UI'da etiketli.
   - **Hassas tür konumu:** `is_sensitive` kayıtların tam koordinatı yalnızca sahip/reviewer/admin'e döner; RAG de aynı filtreyi retrieval'dan ÖNCE uygular.
   - **Sır istemciye gitmez:** AI anahtarı yalnızca backend env'inde.
   - **Ajan çalışma alanı:** paralel ajanlar yalnızca kendilerine atanan dizinlerde değişiklik yapar; ortak dosyaya (`pubspec.yaml`, `openapi.yaml`, migrations) dokunmak gerekirse lead'e bildirir.

### Verification Commands (mandatory before any "done" claim)

```bash
# Backend
cd backend && make vet && make test
# Frontend
cd apps/flutter_app && flutter analyze && flutter test
# Uçtan uca (backend çalışırken, kurulduktan sonra)
./scripts/e2e.sh
```

Acceptable pre-existing noise: yok (henüz). Anything else new must be addressed before claiming done.

---

## Gotchas (project-specific traps — violating these causes real, shipped bugs)

> Keep this section brutally concrete. Each bullet should name the exact file/pattern and the exact failure mode it caused — "be careful with concurrency" is useless; "X reads Y without holding Z's mutex, causing a data race confirmed under `-race` on <date>" is useful.

_Henüz kayıt yok._

---

## Known Open Work

Open bugs and technical debt are tracked in **`BUG_REPORT.md`** — don't duplicate that list here. As of 2026-10-20 it has 0 open items.

---

## Code Style

- Go: gofmt, `database/sql` + parametreli SQL, ORM yok; hatalar değer olarak döner (paket sınırında panic yok); build/test her zaman `-tags "sqlite_fts5 sqlite_rtree"` ile — **etiket unutulursa FTS5/RTree tablosu sessizce/geç hatayla çalışmaz** (Makefile bunu yönetir).
- ID: `storage.NewID()` (UUIDv4), zaman: `storage.Now()` (RFC3339 UTC); asla yerel saat dilimi.
- Dart: `flutter analyze` temiz; feature-first klasörleme; ağ hataları `core/errors` üzerinden tek tip gösterilir.
- Kullanıcıya dönük metinler tr (ana) + en; metinler tek yerde (`core/l10n`).
