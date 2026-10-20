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

> **GÜNCELLEME (kota duruşundan sonra):** B (Flutter temeli) artık **main'e MERGE EDİLDİ** (apps/ çakışmaları B'nin sürümüyle çözüldü). B'nin kendi raporu: `flutter analyze` temiz, `flutter test` 37 geçti, `flutter build linux` başarılı — ancak **bunları ben henüz yeniden çalıştırmadım** (ilk iş: `cd apps/flutter_app && flutter pub get && flutter analyze && flutter test && flutter build linux`). Görsel doğrulama YOK (xvfb ekran görüntüsü siyahtı).
> B'nin açık işleri: fontlar (Fraunces/Atkinson Hyperlegible/IBM Plex Mono) bundle EDİLMEDİ (assets/fonts + pubspec + ğĞşŞıİöçü kontrolü); `flutter build web` sonucu okunmadı; gerçek backend ile çalıştırıp 3 genişlikte görsel kontrol. Eklenti noktaları: `apps/flutter_app/lib/app_extensions.dart` (`registerPanelTab`, `registerRoute`, `registerMapOverlay`), ayrıntı `docs/architecture/flutter.md`.
> Sözleşme belirsizlikleri (B): PATCH /auth/me `consent_location` vs User `location_consent`; /regions/{id}/observations'ta `status` filtresi yok; liste sıralaması belirsiz; tek `category` filtresi. Hâlâ yapılacak: E2 (simülasyon handler), D2 (AI/RSS handler), C, F, Faz 3 — yukarıdaki sıra geçerli.

> **GÜNCELLEME 2:** E2 (simülasyon HTTP katmanı) **main'e MERGE EDİLDİ** (`merge: simulation HTTP handlers...`). Lead doğrulaması: `make vet` temiz, `make test` (-race) tüm paketler `ok` (tests/ 11.5s, yeni TestSimulationHTTP dahil).
> Eklenenler: `/simulations/run|list|{id}|compare`, public `/environment/measurements`, başlangıçta il iklim verisini hazırlayan worker, `ECO_CLIMATE_SNAPSHOT`, OpenAPI eklemeleri (`scale`, `model`, `region_id`, `coverage`, `attribution`, compare).
> E2'nin yapmadığı: canlı sunucuda gerçek `POST /simulations/run` denemesi (il, −20/+2), il centroid'inin snapshot ERA5 hücresine düştüğünün kontrolü (düşmezse ilk istek "veri hazırlanıyor" insufficient_data döner), SIMULATION.md'ye HTTP bölümü. Explainer şimdilik nil (`newSimulationModule(d, simulation.WithExplainer(x))`).
> **Kalan sıra:** (1) B'yi yeniden doğrula (flutter analyze/test/build linux) + fontları gömme; (2) D2: AI/RSS handler'ları + jobs/rss worker'ları `extensions.go`'ya; (3) C ajanı (sihirbaz + offline kuyruk + uzman paneli); (4) F ajanı (AI/Resmî/Simülasyon UI); (5) Faz 3 (e2e, README/DEMO/DATA_SOURCES, final rapor).

# Handoff — 2026-10-20 (Session 2b) — KOTA DURDURMASI: nereden devam edilecek

## Durum (kota bitti, hesap değişecek)
Genel ilerleme ~%60. `main` GitHub'a push'lu. Aşağıda birleşen ve bekleyen parçalar:

| Parça | Durum | Konum |
|-------|-------|-------|
| Faz 0 (iskelet, şema, OpenAPI, AGENTS.md, plan.md, design.md) | ✅ | main |
| A backend-core | ✅ merge + e2e + güvenlik denemeleri geçti | main (`cde7e97`) |
| E1 simülasyon motoru + gerçek ERA5 verisi | ✅ | main (`a8238a8`) |
| D1 ai/jobs/rag/rss servisleri | ✅ merge edildi, `make vet` + `make test -race` geçti. **HTTP handler/wiring YOK** | main (bu commit) |
| B Flutter temeli | ⚠️ **MERGE EDİLMEDİ**: branch `worktree-agent-af2f37e9ce2340c3d` (4 commit, çalışma ağacı temiz, ~11.700 satır Dart). `flutter analyze/test/build linux` henüz benim tarafımdan doğrulanmadı | yerel branch |
| E2 simülasyon HTTP handler'ları | ⚠️ yarım: branch `worktree-agent-adcbb5755c0329019`, 13 dosya commit'siz olabilir (durdurma mesajı gönderildi, ajan commit etmiş olabilir) | yerel branch |

Yerel ajan branch'leri GitHub'da YOK (yalnız `main` push'lu). Aynı makinede devam edilecekse `.claude/worktrees/agent-*` dizinleri duruyor.
Başka makineden devam edilecekse önce bu branch'leri push et: `git push origin worktree-agent-af2f37e9ce2340c3d worktree-agent-adcbb5755c0329019`.

## Sıradaki adımlar (bu sırayla)
1. **E2'yi kontrol et:** `cd .claude/worktrees/agent-adcbb5755c0329019 && git status && git log --oneline main..HEAD`. Commit'siz dosya varsa commit et. `git merge main`, `make vet && make test`. Yarımsa tamamla: `/simulations/run|list|{id}|compare`, `/environment/measurements`, başlangıçta iklim verisi hazırlayan worker (`ECO_CLIMATE_SNAPSHOT`), OpenAPI eklemeleri (`scale`, `model`, `region_id`, `coverage`, compare, atıf "Weather data by Open-Meteo.com"), `internal/app/extensions.go`'ya tek satır. Sonra main'e merge.
2. **B'yi doğrula ve merge et:** `git merge --no-ff worktree-agent-af2f37e9ce2340c3d`; çakışma çıkarsa B'nin `apps/` dosyalarını al (B yalnız `apps/flutter_app` + `docs/architecture/flutter.md` sahibi). Sonra `cd apps/flutter_app && flutter pub get && flutter analyze && flutter test && flutter build linux`; backend'i `ECO_SEED_DEMO=true` ile çalıştırıp uygulamayı Linux'ta aç, ekran görüntüsüyle bak. Eklenti noktaları: `docs/architecture/flutter.md`.
3. **D2 (lead veya yeni ajan):** D1'in servislerini HTTP'ye bağla: `/ai/chat`, `/ai/providers/status`, `/ai/analyze-observation`, `/feeds`, `/feeds/articles`, `/admin/feeds`, `/admin/feeds/sync`. Başlangıçta `rss.SeedSources(db,"datasets/seed/rss_sources.json")`, `jobs.NewWorker` + `jobs.RegisterDefaultHandlers`, `rss.NewScheduler`. `rag.Caller` mutlaka oturumdan (`auth.UserFrom`) kurulmalı, yoksa erişim filtresi atlanır. Kullanılacak imzalar D1 raporunda: `rag.New(db,cfg,providers)`, `Answer(ctx, ChatRequest, Caller)`, `rss.NewService(db,cfg,ragEngine)`, `jobs.EnqueueTx`. E2'nin `Explainer` arayüzüne D1'in text provider'ını bağlayan ince adaptör yaz.
4. **C ajanı (Flutter):** gözlem sihirbazı (6 adım) + **çevrimdışı kuyruk** (`client_ref`, bkz. plan.md) + uzman/review paneli (+ `/expert`, `/observations/new` route'ları). **F ajanı:** AI Asistan, Resmî Duyurular, Simülasyon sekmeleri + analiz grafikleri (+ Open-Meteo atfı). İkisi paralel, B'nin eklenti noktalarına dosya ekleyerek. Tasarım kuralı: `docs/architecture/design.md`.
5. **Faz 3:** entegrasyon e2e (gözlem→review→harita→AI sorgusu; RSS→RAG→kaynaklı cevap; bölge→simülasyon→karşılaştırma), README/DEMO/DATA_SOURCES/ARCHITECTURE dokümanları, `AGENTS.md` Known Pitfalls ve `BUG_REPORT.md` güncelle, dürüst "yapılmadı/doğrulanamadı" raporu. Sonra push.

## Bilinen sınırlar / açık riskler (ayrıntı: ilgili ajan raporları ve docs/)
- AI hiç gerçek sağlayıcıyla denenmedi (sahte OpenAI sunucusuyla test edildi). `ECO_AI_*` doldurulunca ilk iş gerçek chat/vision/embedding denemesi.
- RSS: doğrulanan 10 kaynak `docs/data_sources/rss.md`; MGM, AFAD, DKMP, Kastamonu Valiliği için çalışan RSS bulunamadı (entegrasyon iddia edilmiyor).
- Simülasyon: delta-change deneyi, tahmin değil; wbsi standart SPEI değil; ERA5 25 km hücre; Open-Meteo ücretsiz API ticari olmayan kullanım içindir.
- Android SDK/Chrome yok; iOS/Windows/macOS bu makinede derlenemez — README'de "doğrulanmadı" yazıyor.
- Arama `q` LIKE (İ/ı katlanmaz); sadeleştirilmiş sınırlarda sınır noktaları komşu ilçeye düşebilir; job temizliği yok.
- Test kapsamı bilinçli olarak az tutuldu (kullanıcı kararı); kritik yollar (yetki, yükleme, hassas konum, durum makinesi, RAG erişim filtresi, simülasyon matematiği, e2e) testli.
- Commit kuralı: AI-attribution satırı YOK (AGENTS.md rule 6).

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

### 3b. Durum anlık görüntüsü (~22 dk çalışma sonrası, ölçüm: dosya/commit sayımı)
| Ajan | Commit | Kod | Not |
|------|--------|-----|-----|
| A backend-core | 3 | ~5.300 satır Go (temel hariç), 10 paketin hepsi mevcut | Testler az (2 test dosyası) — test yazımı bekleniyor; e2e henüz yok |
| B flutter-foundation | **0** | ~9.200 satır Dart | Hiç commit atmadı (kurala aykırı); tasarım yönergesi (`docs/architecture/design.md`) mesajla iletildi |
| D1 intelligence | 1 | ~3.800 satır Go; `ai`, `rag`, `rss` paketleri var | 5 test dosyası; `jobs` paketi henüz görünmüyor |
| E1 simulation | 2 | ~5.300 satır Go; `simulation` paketi | 7 test dosyası |
Genel ilerleme tahmini: **~%35** (ölçüm değil). Hepsi hâlâ çalışıyor, hiçbiri final rapor vermedi.

### 3c. Tasarım yönergesi
`docs/architecture/design.md` ("Saha Atlası"): kullanım kolaylığı önce; sade Türkçe, 48dp+ dokunma alanı, çevrimdışı durum görünür, durum etiketleri ikon+yazı,
AI çıktısı "doğrulanmadı" etiketli, Fraunces + Atkinson Hyperlegible + IBM Plex Mono (gömülü), %200 yazı boyutu testi. B'ye iletildi; C/F ajanları da buna uyacak.

### 3d. E1 (simülasyon) BİTTİ ve main'e merge edildi (`a8238a8`)
Doğrulama (lead, merge sonrası): `make vet` temiz; `make test` → simulation, simulation/climate, simulation/model, storage `ok` (-race).
- Gerçek veri: Open-Meteo ERA5 (`models=era5`, 0.25° ≈ 25 km hücre) 1991–2025 günlük yağış/sıcaklık, `datasets/environmental/kastamonu_era5_daily_1991_2025.json` (96 KB, 12.784 gün, 0 null). Lisans CC BY 4.0; ücretsiz API **yalnız ticari olmayan** kullanım (ticari dağıtımda ücretli plan gerekir).
- Model: SPI (gamma), "wbsi" su dengesi indeksi (SPEI DEĞİL: log-logistik PWM; 4/12 ayda normal fallback), Thornthwaite PET (yalnız sıcaklık → ısınmada PET artışını abartabilir), delta-change senaryo. Referans ve senaryo aynı referans dağılımına göre standartlaştırılıyor.
- Örnek (−%20 yağış, +2 °C): yıllık yağış 603→482 mm, PET 690→761 mm, su dengesi −87→−279 mm, SPI-3 ≤ −1 ay sayısı 52→122.
- Yeni migration `0020_simulation_climate_fetches.sql` (çekilen aralık defteri). Phase 2'de OpenAPI'ye eklenecek: `scale` (1/3/6), Result'a `model`,`region_id`,`coverage`, compare payload.
- Phase 2 (E2) için: handler'lar + `internal/app` kaydı; ilk indirme `Service.EnsureData` ile tetiklenmeli (GET içinde fetch yok). Explainer = D1'in text provider'ı.
- Yapılmadı: HTTP handler/wiring; wbsi R-SPEI ile çapraz doğrulanmadı.

### 3e. A (backend core) BİTTİ ve main'e merge edildi (`cde7e97`)
Çakışmalar (add/add: main.go, go.mod/go.sum, openapi.yaml) A'nın sürümleriyle çözüldü, `go mod tidy` yapıldı.
**Lead doğrulaması (gerçek çıktı):** `make vet` temiz; `make test` → geographic, httpx, observations, simulation(+climate,model), storage, tests `ok` (-race);
canlı sunucu (ECO_SEED_DEMO=true) + `scripts/e2e.sh` → `PASS: e2e flow ok`. Güvenlik denemeleri (curl): anon → review/admin 401; citizen → review/admin/region-oluştur 403;
zayıf şifre 400; metin/PHP dosyası .png/.jpg diye yüklenince 415; başka kullanıcı PATCH/başkasının onaysız kaydı 404; anonim listede hassas 2 kayıt **genelleştirilmiş koordinatla** (exact_location yok);
sahte X-Forwarded-For ile 25 login denemesinde 16 tanesi 429 (XFF yok sayılıyor); `q=' OR 1=1--` 200 (parametreli SQL); path traversal 404; 3 MB gövde → 400.
- Sözleşme: `0002_client_ref.sql` + OpenAPI (`client_ref`, duplicate → 200, medya sha256 idempotent, observed_at ≤ 10 yıl geçmiş).
- Gerçek sınırlar: geoBoundaries gbOpen (ODbL) — Kastamonu ili + 20 ilçe (21 bölge), `docs/data_sources/regions.md`.
- Bilinen sınırlar (A raporu): `q` arama LIKE (İ/ı katlanmaz); bbox her zaman PUBLIC koordinata göre; poligon self-intersection kontrolü yok;
  tek medya silme endpoint'i yok (sözleşmede yok); sadeleştirilmiş sınırlarda (~40 m) sınır noktaları komşu ilçeye düşebilir; WebP kaydı JPEG/PNG'ye çevriliyor.
- Eklenti noktası: `backend/internal/app/extensions.go` (Module{Register}, Worker{Start,Stop}); yardımcılar `backend/internal/httpx/README.md`.
- Küçük not: 3 MB JSON gövde 413 yerine 400 dönüyor (kabul edilebilir).

### 3f. Anlık durum (~33 dk) — ilerleme tahmini **~%55**
| Ajan | Durum |
|------|-------|
| A backend-core | ✅ BİTTİ, main'de (`cde7e97`), e2e + güvenlik denemeleri geçti |
| E1 simulation | ✅ Faz 1 BİTTİ, main'de (`a8238a8`); **Faz 2 (E2) başladı**: HTTP handler'ları, `/environment/measurements`, compare, startup'ta iklim verisi hazırlama |
| B flutter-foundation | ⏳ çalışıyor — 3 commit, ~11.700 satır Dart, test commit'i atmış (modeller, API istemcisi, route guard, responsive kabuk, 2x yazı ölçeği); final rapor bekleniyor |
| D1 intelligence | ⏳ çalışıyor — 4 commit; `ai`, `jobs`, `rag`, `rss` paketleri artık mevcut; son commit "rag: index, hybrid retrieval, access filter, facts and cited answers"; final rapor bekleniyor |
| C (wizard + offline kuyruk + uzman paneli) | başlamadı — B bitince |
| F (AI/Resmî/Simülasyon arayüzleri) | başlamadı — B bitince |
| D2 (AI/RSS handler + wiring) | başlamadı — D1 bitince |

Sıradaki adımlar: D1 → merge + D2; B → merge (analyze/test/build linux kontrolü) + C ve F paralel başlat.

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

İlerleme tahmini (ölçüm değil): proje ~%35 (22 dk sonra); ayrıntı yukarıda 3b.

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
