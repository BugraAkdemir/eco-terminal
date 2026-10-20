package rag

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/ai"
)

// Errors for the HTTP layer (map to validation_failed / not_found).
var (
	ErrEmptyQuestion   = errors.New("rag: question is empty")
	ErrQuestionTooLong = errors.New("rag: question is too long (max 2000 characters)")
	ErrRegionNotFound  = errors.New("rag: region not found")
)

// ChatRequest is the body of POST /ai/chat.
type ChatRequest struct {
	Message  string
	RegionID string
	From, To string
	History  []HistoryTurn
}

// ChatAnswer mirrors the OpenAPI ChatAnswer schema.
type ChatAnswer struct {
	Answer      string      `json:"answer"`
	Mode        string      `json:"mode"` // generative | retrieval_only
	Region      *RegionInfo `json:"region"`
	Period      PeriodInfo  `json:"period"`
	Facts       []Fact      `json:"facts"`
	Sources     []SourceRef `json:"sources"`
	Limitations []string    `json:"limitations"`
	NeedsRegion bool        `json:"needs_region"`
}

// RegionInfo is the resolved region.
type RegionInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// PeriodInfo is the period used for facts/retrieval ("" when none).
type PeriodInfo struct {
	From string `json:"from"`
	To   string `json:"to"`
}

const answerSources = 6

// Answer is the whole /ai/chat pipeline: understand -> resolve region/period -> SQL facts -> access-filtered
// hybrid retrieval -> generative answer (when a text provider exists) or deterministic retrieval-only summary.
func (e *Engine) Answer(ctx context.Context, req ChatRequest, caller Caller) (ChatAnswer, error) {
	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		return ChatAnswer{}, ErrEmptyQuestion
	}
	if len([]rune(msg)) > 2000 {
		return ChatAnswer{}, ErrQuestionTooLong
	}
	now := e.now()
	in := Understand(msg, now)
	en := in.English
	out := ChatAnswer{Mode: "retrieval_only", Facts: []Fact{}, Sources: []SourceRef{}, Limitations: []string{}}

	// region: explicit id wins, then a place named in the question
	regionID, regionName := "", ""
	if req.RegionID != "" {
		err := e.DB.QueryRowContext(ctx, `SELECT name FROM geographic_regions WHERE id = ?`, req.RegionID).Scan(&regionName)
		if errors.Is(err, sql.ErrNoRows) {
			return ChatAnswer{}, ErrRegionNotFound
		}
		if err != nil {
			return ChatAnswer{}, err
		}
		regionID = req.RegionID
	} else if id, name, err := e.ResolveRegionByName(ctx, msg); err != nil {
		return ChatAnswer{}, err
	} else if id != "" {
		regionID, regionName = id, name
	}
	if regionID != "" {
		out.Region = &RegionInfo{ID: regionID, Name: regionName}
	}

	// period: explicit request values win over the question text
	period := in.Period
	if req.From != "" || req.To != "" {
		period = Period{From: req.From, To: req.To, Label: bi{"istekte belirtilen dönem", "period given in the request"}.get(en)}
		if len(period.From) > 10 {
			period.From = period.From[:10]
		}
		if len(period.To) > 10 {
			period.To = period.To[:10]
		}
	}
	out.Period = PeriodInfo{From: period.From, To: period.To}

	if in.NeedsRegion && regionID == "" {
		out.NeedsRegion = true
		out.Answer = bi{"Hangi bölgeyi kastettiğinizi belirleyemedim. Lütfen haritadan bir bölge seçin veya bölge adını (ör. ilçe) yazın.",
			"I could not tell which area you mean. Please select a region on the map or name the place (e.g. a district)."}.get(en)
		out.Limitations = append(out.Limitations, bi{"Bölge belirtilmediği için sayım ve kaynak araması yapılmadı.", "No count or search was run because no region was given."}.get(en))
		return out, nil
	}

	// SQL facts for aggregate-type questions
	if in.Aggregate || in.Trend || in.TopSpecies {
		in.Terms = withoutRegionTerms(in.Terms, regionName)
		facts, lims, err := e.ComputeFacts(ctx, in, regionID, regionName, period, caller)
		if err != nil {
			return ChatAnswer{}, err
		}
		out.Facts, out.Limitations = facts, append(out.Limitations, lims...)
	}

	// retrieval (query = question; a very short follow-up is enriched with the previous user turn)
	query := msg
	if len(tokenize(msg)) < 4 {
		for i := len(req.History) - 1; i >= 0; i-- {
			if req.History[i].Role == "user" {
				query = req.History[i].Content + " " + msg
				break
			}
		}
	}
	hits, err := e.Search(ctx, SearchRequest{Query: query, RegionID: regionID, From: period.From, To: period.To, Limit: answerSources}, caller)
	if err != nil {
		return ChatAnswer{}, err
	}
	statuses := e.observationStatuses(ctx, hits)
	var pSources []promptSource
	for i, h := range hits {
		ref := sourceRef(i+1, h, statuses[h.SourceID])
		out.Sources = append(out.Sources, ref)
		pSources = append(pSources, promptSource{ID: ref.ID, Type: ref.Type, Title: h.Title, Date: ptrStr(ref.Date), Text: h.Text})
	}
	// facts are citable sources too (type sql_fact)
	var pFacts []promptFact
	for _, f := range out.Facts {
		id := strconv.Itoa(len(out.Sources) + 1)
		out.Sources = append(out.Sources, SourceRef{ID: id, Type: "sql_fact", Title: f.Label, Snippet: f.Label + ": " + f.Value})
		pFacts = append(pFacts, promptFact{ID: id, Fact: f})
	}
	if len(hits) == 0 {
		out.Limitations = append(out.Limitations, bi{"Bu soruyla eşleşen erişilebilir kaynak bulunamadı.", "No accessible source matched this question."}.get(en))
	}
	if regionID != "" && len(hits) == 0 {
		out.Limitations = append(out.Limitations, bi{"Seçili bölge için ilgili kaynak yok; yanıt yalnızca sayısal verilere dayanıyor.", "No sources for the selected region; the answer relies on numeric data only."}.get(en))
	}
	if !e.VectorSearchEnabled(ctx) {
		out.Limitations = append(out.Limitations, bi{"Anlamsal (vektör) arama kapalı; yalnızca anahtar sözcük araması kullanıldı.", "Semantic (vector) search is off; keyword search only."}.get(en))
	}

	// generative mode
	if e.AI.HasText() && (len(hits) > 0 || len(out.Facts) > 0) {
		msgs := BuildPrompt(PromptInput{Question: msg, English: en, RegionName: regionName, Period: period, Facts: pFacts, Sources: pSources, History: req.History})
		temp := 0.2
		res, gerr := e.AI.Text.Generate(ctx, ai.TextRequest{Messages: msgs, Temperature: &temp, MaxTokens: 700})
		if gerr == nil {
			clean, _, invalid := ValidateCitations(res.Text, len(out.Sources))
			if len(invalid) > 0 {
				out.Limitations = append(out.Limitations, bi{"Yanıttaki geçersiz kaynak numaraları kaldırıldı.", "Invalid source numbers in the answer were removed."}.get(en))
			}
			if bad := UngroundedNumbers(clean, groundingText(msg, out)); len(bad) > 0 {
				out.Limitations = append(out.Limitations, bi{"Yanıttaki şu sayılar sağlanan verilerde bulunamadı, doğrulanmadan kullanmayın: ", "These numbers in the answer were not found in the supplied data; do not rely on them: "}.get(en)+strings.Join(bad, ", "))
			}
			out.Answer, out.Mode = clean, "generative"
			return out, nil
		}
		e.Log.Warn("rag: text generation failed; falling back to retrieval-only answer", "err", gerr)
		out.Limitations = append(out.Limitations, bi{"Metin üretimi başarısız oldu; yapılandırılmış özet gösteriliyor.", "Text generation failed; showing a structured summary."}.get(en))
	} else if !e.AI.HasText() {
		out.Limitations = append(out.Limitations, bi{"Yapay zekâ metin sağlayıcısı yapılandırılmadı; yanıt kayıtlı verilerden derlenmiş yapılandırılmış özettir.", "No AI text provider is configured; the answer is a structured summary of stored data."}.get(en))
	}
	out.Answer = retrievalOnlyAnswer(en, out, len(hits))
	return out, nil
}

func ptrStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func withoutRegionTerms(terms []string, regionName string) []string {
	if regionName == "" {
		return terms
	}
	rt := tokenize(foldTR(regionName))
	var out []string
	for _, t := range terms {
		drop := false
		for _, r := range rt {
			if t == r || (len(r) >= 4 && strings.HasPrefix(t, r)) {
				drop = true
			}
		}
		if !drop {
			out = append(out, t)
		}
	}
	return out
}

func groundingText(question string, a ChatAnswer) string {
	var b strings.Builder
	b.WriteString(question + "\n")
	for _, f := range a.Facts {
		b.WriteString(f.Label + " " + f.Value + " " + f.QueryNote + "\n")
	}
	for _, s := range a.Sources {
		b.WriteString(s.Snippet + " " + s.Title + " " + ptrStr(s.Date) + "\n")
	}
	b.WriteString(a.Period.From + " " + a.Period.To)
	return b.String()
}

// observationStatuses returns the lifecycle status of observation hits (for SourceRef.status).
func (e *Engine) observationStatuses(ctx context.Context, hits []Hit) map[string]string {
	ids := []any{}
	for _, h := range hits {
		if h.SourceType == "observation" {
			ids = append(ids, h.SourceID)
		}
	}
	out := map[string]string{}
	if len(ids) == 0 {
		return out
	}
	rows, err := e.DB.QueryContext(ctx, `SELECT id, status FROM observations WHERE id IN (`+placeholders(len(ids))+`)`, ids...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id, st string
		if rows.Scan(&id, &st) == nil {
			out[id] = st
		}
	}
	return out
}

func sourceRef(n int, h Hit, status string) SourceRef {
	typ := h.SourceType
	switch typ {
	case "manual":
		typ = "scientific"
	case "observation", "rss_article", "scientific", "region_report":
	default:
		typ = "scientific"
	}
	r := SourceRef{ID: strconv.Itoa(n), Type: typ, Title: h.Title, Snippet: snippet(h.Text, 240)}
	if h.URL != "" {
		u := h.URL
		r.URL = &u
	}
	if h.SourceType == "observation" || h.SourceType == "rss_article" {
		id := h.SourceID
		r.EntityID = &id
	}
	if h.DocDate != "" {
		d := h.DocDate
		r.Date = &d
	}
	if status != "" {
		s := status
		r.Status = &s
	}
	return r
}

func snippet(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > max {
		return string(r[:max]) + "…"
	}
	return s
}

// retrievalOnlyAnswer is the deterministic fallback: facts first (they are the only numbers), then the top sources.
func retrievalOnlyAnswer(en bool, a ChatAnswer, nHits int) string {
	var b strings.Builder
	if len(a.Facts) == 0 && nHits == 0 {
		return bi{"Bu soru için yeterli kayıt veya kaynak bulunamadı. Soruyu bölge, tür veya dönem belirterek yeniden sormayı deneyin.",
			"No sufficient records or sources were found for this question. Try asking again with a region, species or period."}.get(en)
	}
	b.WriteString(bi{"Bu yanıt yapay zekâ metin üretimi olmadan, kayıtlı verilerden derlenmiş yapılandırılmış bir özettir.\n",
		"This is a structured summary compiled from stored data, without AI text generation.\n"}.get(en))
	if len(a.Facts) > 0 {
		b.WriteString(bi{"\nSayısal bulgular (SQL ile hesaplandı):\n", "\nNumeric findings (computed with SQL):\n"}.get(en))
		for _, s := range a.Sources {
			if s.Type == "sql_fact" {
				var f Fact
				for _, ff := range a.Facts {
					if ff.Label == s.Title {
						f = ff
					}
				}
				fmt.Fprintf(&b, "• %s: %s [%s]\n", f.Label, f.Value, s.ID)
			}
		}
	}
	if nHits > 0 {
		b.WriteString(bi{"\nİlgili kaynaklar:\n", "\nRelevant sources:\n"}.get(en))
		for _, s := range a.Sources {
			if s.Type == "sql_fact" {
				continue
			}
			date := ""
			if s.Date != nil && *s.Date != "" {
				date = " (" + (*s.Date)[:min(10, len(*s.Date))] + ")"
			}
			st := ""
			if s.Status != nil {
				st = " [" + *s.Status + "]"
			}
			fmt.Fprintf(&b, "[%s] %s%s%s — %s\n", s.ID, s.Title, date, st, s.Snippet)
		}
	}
	return strings.TrimSpace(b.String())
}
