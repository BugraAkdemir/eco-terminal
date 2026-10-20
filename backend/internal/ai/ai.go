// Package ai defines the provider interfaces used by the intelligence layer (RAG answers, photo analysis,
// embeddings, species classification) and an OpenAI-compatible adapter that implements them.
//
// Design rules:
//   - Every capability is optional. An unconfigured capability is a nil interface value in Providers
//     (or, for species classification, the NotConfiguredSpecies implementation). Nothing here panics when
//     the AI is not configured; callers check Providers.Has* / nil and degrade gracefully.
//   - Secrets (API key) never leave this package: Status() exposes only configured/provider/model.
//   - Output of any model is a SUGGESTION. Callers must store/present it with Disclaimer.
//
// Extending: to add a BioCLIP-style (or any local) species classifier, implement SpeciesClassificationProvider
// (e.g. an HTTP adapter to a sidecar) and assign it to Providers.Species via WithSpeciesClassifier;
// no other package changes are needed.
package ai

import (
	"context"
	"errors"
)

// Disclaimer is stored with every AI analysis and returned in the API (AIAnalysis.disclaimer).
const Disclaimer = "Bu sonuç yapay zekâ tarafından üretilmiş doğrulanmamış bir öneridir; uzman incelemesi gerektirir. " +
	"This is an unverified AI suggestion and requires expert review."

// ErrNotConfigured is returned by capabilities that are not configured.
var ErrNotConfigured = errors.New("ai: capability not configured")

// ProviderStatus mirrors the OpenAPI ProviderStatus schema. It never contains secrets.
type ProviderStatus struct {
	Configured bool   `json:"configured"`
	Provider   string `json:"provider"`
	Model      string `json:"model"`
}

// Status mirrors the /ai/providers/status payload.
type Status struct {
	TextGeneration        ProviderStatus `json:"text_generation"`
	Vision                ProviderStatus `json:"vision"`
	Embedding             ProviderStatus `json:"embedding"`
	SpeciesClassification ProviderStatus `json:"species_classification"`
}

// Message is one chat turn.
type Message struct {
	Role    string // system | user | assistant
	Content string
}

// TextRequest asks a model to continue a conversation.
type TextRequest struct {
	Messages    []Message
	Temperature *float64 // nil = provider default
	MaxTokens   int      // 0 = provider default
}

// TextResult is a model completion.
type TextResult struct {
	Text             string
	Model            string
	PromptTokens     int
	CompletionTokens int
}

// ImageInput is raw image bytes (already validated/re-encoded by internal/media).
type ImageInput struct {
	MIME string
	Data []byte
}

// VisionRequest asks a multimodal model about one or more images.
type VisionRequest struct {
	System string
	Prompt string
	Images []ImageInput
}

// EmbeddingResult holds one vector per input text, in order.
type EmbeddingResult struct {
	Vectors [][]float32
	Model   string
}

// SpeciesRequest asks a classifier to rank candidate species for photos.
type SpeciesRequest struct {
	Images []ImageInput
	Region string // optional hint, e.g. "Kastamonu"
}

// SpeciesCandidate is one ranked prediction.
type SpeciesCandidate struct {
	CommonName     string
	ScientificName string
	Score          float64 // 0..1
}

// TextGenerationProvider generates text (RAG answers, narratives).
type TextGenerationProvider interface {
	Generate(ctx context.Context, req TextRequest) (TextResult, error)
	Status() ProviderStatus
}

// VisionAnalysisProvider analyses images with a multimodal model.
type VisionAnalysisProvider interface {
	AnalyzeImages(ctx context.Context, req VisionRequest) (TextResult, error)
	Status() ProviderStatus
}

// EmbeddingProvider produces dense vectors for retrieval.
type EmbeddingProvider interface {
	Embed(ctx context.Context, texts []string) (EmbeddingResult, error)
	Dimensions() int
	Status() ProviderStatus
}

// SpeciesClassificationProvider is the adapter slot for dedicated species classifiers (e.g. BioCLIP served
// behind an HTTP sidecar). No implementation ships by default; see NotConfiguredSpecies.
type SpeciesClassificationProvider interface {
	Classify(ctx context.Context, req SpeciesRequest) ([]SpeciesCandidate, error)
	Status() ProviderStatus
}

// NotConfiguredSpecies is the default SpeciesClassificationProvider: always reports not configured.
type NotConfiguredSpecies struct{}

func (NotConfiguredSpecies) Classify(context.Context, SpeciesRequest) ([]SpeciesCandidate, error) {
	return nil, ErrNotConfigured
}
func (NotConfiguredSpecies) Status() ProviderStatus { return ProviderStatus{} }
