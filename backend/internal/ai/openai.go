package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// maxResponseBytes caps any provider response body (embeddings batches are the largest legitimate case).
	maxResponseBytes = 16 << 20
	// maxErrorBody is how much of an error body is kept in error messages.
	maxErrorBody   = 512
	defaultRetries = 3
)

// APIError is a non-2xx answer from the provider. Message never contains the API key.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("ai: provider returned HTTP %d: %s", e.Status, e.Message)
}

// Retryable reports whether the failure is worth retrying later (429/5xx).
func (e *APIError) Retryable() bool { return e.Status == http.StatusTooManyRequests || e.Status >= 500 }

// OpenAIClient talks to any OpenAI-compatible API (OpenAI, OpenRouter, Ollama /v1, vLLM, ...).
// BaseURL is the API root including the version segment, e.g. https://api.openai.com/v1.
type OpenAIClient struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
	// MaxRetries is the number of retries (after the first attempt) for 429/5xx/network errors.
	MaxRetries int
	// Backoff returns the wait before retry number attempt (1-based). Defaults to exponential with jitter.
	Backoff func(attempt int) time.Duration
	// Sleep waits d or until ctx is done. Tests replace it to avoid real sleeping.
	Sleep func(ctx context.Context, d time.Duration) error
}

// NewOpenAIClient builds a client with the given per-request timeout.
func NewOpenAIClient(baseURL, apiKey string, timeout time.Duration) *OpenAIClient {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &OpenAIClient{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		APIKey:     apiKey,
		HTTP:       &http.Client{Timeout: timeout},
		MaxRetries: defaultRetries,
	}
}

func (c *OpenAIClient) backoff(attempt int) time.Duration {
	if c.Backoff != nil {
		return c.Backoff(attempt)
	}
	base := 500 * time.Millisecond << (attempt - 1)
	if base > 8*time.Second {
		base = 8 * time.Second
	}
	return base/2 + time.Duration(rand.Int64N(int64(base/2)+1))
}

func (c *OpenAIClient) sleep(ctx context.Context, d time.Duration) error {
	if c.Sleep != nil {
		return c.Sleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// postJSON POSTs body to path and decodes the JSON answer into out, retrying 429/5xx and network errors.
func (c *OpenAIClient) postJSON(ctx context.Context, path string, body, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	var lastErr error
	for attempt := 0; attempt <= c.MaxRetries; attempt++ {
		if attempt > 0 {
			wait := c.backoff(attempt)
			if ra, ok := lastErr.(*retryAfterError); ok && ra.after > 0 {
				wait = ra.after
			}
			if err := c.sleep(ctx, wait); err != nil {
				return err
			}
		}
		err := c.doOnce(ctx, path, payload, out)
		if err == nil {
			return nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !isRetryable(err) {
			return unwrapRetry(err)
		}
	}
	return unwrapRetry(lastErr)
}

// retryAfterError decorates a retryable error with the server-provided Retry-After.
type retryAfterError struct {
	err   error
	after time.Duration
}

func (e *retryAfterError) Error() string { return e.err.Error() }
func (e *retryAfterError) Unwrap() error { return e.err }

func unwrapRetry(err error) error {
	var ra *retryAfterError
	if errors.As(err, &ra) {
		return ra.err
	}
	return err
}

func isRetryable(err error) bool {
	var api *APIError
	if errors.As(err, &api) {
		return api.Retryable()
	}
	var net interface{ Timeout() bool }
	if errors.As(err, &net) {
		return true
	}
	// transport-level failures (connection reset, EOF) are retryable; decode errors are not.
	return errors.Is(err, errTransport)
}

var errTransport = errors.New("transport error")

func (c *OpenAIClient) doOnce(ctx context.Context, path string, payload []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		// Do not echo the URL-with-credentials style errors verbatim; the key is only in a header so it is safe,
		// but keep the wrapper to classify as retryable.
		return fmt.Errorf("%w: %v", errTransport, redactErr(err, c.APIKey))
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("%w: reading body: %v", errTransport, err)
	}
	if len(data) > maxResponseBytes {
		return fmt.Errorf("ai: provider response exceeds %d bytes", maxResponseBytes)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg := extractErrorMessage(data)
		apiErr := &APIError{Status: resp.StatusCode, Message: redactStr(msg, c.APIKey)}
		if apiErr.Retryable() {
			return &retryAfterError{err: apiErr, after: parseRetryAfter(resp.Header.Get("Retry-After"))}
		}
		return apiErr
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("ai: decoding provider response: %w", err)
	}
	return nil
}

func parseRetryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && secs >= 0 {
		d := time.Duration(secs) * time.Second
		if d > 30*time.Second {
			d = 30 * time.Second
		}
		return d
	}
	return 0
}

func extractErrorMessage(data []byte) string {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &e) == nil && e.Error.Message != "" {
		return truncate(e.Error.Message, maxErrorBody)
	}
	return truncate(strings.TrimSpace(string(data)), maxErrorBody)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func redactStr(s, secret string) string {
	if secret == "" {
		return s
	}
	return strings.ReplaceAll(s, secret, "[redacted]")
}

func redactErr(err error, secret string) error {
	if secret == "" {
		return err
	}
	return errors.New(redactStr(err.Error(), secret))
}

// ─── chat completions ──────────────────────────────────────────────

type chatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type chatContentPart struct {
	Type     string        `json:"type"`
	Text     string        `json:"text,omitempty"`
	ImageURL *chatImageURL `json:"image_url,omitempty"`
}

type chatImageURL struct {
	URL string `json:"url"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature *float64      `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

type chatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content any `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

func (c *OpenAIClient) chat(ctx context.Context, model string, msgs []chatMessage, temp *float64, maxTokens int) (TextResult, error) {
	var resp chatResponse
	err := c.postJSON(ctx, "/chat/completions", chatRequest{Model: model, Messages: msgs, Temperature: temp, MaxTokens: maxTokens}, &resp)
	if err != nil {
		return TextResult{}, err
	}
	if len(resp.Choices) == 0 {
		return TextResult{}, errors.New("ai: provider returned no choices")
	}
	text := contentToString(resp.Choices[0].Message.Content)
	if strings.TrimSpace(text) == "" {
		return TextResult{}, errors.New("ai: provider returned an empty completion")
	}
	model2 := resp.Model
	if model2 == "" {
		model2 = model
	}
	return TextResult{Text: text, Model: model2, PromptTokens: resp.Usage.PromptTokens, CompletionTokens: resp.Usage.CompletionTokens}, nil
}

// contentToString handles both string content and the array-of-parts variant some servers return.
func contentToString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []any:
		var sb strings.Builder
		for _, p := range t {
			if m, ok := p.(map[string]any); ok {
				if s, ok := m["text"].(string); ok {
					sb.WriteString(s)
				}
			}
		}
		return sb.String()
	}
	return ""
}

// ─── text generation ───────────────────────────────────────────────

type openAIText struct {
	c     *OpenAIClient
	model string
}

// NewOpenAIText returns a TextGenerationProvider backed by an OpenAI-compatible chat model.
func NewOpenAIText(c *OpenAIClient, model string) TextGenerationProvider {
	return &openAIText{c, model}
}

func (p *openAIText) Status() ProviderStatus {
	return ProviderStatus{Configured: true, Provider: ProviderName, Model: p.model}
}

func (p *openAIText) Generate(ctx context.Context, req TextRequest) (TextResult, error) {
	msgs := make([]chatMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		msgs = append(msgs, chatMessage{Role: m.Role, Content: m.Content})
	}
	return p.c.chat(ctx, p.model, msgs, req.Temperature, req.MaxTokens)
}

// ─── vision ────────────────────────────────────────────────────────

type openAIVision struct {
	c     *OpenAIClient
	model string
}

// NewOpenAIVision returns a VisionAnalysisProvider using chat completions with image_url content parts
// (images are sent inline as base64 data URLs, so no public media URL is ever exposed to the provider).
func NewOpenAIVision(c *OpenAIClient, model string) VisionAnalysisProvider {
	return &openAIVision{c, model}
}

func (p *openAIVision) Status() ProviderStatus {
	return ProviderStatus{Configured: true, Provider: ProviderName, Model: p.model}
}

func (p *openAIVision) AnalyzeImages(ctx context.Context, req VisionRequest) (TextResult, error) {
	if len(req.Images) == 0 {
		return TextResult{}, errors.New("ai: vision request has no images")
	}
	parts := []chatContentPart{{Type: "text", Text: req.Prompt}}
	for _, img := range req.Images {
		mime := img.MIME
		if mime == "" {
			mime = "image/jpeg"
		}
		parts = append(parts, chatContentPart{
			Type:     "image_url",
			ImageURL: &chatImageURL{URL: "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(img.Data)},
		})
	}
	msgs := []chatMessage{}
	if req.System != "" {
		msgs = append(msgs, chatMessage{Role: "system", Content: req.System})
	}
	msgs = append(msgs, chatMessage{Role: "user", Content: parts})
	zero := 0.2
	return p.c.chat(ctx, p.model, msgs, &zero, 600)
}

// ─── embeddings ────────────────────────────────────────────────────

type openAIEmbedding struct {
	c     *OpenAIClient
	model string
	dim   int
}

// NewOpenAIEmbedding returns an EmbeddingProvider (POST /embeddings). dim is the configured vector size;
// responses with another length are rejected so the vec0 table can never be corrupted.
func NewOpenAIEmbedding(c *OpenAIClient, model string, dim int) EmbeddingProvider {
	return &openAIEmbedding{c, model, dim}
}

func (p *openAIEmbedding) Dimensions() int { return p.dim }

func (p *openAIEmbedding) Status() ProviderStatus {
	return ProviderStatus{Configured: true, Provider: ProviderName, Model: p.model}
}

// embedBatch is the max number of inputs per request.
const embedBatch = 64

func (p *openAIEmbedding) Embed(ctx context.Context, texts []string) (EmbeddingResult, error) {
	out := EmbeddingResult{Model: p.model, Vectors: make([][]float32, 0, len(texts))}
	for start := 0; start < len(texts); start += embedBatch {
		end := min(start+embedBatch, len(texts))
		var resp struct {
			Model string `json:"model"`
			Data  []struct {
				Index     int       `json:"index"`
				Embedding []float32 `json:"embedding"`
			} `json:"data"`
		}
		req := map[string]any{"model": p.model, "input": texts[start:end]}
		if err := p.c.postJSON(ctx, "/embeddings", req, &resp); err != nil {
			return EmbeddingResult{}, err
		}
		if len(resp.Data) != end-start {
			return EmbeddingResult{}, fmt.Errorf("ai: embeddings returned %d vectors for %d inputs", len(resp.Data), end-start)
		}
		batch := make([][]float32, end-start)
		for _, d := range resp.Data {
			if d.Index < 0 || d.Index >= len(batch) || batch[d.Index] != nil {
				return EmbeddingResult{}, errors.New("ai: embeddings response has invalid indexes")
			}
			if p.dim > 0 && len(d.Embedding) != p.dim {
				return EmbeddingResult{}, fmt.Errorf("ai: embedding dimension %d does not match configured ECO_AI_EMBEDDING_DIM=%d", len(d.Embedding), p.dim)
			}
			batch[d.Index] = d.Embedding
		}
		out.Vectors = append(out.Vectors, batch...)
	}
	return out, nil
}
