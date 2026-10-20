# Bio Radar veri kaynakları

- Bölge sınırları: geoBoundaries gbOpen Kastamonu ili ve 20 ilçe; ODbL ve sadeleştirme sınırları: [regions.md](data_sources/regions.md).
- İklim: Open-Meteo Historical API / ERA5, 1991–2025 günlük referans snapshot'ı; CC BY 4.0, 25 km çözünürlük, atıf ve kullanım koşulları: [climate.md](data_sources/climate.md).
- RSS: yalnız doğrulanmış kaynak listesi; 10 yapılandırma, 9 etkin kaynak. Kurumlar için bulunamayan RSS entegrasyonu varmış gibi gösterilmez: [rss.md](data_sources/rss.md).
- Vatandaş kayıtları: kullanıcı beyanı, uzman doğrulamasından önce bilimsel doğrulanmış kayıt değildir.
- Demo kayıtları: `datasets/seed/demo_observations.json`; 30 kurgu kayıt, tamamı `is_demo=1`. Yeni başlatmada mevcut örnekler sıfırlanmaz; yeni client_ref'ler tekrarsız eklenir.

LLM ölçüm/sayım kaynağı değildir. Sayılar SQL'den, iklim göstergeleri deterministik simülasyon motorundan gelir. Demo kayıtları sunum veritabanında tutulur; gerçek kullanımda `ECO_SEED_DEMO=false` kullanın.
