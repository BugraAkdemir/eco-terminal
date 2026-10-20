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

**Commit status:** `main` GitHub'a push'landı (Faz 0: `1bfe114`, `dff1c9e`; ardından README + DATABASE.md + plan.md + bu güncelleme).
Ajan branch'leri (`worktree-agent-*`) yalnız yerelde, henüz commit atmadılar, merge edilmedi.

---

## What Was Done

### 1. Faz 0 kararları
- **SQLite sürücüsü:** `mattn/go-sqlite3` (CGO) + sqlite-vec CGO. ncruces WASM yolu denendi, Go 1.27'de ABI uyuşmazlığıyla başarısız
  (bkz. `docs/architecture/vector-store.md`). Spike: vec0 KNN + FTS5 + RTree çalışıyor (SQLite 3.53.4, vec 0.1.6).
  Build etiketleri zorunlu: `-tags "sqlite_fts5 sqlite_rtree"` (`backend/Makefile`).
- **Harita:** `flutter_map` (MapLibre GL eklentileri Linux/Windows masaüstünü desteklemiyor).
- **Ortam:** Go 1.27.1, Flutter 3.44.6. Android SDK ve Chrome YOK → android/web derlemeleri doğrulanamaz; iOS/Windows/macOS bu Linux makinede derlenemez.

### 2. Paralel ajanlar (Faz 1, ayrı worktree'ler)

### Ajan → branch → iş eşlemesi

| Ajan | Branch (yerel) | Worktree dizini | Sahip olduğu yerler | İşi |
|------|----------------|-----------------|---------------------|-----|
| **A — backend-core** | `worktree-agent-a1bf5515fb8264ef9` | `.claude/worktrees/agent-a1bf5515fb8264ef9` | `backend/internal/{httpx,auth,audit,observations,media,geographic,review,notifications,app}`, `cmd/server`, `scripts/e2e.sh`, `datasets/geographical`, `datasets/seed` | Auth/roller, gözlem CRUD + durum makinesi, medya yükleme (EXIF temizleme), bölgeler (gerçek Kastamonu sınırları), review kuyruğu, bildirimler, `client_ref` idempotency, app bağlama, e2e testleri |
| **B — flutter-foundation** | `worktree-agent-af2f37e9ce2340c3d` | `.claude/worktrees/agent-af2f37e9ce2340c3d` | `apps/flutter_app/` (core, tema, shared_widgets, authentication, maps, user_profile) | Tasarım sistemi, API istemcisi + modeller, giriş/kayıt, masaüstü/tablet/mobil kabuk, harita (flutter_map), profil, eklenti noktaları |
| **D1 — intelligence-services** | `worktree-agent-a4729348fff5de718` | `.claude/worktrees/agent-a4729348fff5de718` | `backend/internal/{ai,rag,rss,jobs}`, `datasets/seed/rss_sources.json`, `docs/data_sources/rss.md` | AI sağlayıcı arayüzleri + OpenAI-uyumlu adaptör, job işçisi, RAG (hibrit arama, kaynaklı cevap, erişim filtresi), RSS toplama + zamanlayıcı (yalnız doğrulanmış kaynaklar). HTTP handler yok (Faz 2) |
| **E1 — simulation** | `worktree-agent-adcbb5755c0329019` | `.claude/worktrees/agent-adcbb5755c0329019` | `backend/internal/simulation`, `docs/simulations`, `docs/data_sources/climate.md`, `datasets/environmental` | Open-Meteo (ERA5) iklim verisi, kuraklık/su dengesi modeli, senaryo kaydı ve karşılaştırma. HTTP handler yok (Faz 2) |

`main`: yalnızca lead (ben) çalışıyor; ajan branch'leri bitince sırayla `main`'e merge edilir. Planlanan sonraki ajanlar: **D2** (AI/RSS handler'ları), **E2** (simülasyon handler'ları), **C** (gözlem sihirbazı + offline kuyruk + uzman paneli), **F** (AI/Resmî/Simülasyon arayüzleri).

| Ajan | Sahiplik | Durum |
|------|----------|-------|
| A backend-core | httpx, auth, audit, observations, media, geographic, review, notifications, app, e2e (+ client_ref) | çalışıyor |
| B flutter-foundation | apps/flutter_app core, tema, API istemcisi, auth, shell, harita, profil | çalışıyor |
| D1 intelligence | ai, rag, rss, jobs (yalnız servis katmanı) | çalışıyor |
| E1 simulation | simulation + Open-Meteo iklim verisi (yalnız servis katmanı) | çalışıyor |

### 3. Sonradan eklenen kapsam: çevrimdışı gözlem kuyruğu (kullanıcı fikri)
Dağda/köyde sinyal yokken çekilen gözlemler telefonda saklanıp internet gelince gönderilecek. Karar: `client_ref` (cihazda üretilen ID,
`(user_id, client_ref)` benzersiz) ile idempotent POST + medya sha256 ile idempotent yükleme; `observed_at` gerçek çekim zamanı.
Backend tarafı A'ya iletildi (migration `0002_client_ref.sql` + OpenAPI güncellemesi izinli tek istisna). Flutter kuyruğu C ajanında
(sihirbaz ile birlikte). Çevrimdışı harita karo paketi: stretch.

### 4. Bilinen sorunlar / gözlemler
- A ve B worktree'leri yanlışlıkla eski `7f6c707` commit'inden açıldı (D1/E1 doğru). A temel dosyaları kopyalayarak toparladı; merge'de yalnız ajanların kendi dizinleri alınacak.
- A'nın `httpx/audit/password` kodu okundu: ciddi bulgu yok. Merge'de bakılacaklar: genel `BodyLimit` vs medya limiti, reverse-proxy arkasında tek-IP rate limit,
  audit `before/after` içine PII girmemesi, `/admin/audit-logs?action=` OpenAPI'de yok.
- Android SDK / Chrome yok; iOS/Windows/macOS bu makinede derlenemez.

**Deliberately not done:** Flutter gözlem sihirbazı/uzman paneli/AI/Resmî/Simülasyon ekranları (B bitince C/F ajanları),
D1/E1 HTTP handler'ları (A merge olunca faz 2).

---

## Verification

Plan: `plan.md` (repo kökü) — tüm fazlar ve işaretli ilerleme orada.

İlerleme tahmini (ölçüm değil): proje ~%20; A ~%30, B ~%35, E1 ~%35, D1 ~%10 (ajanlar henüz commit atmadı).

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
