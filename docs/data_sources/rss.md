# Resmî / bilimsel RSS kaynakları (doğrulanmış liste)

Bu belge `datasets/seed/rss_sources.json` içindeki her kaynağın **gerçekten çekilip doğrulandığını** kanıtlar.
Kural (AGENTS.md): uydurma RSS URL'si yasaktır. Aşağıdaki her satır, `curl` ile **2026-10-20** tarihinde
indirilmiş, HTTP 200 dönen, RSS/Atom olarak ayrıştırılabilen ve güncel öğe içeren bir adrestir.
Yöntem: önce kurumun ana sayfasında `<link rel="alternate" type="application/rss+xml">` ve "RSS" bağlantıları
arandı, sonra yaygın yol kalıpları denendi; bulunan her aday indirilip içeriği kontrol edildi.
Kullanıcı ajanı: `EcoTerminal/1.0 (+https://github.com/BugraAkdemir/eco-terminal)`.

## Doğrulanan ve seed'e eklenen kaynaklar (fetched-at: 2026-10-20)

| # | Kaynak | Kurum | Feed URL | Biçim | Öğe | En yeni öğe | Lisans / kullanım notu |
|---|--------|-------|----------|-------|-----|-------------|------------------------|
| 1 | Haberler | T.C. Çevre, Şehircilik ve İklim Değişikliği Bakanlığı | `https://csb.gov.tr/rss` | RSS 2.0 | 20 | 2026-10-20 | Kamu kurumu haber akışı. Feed lisans beyan etmiyor (`dc:rights: Copyright 2026`). Yalnızca başlık + temizlenmiş özet + kaynak bağlantısı saklanır, tam metin kopyalanmaz. |
| 2 | OGM Haberler | Orman Genel Müdürlüğü (Tarım ve Orman Bak.) | `https://www.ogm.gov.tr/tr/haber-sitesi/_layouts/15/listfeed.aspx?List=%7b2B7C95DD-2EE4-45AF-8C0E-248300D5EB7D%7d&Source=https%3a//www.ogm.gov.tr/tr/haber-sitesi/Pages/Forms/AllItems.aspx` | RSS 2.0 (SharePoint) | 50 | 2026-10-19 | Kamu kurumu. Lisans beyanı yok; başlık/özet/bağlantı. Başlıklar kaynakta URL slug biçimindedir (ör. `sonbaharin-renkleri-adana-ormanlarinda`), olduğu gibi saklanır. ETag/Last-Modified destekler. |
| 3 | OGM Duyurular | Orman Genel Müdürlüğü | `https://www.ogm.gov.tr/tr/duyurular-sitesi/_layouts/15/listfeed.aspx?List=%7bB765E021-81AE-46C0-A35E-311DB67E9436%7d&Source=https%3a//www.ogm.gov.tr/tr/duyurular-sitesi/Pages/Forms/AllItems.aspx` | RSS 2.0 (SharePoint) | 50 | 2026-10-14 | Yukarıdakiyle aynı. |
| 4 | Haber ve Duyurular | T.C. Kastamonu Belediyesi | `https://www.kastamonu.bel.tr/feed/` | RSS 2.0 (WordPress) | 10 | 2026-10-08 | Yerel yönetim; lisans beyanı yok. Yerel duyurular (yıkım, ihale, etkinlik) çevre dışı içerik de taşır; kategori kuralları sadece öneridir. |
| 5 | TÜBİTAK Bilim Genç | TÜBİTAK | `https://bilimgenc.tubitak.gov.tr/rss.xml` | RSS 2.0 | 10 | 2026-10-16 | Kamu kurumu bilim iletişimi. Lisans beyanı yok; başlık/özet/bağlantı. Feed'de 2017 tarihli bir öğe de vardır (sabitlenmiş içerik). |
| 6 | Doğa Derneği | Doğa Derneği (STK — **resmî kurum değildir**) | `https://dogadernegi.org/tr/feed/` | RSS 2.0 (WordPress) | 10 | 2026-10-12 | Sivil toplum kuruluşu; UI'da "resmî değil" etiketi gösterilmelidir. (`/feed/` → 301 → `/tr/feed/`.) |
| 7 | Copernicus Climate Change Service | Copernicus / ECMWF / AB | `https://climate.copernicus.eu/rss.xml` | RSS 2.0 | 10 | 2026-08-11 | Copernicus açık erişim politikası veri için geçerlidir; haber metni için yayıncı koşulları geçerli, yalnızca özet + bağlantı. |
| 8 | NASA Science (Earth) | NASA | `https://science.nasa.gov/feed/?science_org=19791%2C22453` | RSS 2.0 | 10 | 2026-10-20 | NASA içeriği genel olarak telifsiz yayımlanır (NASA medya yönergeleri); yine de yalnızca özet + bağlantı. Feed kök öğesinde bozuk öznitelik boşluğu vardır (`"xmlns:apod=…"xmlns:media=…`), ayrıştırıcı bunu onarır. `earthobservatory.nasa.gov/feeds/earth-observatory.rss` bu adrese yönlenir. |
| 9 | Nature — Biodiversity | Nature Portfolio (Springer Nature) | `https://www.nature.com/subjects/biodiversity.rss` | RSS 2.0 | 30 | 2026-10-20 | Bilimsel yayın akışı; yalnızca başlık + bağlantı (özet çoğunlukla boş), yayıncı koşulları geçerlidir. |
| 10 | USGS Significant Earthquakes (past month) | U.S. Geological Survey | `https://earthquake.usgs.gov/earthquakes/feed/v1.0/summary/significant_month.atom` | Atom | 15 | 2026-10-20 | ABD federal kamu verisi (kamu malı). **Varsayılan olarak devre dışı** (`enabled:false`): küresel kapsamlı olduğu ve AFAD feed'i bulunmadığı için yalnızca admin isterse açar. |

Her feed'in indirme boyutu (bayt): csb 17 042; ogm haber 155 886; ogm duyuru 195 640; kastamonu.bel 53 464;
tubitak 66 543; doğa derneği 68 449; c3s 159 883; nasa 466 477; nature 12 870; usgs 21 095. Hepsi 5 MB üst sınırının altında.
Test fixture'ları (`backend/internal/rss/testdata/*.xml`) bu indirmelerin kırpılmış (2–3 öğe) gerçek kopyalarıdır.

## Denendi, doğrulanamadı — entegre EDİLMEDİ

| Kurum | Denenenler | Sonuç |
|-------|-----------|-------|
| **MGM** (Meteoroloji Genel Müdürlüğü) | ana sayfada RSS/alternate bağlantısı yok; `/rss`, `/rss.xml`, `/rss/duyuru.xml`, `/site/rss.aspx` | 404 sayfası (200 + HTML). Web aramasında da RSS adresi bulunamadı. |
| **AFAD** | `/rss`, `/feed`, `/rss.xml`, `/haberler/rss`; `deprem.afad.gov.tr` | 404 / yalnızca HTML. Deprem verisi için RSS yok; resmî olay servisi (`deprem.afad.gov.tr` API'si) ayrı bir entegrasyon konusudur. |
| **DKMP** (Doğa Koruma ve Milli Parklar) | `tarimorman.gov.tr/DKMP/Haber/rss`, `/DKMP/Rss` | `…/Haber/rss` HTTP 200 döner ama bir **HTML sayfasıdır**, feed listesi içermez. Feed yok sayıldı. |
| **Tarım ve Orman Bakanlığı** (genel) | `/rss`, `/Rss`, `/rss.xml`, `/Haber/rss` | 404 veya HTML. (OGM ayrı bir SharePoint sitesi olarak feed sunuyor — yukarıda.) |
| **Kastamonu Valiliği** | `kastamonu.gov.tr/rss`, `/feed`, `/rss.xml` | 404. Ana sayfada RSS bağlantısı yok. |
| **Kastamonu Üniversitesi** | `/rss`, `/rss/haberler` | HTML (yönlendirme), feed değil. |
| **DSİ, TÜİK** | `/rss` | HTML hata sayfası / 404. |
| IUCN | `https://iucn.org/rss.xml` | Geçerli RSS ama **güncel değil** (en yeni öğe 2022) — eklenmedi. |
| copernicus.eu | `https://www.copernicus.eu/en/rss.xml` | Geçerli RSS, en yeni öğe 2026-05 — seyrek; eklenmedi (C3S feed'i yeterli). |
| Mongabay | `https://news.mongabay.com/feed/` | Geçerli ve güncel RSS, ancak yeniden yayın koşulları kısıtlayıcı bir gazetecilik içeriği; eklenmedi. |
| WWF Türkiye | `/feed/` | 403 (bot engeli). |
| GBIF haberleri | `/rss/news` | 403 (bot engeli). |
| UNEP, EEA, FAO, WMO | `/rss.xml`, `/rss` | 404. |

**Dürüst sonuç:** Türk kurumlarından yalnızca ÇŞİDB, OGM (2 akış), Kastamonu Belediyesi ve TÜBİTAK Bilim Genç RSS sunuyor.
MGM uyarıları, AFAD deprem/afet duyuruları, DKMP ve Valilik için çalışan bir RSS **bulunamadı**; bu kaynaklar ileride
manuel doküman (`rag.IndexManualDocument`) veya resmî API entegrasyonuyla eklenebilir. Bu kaynaklar için "entegre"
iddiası yapılmamalıdır.

## İşleme kuralları (uygulama: `backend/internal/rss`)

- **Çekme:** koşullu GET (`ETag` / `Last-Modified`, `rss_sources.etag` sütununda `etag\nlast-modified` olarak tutulur),
  zaman aşımı `ECO_RSS_FETCH_TIMEOUT_SECONDS`, 5 MB üst sınır, en çok 5 yönlendirme, `ECO_RSS_USER_AGENT`.
- **SSRF koruması:** yalnızca http/https; kullanıcı bilgisi yok; loopback/özel/link-local/CGNAT/metadata adresleri ve
  bunları gömen IPv6 biçimleri engellenir. Doğrulama hem URL çözümlemede hem de **gerçek bağlantı anında** (dialer)
  yapıldığı için DNS rebinding / yönlendirme ile aşılamaz. `ECO_ENV=production` iken istisna yoktur.
- **Ayrıştırma:** RSS 2.0, RSS 1.0 (RDF), Atom; BOM, ISO-8859-9/windows-1254, CDATA, `content:encoded`, `dc:date`,
  tarih-only `pubDate` (ÇŞİDB), bozuk kök öznitelikleri (NASA) desteklenir.
- **Temizleme:** HTML → düz metin (script/style/yorum atılır, etiketler soyulur, varlıklar çözülür, sıfır genişlikli
  karakterler silinir). Saklanan metin hiçbir zaman HTML olarak yorumlanmaz.
- **Tekilleştirme:** `(source_id, guid)` ve global `content_hash` (başlık+özet+bağlantı). Aynı guid'de içerik değişirse
  güncellenir ve yeniden indekslenir.
- **Tarihler:** `published_at` (feed'den; ayrıştırılamazsa veya 48 saatten fazla gelecekteyse NULL) ve `ingested_at` ayrı tutulur.
- **Bölge etiketleme:** `geographic_regions` (il/ilçe/köy) adları büyük-küçük harf ve aksan duyarsız eşleştirilir
  (`TOSYA'DA`, `Tosyada`). Köy adları ve gündelik sözcük olan ilçe adları (Araç, Küre, Bozkurt…) yalnızca metinde
  "Kastamonu" da geçerse etiketlenir. Eşleşme yoksa **hiçbir bölge etiketlenmez** ve `location_note = "Konum belirlenemedi"`.
- **Kategori:** anahtar sözcük kurallarıyla *öneri* (weather, disaster, forest, wildlife, water, climate, pollution, general).
- **Zamanlayıcı:** kaynak başına `interval_minutes` (en az 5 dk) + kaynağa özgü sabit jitter (≤%10); hatalı kaynak 2× bekler;
  eşzamanlı en çok 3 senkron; çağıranları asla bloklamaz.
- **Seed:** `rss.SeedSources(db, "datasets/seed/rss_sources.json")` yalnızca `rss_sources` boşken yükler.
