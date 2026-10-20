# handoff.md — how this file works

`handoff.md` is the running session log a coding agent reads at the **start**
of every session and writes to at the **end** of every session. It is what
lets context survive across sessions and across different models — the agent
should never have to reconstruct "what was I doing" from git log alone.

**Rules for this file:**

1. **Newest entry always goes on top.** Prepend, never append — the top of
   the file is always "what happened most recently and what's pending."
2. **One entry per session**, using the exact heading format below. If a
   session has multiple distinct legs (e.g. resumed later the same day),
   either add a new dated entry or clearly mark a sub-section — don't merge
   unrelated work into one undifferentiated blob.
3. **Every entry ends with a "Next Session" section.** This is the single
   most important part of the entry — it's the actual handoff. Be specific:
   name files, name the exact next step, name what was *deliberately left
   out of scope* and why (so it isn't silently forgotten or redone).
4. **Verification results are pasted, not summarized.** "Tests pass" is not
   verification; the actual command and its actual output (or a faithful
   excerpt) is.
5. This file is allowed to grow long. Don't prune old entries — they're the
   permanent record. If it gets unwieldy to read top-to-bottom, that's a sign
   to `grep` for a module/date, not a sign to delete history.
6. Fixed bugs get their permanent record in `AGENTS.md`'s Known Pitfalls
   section (with the `~~struck~~ → fixed` convention) and in git history —
   this file's job is session narrative and handoff state, not a bug
   database. Open bugs live in `BUG_REPORT.md`.

**Entry shape** (copy for each new session, newest on top):

```
# Handoff — <YYYY-MM-DD> (Session <N>) — <one-line title>
## Summary            (2-5 sentences + **Commit status:** hashes / "not committed")
## What Was Done      (sub-tasks; root cause for bugs; files table; "Deliberately not done")
## Verification       (exact commands + pasted output; state what was NOT verified)
## Next Session       (specific, numbered next steps)
```

---

# Handoff — 2026-10-20 (Session 2) — Faz 0 temeli + paralel ajanlar (DEVAM EDİYOR)

## Summary

Kullanıcının "Master Development Prompt"u (tam Eco Terminal) ile geliştirme başladı. Plan onaylandı
(kapsam: P0+P1+tek simülasyon; AI: OpenAI-uyumlu env adaptörü; commit'lerde AI-attribution YOK).
Faz 0 (temel + sözleşmeler) bitti ve commit'lendi; Faz 1'de 4 ajan kendi git worktree'lerinde paralel çalışıyor.

**Commit status:** `1bfe114` (main) — monorepo iskeleti, `0001_init.sql`, `docs/api/openapi.yaml`,
`backend/internal/{config,storage}`, Flutter iskeleti, doldurulmuş `AGENTS.md`. Ajan branch'leri henüz merge edilmedi.

---

## What Was Done

### 1. Faz 0 kararları
- **SQLite sürücüsü:** `mattn/go-sqlite3` (CGO) + sqlite-vec CGO. ncruces WASM yolu denendi, Go 1.27'de ABI uyuşmazlığıyla başarısız
  (bkz. `docs/architecture/vector-store.md`). Spike: vec0 KNN + FTS5 + RTree çalışıyor (SQLite 3.53.4, vec 0.1.6).
  Build etiketleri zorunlu: `-tags "sqlite_fts5 sqlite_rtree"` (`backend/Makefile`).
- **Harita:** `flutter_map` (MapLibre GL eklentileri Linux/Windows masaüstünü desteklemiyor).
- **Ortam:** Go 1.27.1, Flutter 3.44.6. Android SDK ve Chrome YOK → android/web derlemeleri doğrulanamaz; iOS/Windows/macOS bu Linux makinede derlenemez.

### 2. Paralel ajanlar (Faz 1, ayrı worktree'ler)
| Ajan | Sahiplik | Durum |
|------|----------|-------|
| A backend-core | httpx, auth, audit, observations, media, geographic, review, notifications, app, e2e | çalışıyor |
| B flutter-foundation | apps/flutter_app core, tema, API istemcisi, auth, shell, harita, profil | çalışıyor |
| D1 intelligence | ai, rag, rss, jobs (yalnız servis katmanı) | çalışıyor |
| E1 simulation | simulation + Open-Meteo iklim verisi (yalnız servis katmanı) | çalışıyor |

**Deliberately not done:** Flutter gözlem sihirbazı/uzman paneli/AI/Resmî/Simülasyon ekranları (B bitince C/F ajanları),
D1/E1 HTTP handler'ları (A merge olunca faz 2).

---

## Verification

Faz 0: `cd backend && go test -tags "sqlite_fts5 sqlite_rtree" -race ./...` → `ok .../internal/storage 1.019s`; `go vet` temiz;
`sqlite3 < 0001_init.sql` hatasız (98 şema nesnesi); openapi.yaml YAML olarak geçerli (31 path, 25 schema).

---

## Next Session

1. Ajan raporlarını al, branch'leri sırayla merge et (A → D1/E1 → B), `go.mod` çakışmalarını `go mod tidy` ile çöz, tam doğrulamayı çalıştır.
2. Faz 2: D1 ve E1'e HTTP handler + `internal/app` kaydı için devam mesajı; B sonrası C (gözlem sihirbazı + uzman paneli) ve F (AI/Resmî/Simülasyon UI) ajanları.
3. Faz 3: entegrasyon testleri, README/ARCHITECTURE/DATABASE/DATA_SOURCES/SIMULATION/DEMO dokümanları, dürüst "yapılmadı/doğrulanamadı" raporu.

---

# Handoff — 2026-10-20 (Session 1) — Agent şablonlarının kurulumu

## Summary

`setup.md` talimatına göre `*.example.md` şablonlarından `AGENTS.md`,
`handoff.md` ve `BUG_REPORT.md` oluşturuldu, example dosyaları silindi. Proje
teknolojisi henüz belli olmadığı için tüm teknoloji/mimari/komut/proje-özel
bölümler bilerek `TBD` bırakıldı; süreç bölümleri (Agent Working Rules,
Known Pitfalls kuralları, handoff kuralları) şablondan aynen alındı.

**Commit status:** not yet committed — dosyalar working tree'de duruyor.

---

## What Was Done

### 1. Şablonların kurulması

| File | Change |
|------|--------|
| `AGENTS.md` | `AGENTS.example.md`'den oluşturuldu; Tech Stack, Architecture, Module Map, Development, Gotchas, Code Style `TBD` |
| `handoff.md` | `handoff.example.md`'den oluşturuldu; örnek girdiler silindi, bu ilk gerçek girdi eklendi |
| `BUG_REPORT.md` | `BUG_REPORT.example.md`'den oluşturuldu; tüm sayaçlar 0 |
| `*.example.md` (3 dosya) | Silindi |

**Deliberately not done / out of scope:** Teknoloji yığını, komutlar,
commit convention'ı ve AI-attribution (Co-Authored-By) politikası
doldurulmadı — proje koda bağlanınca / sahibi karar verince eklenecek.
`setup.md` silinmedi (talimat dosyası; istenirse silinebilir).

---

## Verification

Kod olmadığı için derleme/test komutu yok; yalnızca dosyaların varlığı
kontrol edildi (bkz. oturum çıktısı).

---

## Next Session

1. Proje teknolojisi belli olunca `AGENTS.md`'deki `TBD` bölümlerini doldur:
   Tech Stack, Architecture/Module Map, Development komutları, Verification Commands.
2. Rule 6'daki commit convention'ı ve attribution politikasını kullanıcıyla netleştir.
3. İlk gerçek kod/bug bulgularını `AGENTS.md` Known Pitfalls ve `BUG_REPORT.md`'ye işle.
