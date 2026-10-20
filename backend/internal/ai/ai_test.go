package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/BugraAkdemir/eco-terminal/backend/internal/config"
)

// fakeOpenAI is a test-only OpenAI-compatible server.
type fakeOpenAI struct {
	srv       *httptest.Server
	chatCalls atomic.Int32
	embCalls  atomic.Int32
	lastChat  atomic.Value // chatRequest as raw map
	lastAuth  atomic.Value
	failFirst int32 // number of initial 5xx/429 answers
	failCode  int
	embDim    int
}

func newFake(t *testing.T) *fakeOpenAI {
	f := &fakeOpenAI{embDim: 4, failCode: 500}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		n := f.chatCalls.Add(1)
		f.lastAuth.Store(r.Header.Get("Authorization"))
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.lastChat.Store(body)
		if n <= f.failFirst {
			if f.failCode == 429 {
				w.Header().Set("Retry-After", "0")
			}
			w.WriteHeader(f.failCode)
			_, _ = w.Write([]byte(`{"error":{"message":"busy sk-secret-key"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"model":"m-actual","choices":[{"message":{"content":"merhaba"}}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`))
	})
	mux.HandleFunc("/v1/embeddings", func(w http.ResponseWriter, r *http.Request) {
		f.embCalls.Add(1)
		var body struct {
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		type item struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		}
		var data []item
		// return in reverse order to prove index handling
		for i := len(body.Input) - 1; i >= 0; i-- {
			v := make([]float32, f.embDim)
			v[0] = float32(i + 1)
			data = append(data, item{Index: i, Embedding: v})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeOpenAI) client() *OpenAIClient {
	c := NewOpenAIClient(f.srv.URL+"/v1", "sk-secret-key", 5*time.Second)
	c.Sleep = func(context.Context, time.Duration) error { return nil }
	return c
}

func TestStatusNeverLeaksSecretsAndHandlesUnconfigured(t *testing.T) {
	none := NewProviders(config.AI{})
	if none.HasText() || none.HasVision() || none.HasEmbedding() || none.HasSpecies() {
		t.Fatal("nothing should be configured")
	}
	st := none.Status()
	if st.TextGeneration.Configured || st.Vision.Configured || st.Embedding.Configured || st.SpeciesClassification.Configured {
		t.Fatalf("status = %+v", st)
	}
	var nilP *Providers
	_ = nilP.Status() // must not panic
	if nilP.HasText() {
		t.Fatal("nil providers has text")
	}

	p := NewProviders(config.AI{BaseURL: "http://x/v1", APIKey: "sk-secret-key", TextModel: "gpt-x", EmbeddingModel: "emb", EmbeddingDim: 8})
	b, _ := json.Marshal(p.Status())
	if strings.Contains(string(b), "sk-secret-key") {
		t.Fatal("secret leaked in status")
	}
	if !p.HasText() || !p.HasEmbedding() || !p.HasVision() {
		t.Fatal("expected text, embedding and (reused) vision")
	}
	if p.Status().Vision.Model != "gpt-x" {
		t.Fatalf("vision should reuse text model, got %q", p.Status().Vision.Model)
	}
	if _, err := p.Species.Classify(context.Background(), SpeciesRequest{}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("species err = %v", err)
	}
}

func TestEmbeddingNeedsDimension(t *testing.T) {
	p := NewProviders(config.AI{BaseURL: "http://x/v1", EmbeddingModel: "emb"})
	if p.HasEmbedding() || len(p.Warnings) == 0 {
		t.Fatal("embedding without dim must be disabled with a warning")
	}
	p = NewProviders(config.AI{TextModel: "m"})
	if p.HasText() || len(p.Warnings) == 0 {
		t.Fatal("models without base URL must be disabled with a warning")
	}
}

func TestGenerateAndAuthHeader(t *testing.T) {
	f := newFake(t)
	text := NewOpenAIText(f.client(), "m")
	res, err := text.Generate(context.Background(), TextRequest{Messages: []Message{{Role: "system", Content: "s"}, {Role: "user", Content: "u"}}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "merhaba" || res.Model != "m-actual" || res.PromptTokens != 3 {
		t.Fatalf("res = %+v", res)
	}
	if f.lastAuth.Load() != "Bearer sk-secret-key" {
		t.Fatalf("auth = %v", f.lastAuth.Load())
	}
}

func TestVisionSendsImageURLContentPart(t *testing.T) {
	f := newFake(t)
	v := NewOpenAIVision(f.client(), "vm")
	_, err := v.AnalyzeImages(context.Background(), VisionRequest{Prompt: "bak", Images: []ImageInput{{MIME: "image/png", Data: []byte{1, 2, 3}}}})
	if err != nil {
		t.Fatal(err)
	}
	body := f.lastChat.Load().(map[string]any)
	msgs := body["messages"].([]any)
	content := msgs[len(msgs)-1].(map[string]any)["content"].([]any)
	img := content[1].(map[string]any)
	if img["type"] != "image_url" {
		t.Fatalf("part = %v", img)
	}
	url := img["image_url"].(map[string]any)["url"].(string)
	if !strings.HasPrefix(url, "data:image/png;base64,") {
		t.Fatalf("url = %s", url)
	}
	if _, err := v.AnalyzeImages(context.Background(), VisionRequest{Prompt: "x"}); err == nil {
		t.Fatal("no images must error")
	}
}

func TestRetriesOn429And5xxThenSucceeds(t *testing.T) {
	for _, code := range []int{429, 503} {
		f := newFake(t)
		f.failFirst, f.failCode = 2, code
		_, err := NewOpenAIText(f.client(), "m").Generate(context.Background(), TextRequest{Messages: []Message{{Role: "user", Content: "u"}}})
		if err != nil {
			t.Fatalf("code %d: %v", code, err)
		}
		if f.chatCalls.Load() != 3 {
			t.Fatalf("code %d: calls = %d", code, f.chatCalls.Load())
		}
	}
}

func TestGivesUpAfterRetriesAndRedactsKey(t *testing.T) {
	f := newFake(t)
	f.failFirst = 100
	_, err := NewOpenAIText(f.client(), "m").Generate(context.Background(), TextRequest{Messages: []Message{{Role: "user", Content: "u"}}})
	var api *APIError
	if !errors.As(err, &api) || api.Status != 500 {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "sk-secret-key") {
		t.Fatalf("key leaked: %v", err)
	}
	if f.chatCalls.Load() != int32(defaultRetries+1) {
		t.Fatalf("calls = %d", f.chatCalls.Load())
	}
}

func TestNoRetryOn4xx(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":{"message":"bad model"}}`))
	}))
	defer srv.Close()
	c := NewOpenAIClient(srv.URL, "", time.Second)
	c.Sleep = func(context.Context, time.Duration) error { return nil }
	_, err := NewOpenAIText(c, "m").Generate(context.Background(), TextRequest{Messages: []Message{{Role: "user", Content: "u"}}})
	var api *APIError
	if !errors.As(err, &api) || api.Status != 400 || calls.Load() != 1 {
		t.Fatalf("err=%v calls=%d", err, calls.Load())
	}
}

func TestResponseSizeCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 1<<20)
		for i := 0; i < 20; i++ {
			_, _ = w.Write(buf)
		}
	}))
	defer srv.Close()
	c := NewOpenAIClient(srv.URL, "", 10*time.Second)
	_, err := NewOpenAIText(c, "m").Generate(context.Background(), TextRequest{Messages: []Message{{Role: "user", Content: "u"}}})
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("err = %v", err)
	}
}

func TestEmbeddingsOrderBatchingAndDimensionCheck(t *testing.T) {
	f := newFake(t)
	emb := NewOpenAIEmbedding(f.client(), "e", 4)
	texts := make([]string, 70) // forces 2 batches
	for i := range texts {
		texts[i] = "t"
	}
	res, err := emb.Embed(context.Background(), texts)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Vectors) != 70 || f.embCalls.Load() != 2 {
		t.Fatalf("vectors=%d calls=%d", len(res.Vectors), f.embCalls.Load())
	}
	if res.Vectors[0][0] != 1 || res.Vectors[5][0] != 6 {
		t.Fatal("vectors not in input order")
	}
	f.embDim = 3
	if _, err := emb.Embed(context.Background(), []string{"a"}); err == nil {
		t.Fatal("dimension mismatch must fail")
	}
}

func TestContextCancelStopsRetry(t *testing.T) {
	f := newFake(t)
	f.failFirst = 100
	ctx, cancel := context.WithCancel(context.Background())
	c := f.client()
	c.Sleep = func(ctx context.Context, d time.Duration) error { cancel(); return ctx.Err() }
	_, err := NewOpenAIText(c, "m").Generate(ctx, TextRequest{Messages: []Message{{Role: "user", Content: "u"}}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}
