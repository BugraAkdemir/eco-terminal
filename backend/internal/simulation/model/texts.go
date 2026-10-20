package model

import "fmt"

// AssumptionsTR lists the assumptions of THIS run (Turkish; the primary UI language).
// English counterparts are documented in docs/simulations/SIMULATION.md.
func AssumptionsTR(in Input, p Params, cov Coverage) []string {
	out := []string{
		fmt.Sprintf("Referans dönemi %s – %s (%d ay, %.1f yıl); aylık veri kapsaması %%%.0f.", in.From, in.To, cov.Months, cov.Years, 100*cov.Both),
		fmt.Sprintf("Senaryo, gözlenen referans döneminin basit bir delta-değişim (pertürbasyon) denemesidir: yağış × %.2f, sıcaklık %+.1f °C. Bu bir iklim projeksiyonu veya tahmini DEĞİLDİR.", 1+p.PrecipChangePct/100, p.TempChangeC),
		"Yağış ve sıcaklık günlük ERA5 yeniden-analizinden (Open-Meteo üzerinden) gelir; bölgenin merkez noktasına en yakın ızgara hücresi kullanılır, bölge ortalaması değildir.",
		"Potansiyel evapotranspirasyon yalnızca sıcaklık ve enleme dayanan Thornthwaite (1948) yöntemiyle hesaplanır; ısı indeksi her iklimin (referans / senaryo) kendi aylık normallerinden türetilir.",
		fmt.Sprintf("SPI ve su dengesi standart indeksi %d aylık birikim penceresiyle, YALNIZCA referans dönemine uydurulan dağılımlarla hesaplanır; senaryo aynı dağılımlara göre değerlendirilir (kendi dağılımına göre değil).", p.Scale),
		"Kurak ay: standart indeks ≤ −1. Referans dönemde bu oran tanım gereği yaklaşık %16'dır; asıl bilgi senaryodaki kaymadır.",
		"Eksik günler: ayın günlerinin en az %90'ı doluysa ay geçerlidir ve toplam, geçerli günlere göre ay uzunluğuna ölçeklenir; aksi halde ay boş sayılır.",
	}
	if cov.WBSIFallbackMonths > 0 {
		out = append(out, fmt.Sprintf("Su dengesi standart indeksinde %d takvim ayı için log-lojistik dağılım uydurulamadı (örneklem pozitif çarpık değil); bu aylarda normal dağılım kullanıldı.", cov.WBSIFallbackMonths))
	}
	return out
}

// LimitationsTR lists the standing scientific limitations (Turkish).
func LimitationsTR() []string {
	return []string{
		"Bu çıktı bir tahmin veya iklim projeksiyonu değildir; gözlenen geçmişin basit bir 'ya şöyle olsaydı' pertürbasyonudur. Yağış rejimi, mevsimsellik ve olay dinamikleri değişmez varsayılır.",
		"ERA5 ızgarası (~0,25° ≈ 25 km) bölge ölçeğinden kaba olabilir: dağlık Kastamonu'da yükselti ve yerel yağış farkları ızgara hücresinde ortalanır; istasyon (MGM) ölçümü yerine geçmez.",
		"Thornthwaite yalnızca sıcaklığa dayanır; Penman–Monteith gibi radyasyon, rüzgâr ve nem içeren yöntemlerden farklı sonuç verir ve ısınmanın PET etkisini genellikle olduğundan farklı (çoğunlukla fazla) gösterebilir. Su dengesi göstergesi bu nedenle ‘su dengesi standart indeksi’dir; standart SPEI ile birebir aynı olduğu iddia edilmez.",
		"Kısa referans dönemleri (10–20 yıl) dağılım uydurmasını ve normalleri belirsiz kılar; Dünya Meteoroloji Örgütü 30 yıllık normal önerir.",
		"Toprak nemi, kar erimesi, sulama, arazi örtüsü ve yer altı suyu modellenmez; ekolojik tepki, tür/popülasyon etkisi veya risk iddiası içermez.",
		"Gösterge değerleri tek bir deterministik hesaptır; belirsizlik aralığı verilmez.",
	}
}
