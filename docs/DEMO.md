# Bio Radar — tanıtım ve kullanım akışı

Bio Radar, Kastamonu'daki doğa gözlemlerini ve çevre sorunlarını vatandaşlardan toplar. Uzmanlar kayıtları inceleyip doğrular; harita, kaynak gösteren soru-cevap ve iklim senaryoları aynı bölge bağlamında bu verileri kullanır. Gözlem sayıları popülasyon büyüklüğü değildir. Uygulama resmî ihbar sistemi veya acil yardım kanalı değildir.

## Başlatma

Linux'ta depo kökünde:

```bash
./run.sh
```

Başlatıcı ayrı bir sunum veritabanı açar, 30 etiketli demo kaydı hazırlar, rastgele üretilen yönetici girişini terminalde gösterir ve Flutter uygulamasını açar. İlk derleme sonraki açılışlardan uzun sürer. Uygulama kapanınca başlatıcının açtığı API de kapanır. Başka bir API 8080'i kullanıyorsa `BIO_RADAR_PORT=8081 ./run.sh` kullanın.

Yalnız API: `./run.sh --api-only`. Giriş bilgileri ve veri `backend/data/presentation/` altında saklanır ve Git'e dahil edilmez. Demo modunda düzenli RSS taraması kapalıdır; yönetici isterse duyuru sekmesinden güncelleme kuyruğu oluşturabilir. AI anahtarı zorunlu değildir. Gerçek AI sağlayıcısı kullanılacaksa `ECO_AI_*` ayarlarını yalnız backend ortamına verin.

## 6–8 dakikalık sunum

1. **Sorunu anlatın (30 sn):** “Doğa gözlemleri farklı yerlerde kayboluyor. Bio Radar, vatandaş kaydını uzman doğrulaması ve bölgesel analizle birleştiriyor.”
2. **Haritayı gösterin (1 dk):** Kastamonu veya bir ilçe seçin. Hayvan/bitki/su/orman/kirlilik filtrelerini kullanın. Bilgi sekmesindeki sayılar ve grafikler veritabanından gelir. Veri kalitesi uyarısını gösterin.
3. **Demo örneklerini bulun (1 dk):** Aramaya “Tanıtım” yazın. “Tanıtım: Dere kenarında su samuru” kaydının Demo veri ve hassas konum etiketlerini gösterin. Yönetici tam konumu görebilir; çıkış yaptıktan sonra halka açık görünüm genelleştirilir. “Tanıtım: Ormanda kayın gençleşmesi” ve “Tanıtım: Derede bulanıklık bildirimi” doğrulanmış örneklerdir. Bunlar gerçek gözlem değildir.
4. **Sahadan kayıt (1 dk):** Giriş yapın → Gözlem Ekle. Kategori, fotoğraf (isteğe bağlı), konum, açıklama/zaman, tür tahmini ve son onay olmak üzere altı adımı gösterin. GPS yoksa koordinatı elle girebilirsiniz. Taslak kaydedip çıkın; Profil → Cihazdaki saha kayıtları bölümünden devam edin. Çevrimdışı kayıt ve fotoğraflar cihazda kalır; uygulama açıkken bağlantı gelince tekrar denenir. Aynı kayıt tekrar oluşturulmaz.
5. **Uzman incelemesi (1 dk):** Kullanıcı menüsü → Uzman paneli. “Tanıtım: Kıyıda plastik atık bildirimi” bekleyen örneğini açın. Not, tür düzeltmesi ve hassas tür seçeneğini gösterin; onay/ret için açık karar onayı istenir. “Tanıtım: Yaralı kuş için ek bilgi” kaydı uzman bilgi talebi örneğidir. Sahibi açıklamayı tamamlayıp yeniden incelemeye gönderebilir.
6. **Kaynaklı soru-cevap (1 dk):** Haritada bir bölge seçin → AI Asistan → “Bu bölgede kaç hayvan gözlemi var?” Kaynakları ve sınırlamaları gösterin. Anahtar yokken ekran “Kayıtlı verilerden özet” der; hayalî AI cevabı üretmez. Anahtar varsa da sayıları SQL hesaplar, model açıklar.
7. **İklim deneyi (1 dk):** Yönetici/araştırmacı hesabıyla Kastamonu ilini seçin → Simülasyon. Referans 1991-01–2020-12; yağış −%20, sıcaklık +2 °C. Hesaplanan göstergeleri, referans/senaryo çizgilerini ve Open-Meteo atfını gösterin. İkinci senaryoyu 0/0 ile kaydedip karşılaştırın. Bu bir tahmin değildir; nüfus/tür kaybı sonucu çıkarılmaz.
8. **Dürüst kapanış (30 sn):** “İki süreç, yerel SQLite, uzman kontrolü, hassas konum koruması ve kaynaklı analiz. Demo gözlemler kurgudur; iklim referans verisi gerçek ERA5 verisidir.”

## Demo içerikleri

30 kurgu gözlem/bildirim; yeni 5 örnek “Tanıtım:” ile başlar. Onaylanmış, bekleyen, incelenen, reddedilmiş, ek bilgi istenmiş ve çözülmüş kayıtlar; hassas tür ve farklı kategori örnekleri vardır. Gerçek RSS kaynakları kurgu makalelerle doldurulmaz. Duyuru listesi boşsa tarih/bölge filtresini ve kaynak sağlığını gösterin; çalışan bir entegrasyon varmış gibi anlatmayın.

## Doğrulanan sınırlar

Linux istemcisi ve Go backend doğrulanır. Android/iOS/Windows/macOS/web bu ortamda çalıştırılıp test edilmiş sayılmaz. Kamera ve GPS işletim sistemi donanım/izinlerine bağlıdır; manuel konum ve galeriden fotoğraf yolu vardır. Web saklama kotası tarayıcıya bağlıdır, saklama hatası başarı olarak gösterilmez. Uygulama kapalıyken arka planda mobil senkronizasyon ve çevrimdışı harita karo paketi bu sürümün kapsamı dışındadır. Gerçek uzak AI sağlayıcısı için kullanıcı anahtarıyla ayrıca kabul testi gerekir.
