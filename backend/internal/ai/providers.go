package ai

import (
	"github.com/BugraAkdemir/eco-terminal/backend/internal/config"
)

// ProviderName is reported in ProviderStatus.provider for the OpenAI-compatible adapter.
const ProviderName = "openai-compatible"

// Providers groups the configured AI capabilities. A nil field means "not configured" - callers must check
// (or use the Has* helpers). The struct is safe to share between goroutines (providers are stateless).
type Providers struct {
	Text      TextGenerationProvider
	Vision    VisionAnalysisProvider
	Embedding EmbeddingProvider
	Species   SpeciesClassificationProvider

	// Warnings are human-readable configuration problems found by NewProviders (no secrets). Log them at startup.
	Warnings []string
}

// Option customises NewProviders.
type Option func(*Providers)

// WithSpeciesClassifier plugs a species classifier (e.g. a BioCLIP sidecar adapter) into the registry.
func WithSpeciesClassifier(p SpeciesClassificationProvider) Option {
	return func(r *Providers) { r.Species = p }
}

// NewProviders builds the registry from config. Rules:
//   - a base URL is required for every OpenAI-compatible capability;
//   - text: needs ECO_AI_TEXT_MODEL;
//   - vision: ECO_AI_VISION_MODEL, otherwise the text model is reused (it must accept images; if it does not,
//     analysis jobs fail with the provider's error and are recorded as failed - they never block a submission);
//   - embedding: needs ECO_AI_EMBEDDING_MODEL and ECO_AI_EMBEDDING_DIM > 0.
//
// It never fails and never panics; unconfigured capabilities stay nil.
func NewProviders(cfg config.AI, opts ...Option) *Providers {
	r := &Providers{Species: NotConfiguredSpecies{}}
	if cfg.BaseURL != "" {
		client := NewOpenAIClient(cfg.BaseURL, cfg.APIKey, cfg.Timeout)
		if cfg.TextModel != "" {
			r.Text = NewOpenAIText(client, cfg.TextModel)
		}
		visionModel := cfg.VisionModel
		if visionModel == "" {
			visionModel = cfg.TextModel
		}
		if visionModel != "" {
			r.Vision = NewOpenAIVision(client, visionModel)
		}
		switch {
		case cfg.EmbeddingModel != "" && cfg.EmbeddingDim > 0:
			r.Embedding = NewOpenAIEmbedding(client, cfg.EmbeddingModel, cfg.EmbeddingDim)
		case cfg.EmbeddingModel != "":
			r.Warnings = append(r.Warnings, "ECO_AI_EMBEDDING_MODEL is set but ECO_AI_EMBEDDING_DIM is missing/0: embeddings disabled")
		}
	} else if cfg.TextModel != "" || cfg.VisionModel != "" || cfg.EmbeddingModel != "" {
		r.Warnings = append(r.Warnings, "AI models are set but ECO_AI_BASE_URL is empty: AI disabled")
	}
	for _, o := range opts {
		o(r)
	}
	return r
}

// HasText / HasVision / HasEmbedding report whether a capability is configured. Safe on a nil receiver.
func (r *Providers) HasText() bool      { return r != nil && r.Text != nil }
func (r *Providers) HasVision() bool    { return r != nil && r.Vision != nil }
func (r *Providers) HasEmbedding() bool { return r != nil && r.Embedding != nil }
func (r *Providers) HasSpecies() bool {
	return r != nil && r.Species != nil && r.Species.Status().Configured
}

// Status returns the /ai/providers/status payload. It contains no secrets. Safe on a nil receiver.
func (r *Providers) Status() Status {
	var s Status
	if r == nil {
		return s
	}
	if r.Text != nil {
		s.TextGeneration = r.Text.Status()
	}
	if r.Vision != nil {
		s.Vision = r.Vision.Status()
	}
	if r.Embedding != nil {
		s.Embedding = r.Embedding.Status()
	}
	if r.Species != nil {
		s.SpeciesClassification = r.Species.Status()
	}
	return s
}
