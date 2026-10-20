package app

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/ai"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/audit"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/auth"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/httpx"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/jobs"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/observations"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/rag"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/rss"
	"github.com/BugraAkdemir/eco-terminal/backend/internal/storage"
)

func (m *intelligenceModule) providerStatus(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, m.providers.Status())
}

// Normalize to calendar days; the RAG and feed date filters operate at day granularity.
func intelligencePeriod(from, to string) (string, string, error) {
	var f, t string
	var err error
	if from != "" {
		f, err = httpx.ParseTimeParam("from", from, false)
		if err != nil {
			return "", "", err
		}
		f = f[:10]
	}
	if to != "" {
		t, err = httpx.ParseTimeParam("to", to, true)
		if err != nil {
			return "", "", err
		}
		t = t[:10]
	}
	if f != "" && t != "" && f > t {
		return "", "", httpx.Validation("Başlangıç tarihi bitiş tarihinden sonra olamaz.", nil)
	}
	return f, t, nil
}

func (m *intelligenceModule) chat(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Message  string            `json:"message"`
		RegionID string            `json:"region_id"`
		From     string            `json:"from"`
		To       string            `json:"to"`
		History  []rag.HistoryTurn `json:"history"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, err)
		return
	}
	if strings.TrimSpace(in.Message) == "" || utf8.RuneCountInString(in.Message) > 2000 || len(in.History) > 20 {
		httpx.WriteError(w, httpx.Validation("Soru 1–2000 karakter, geçmiş en fazla 20 mesaj olmalıdır.", nil))
		return
	}
	for _, turn := range in.History {
		if (turn.Role != "user" && turn.Role != "assistant") || utf8.RuneCountInString(turn.Content) > 2000 {
			httpx.WriteError(w, httpx.Validation("Geçmişte yalnızca kullanıcı ve asistan mesajları olabilir (en fazla 2000 karakter).", nil))
			return
		}
	}
	from, to, err := intelligencePeriod(in.From, in.To)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	// Never accept user id / role from JSON, headers or history.
	user, _ := auth.UserFrom(r.Context())
	answer, err := m.rag.Answer(r.Context(), rag.ChatRequest{Message: in.Message, RegionID: in.RegionID, From: from, To: to, History: in.History}, rag.Caller{UserID: user.ID, Role: user.Role})
	switch {
	case errors.Is(err, rag.ErrRegionNotFound):
		err = httpx.NotFound("Bölge bulunamadı.")
	case errors.Is(err, rag.ErrEmptyQuestion) || errors.Is(err, rag.ErrQuestionTooLong):
		err = httpx.Validation("Geçerli bir soru girin.", nil)
	}
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, answer)
}

func (m *intelligenceModule) analyze(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ObservationID string `json:"observation_id"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, err)
		return
	}
	if strings.TrimSpace(in.ObservationID) == "" {
		httpx.WriteError(w, httpx.Validation("Gözlem seçin.", nil))
		return
	}
	user, _ := auth.UserFrom(r.Context())
	obs, err := m.d.Observations.Get(r.Context(), user, in.ObservationID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	if !observations.Privileged(user, obs.UserID) {
		httpx.WriteError(w, httpx.Forbidden("Bu gözlem için analiz isteyemezsiniz."))
		return
	}
	if !m.providers.HasVision() {
		httpx.WriteError(w, httpx.AINotConfigured())
		return
	}
	tx, err := m.d.DB.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	defer tx.Rollback()
	st := m.providers.Vision.Status()
	out := observations.AIAnalysis{ID: storage.NewID(), Status: "pending", Provider: st.Provider, Model: st.Model, Disclaimer: ai.Disclaimer, CreatedAt: storage.Now()}
	raw, _ := json.Marshal(map[string]string{"disclaimer": ai.Disclaimer})
	_, err = tx.ExecContext(r.Context(), `INSERT INTO observation_ai_analysis(id,observation_id,provider,model,status,raw_json,created_at,updated_at) VALUES (?,?,?,?,'pending',?,?,?)`, out.ID, obs.ID, out.Provider, out.Model, string(raw), out.CreatedAt, out.CreatedAt)
	if err == nil {
		_, err = jobs.EnqueueTx(r.Context(), tx, jobs.KindAnalyzeObservation, jobs.AnalyzePayload{ObservationID: obs.ID, AnalysisID: out.ID}, time.Time{})
	}
	if err == nil {
		err = audit.Log(r.Context(), tx, audit.Entry{Actor: auth.ActorFrom(r), Action: "observation.analysis_requested", EntityType: "observation", EntityID: obs.ID, After: map[string]string{"analysis_id": out.ID}})
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	m.jobs.Notify()
	httpx.WriteJSON(w, http.StatusAccepted, out)
}

func (m *intelligenceModule) publicFeeds(w http.ResponseWriter, r *http.Request) {
	m.listFeeds(w, r, false)
}
func (m *intelligenceModule) adminFeeds(w http.ResponseWriter, r *http.Request) {
	m.listFeeds(w, r, true)
}
func (m *intelligenceModule) listFeeds(w http.ResponseWriter, r *http.Request, all bool) {
	sources, err := m.feeds.ListSources(r.Context(), all)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, sources)
}

func (m *intelligenceModule) articles(w http.ResponseWriter, r *http.Request) {
	page, err := httpx.ParsePage(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	q := r.URL.Query()
	from, to, err := intelligencePeriod(q.Get("from"), q.Get("to"))
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	items, total, err := m.feeds.ListArticles(r.Context(), rss.ArticleFilter{RegionID: q.Get("region_id"), SourceID: q.Get("source_id"), Category: q.Get("category"), From: from, To: to, Page: page.Page, PageSize: page.Size})
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSONMeta(w, http.StatusOK, items, page.Meta(total))
}

func (m *intelligenceModule) addFeed(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name            string `json:"name"`
		Organization    string `json:"organization"`
		FeedURL         string `json:"feed_url"`
		Category        string `json:"category"`
		IntervalMinutes int    `json:"interval_minutes"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, err)
		return
	}
	source, err := m.feeds.AddSource(r.Context(), rss.AddSourceInput{Name: in.Name, Organization: in.Organization, FeedURL: in.FeedURL, Category: in.Category, IntervalMinutes: in.IntervalMinutes}, auth.ActorFrom(r))
	if err != nil {
		switch {
		case errors.Is(err, rss.ErrDuplicateSource):
			err = httpx.Conflict("Bu kaynak zaten kayıtlı.")
		case errors.Is(err, rss.ErrValidation), errors.Is(err, rss.ErrInvalidURL), errors.Is(err, rss.ErrBlockedAddress), errors.Is(err, rss.ErrNotFeed), errors.Is(err, rss.ErrTooLarge):
			err = httpx.Validation("Kaynak doğrulanamadı. Erişilebilir bir RSS/Atom adresi girin.", nil)
		default:
			var verification *rss.VerificationError
			if errors.As(err, &verification) {
				err = httpx.Validation("Kaynak doğrulanamadı. Erişilebilir bir RSS/Atom adresi girin.", nil)
			}
		}
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, source)
}

func (m *intelligenceModule) syncFeeds(w http.ResponseWriter, r *http.Request) {
	var in struct {
		SourceID string `json:"source_id"`
	}
	if r.ContentLength != 0 {
		if err := httpx.DecodeJSON(w, r, &in); err != nil {
			httpx.WriteError(w, err)
			return
		}
	}
	if in.SourceID != "" {
		if _, err := m.feeds.GetSource(r.Context(), in.SourceID); err != nil {
			if errors.Is(err, rss.ErrSourceNotFound) || errors.Is(err, sql.ErrNoRows) {
				err = httpx.NotFound("Kaynak bulunamadı.")
			}
			httpx.WriteError(w, err)
			return
		}
	}
	tx, err := m.d.DB.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	defer tx.Rollback()
	id, err := jobs.EnqueueTx(r.Context(), tx, jobs.KindSyncFeed, jobs.SyncPayload{SourceID: in.SourceID}, time.Time{})
	if err == nil {
		err = audit.Log(r.Context(), tx, audit.Entry{Actor: auth.ActorFrom(r), Action: "feed.sync_requested", EntityType: "rss_source", EntityID: in.SourceID, After: map[string]string{"job_id": id}})
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	m.jobs.Notify()
	httpx.WriteJSON(w, http.StatusAccepted, map[string]string{"job_id": id})
}
