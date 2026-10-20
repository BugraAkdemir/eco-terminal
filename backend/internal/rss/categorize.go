package rss

import "strings"

// Article categories produced by SuggestCategory. "general" is the fallback.
const (
	CatWeather   = "weather"
	CatDisaster  = "disaster"
	CatForest    = "forest"
	CatWildlife  = "wildlife"
	CatWater     = "water"
	CatClimate   = "climate"
	CatPollution = "pollution"
	CatGeneral   = "general"
)

type catRule struct {
	cat   string
	words []string // folded (diacritic-free, lower-case) substrings of tokens
}

// Rules are keyword based and deterministic (Turkish + English). The category with most keyword hits wins;
// ties resolve in rule order. These are *suggestions* shown as a filter, not facts.
var catRules = []catRule{
	{CatDisaster, []string{"deprem", "afet", "earthquake", "seismic", "heyelan", "landslide", "sel ", "taskin", "flood", "tsunami", "volkan", "volcano", "acil durum"}},
	{CatForest, []string{"orman", "forest", "yangin", "wildfire", "agaclandirma", "fidan", "ekosistem", "ormancilik", "deforest"}},
	{CatWildlife, []string{"yaban", "wildlife", "milli park", "biyolojik cesitlilik", "biyocesitlilik", "biodiversity", "species", "nesli tukenme", "endangered", "kus ", "birds", "habitat", "koruma alani", "doga koruma", "conservation"}},
	{CatWater, []string{"baraj", "kuraklik", "drought", "nehir", "river", "gol ", "su kaynak", "sulak", "wetland", "su varlig", "irmak", "dere "}},
	{CatClimate, []string{"iklim", "climate", "karbon", "carbon", "emisyon", "emission", "kuresel isinma", "global warming", "sera gaz", "greenhouse", "sicaklik anomali", "temperature anomal"}},
	{CatWeather, []string{"meteoroloji", "yagis", "saganak", "hava durumu", "weather", "firtina", "storm", "kar yagis", "don ", "sicaklik", "ruzgar", "forecast", "tahmin"}},
	{CatPollution, []string{"kirlilik", "pollution", "atik", "waste", "hava kalitesi", "air quality", "plastik", "plastic", "zehir", "toxic", "cop "}},
}

// SuggestCategory returns the best-matching category for title+summary+feed categories, or fallback
// (the source's configured category) when no rule matches. fallback "" means CatGeneral.
func SuggestCategory(title, summary string, feedCats []string, fallback string) string {
	text := " " + foldTR(title+" "+summary+" "+strings.Join(feedCats, " ")) + " "
	best, bestN := "", 0
	for _, r := range catRules {
		n := 0
		for _, w := range r.words {
			needle := w
			if strings.HasSuffix(w, " ") { // trailing space = whole-word match
				needle = " " + w
			}
			if strings.Contains(text, needle) {
				n++
			}
		}
		if n > bestN {
			best, bestN = r.cat, n
		}
	}
	if best != "" {
		return best
	}
	if fallback == "" {
		return CatGeneral
	}
	return fallback
}
