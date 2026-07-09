package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/kvn8888/codegym/backend/internal/api/response"
	"github.com/kvn8888/codegym/backend/internal/generation"
	"github.com/kvn8888/codegym/backend/internal/memory"
)

// GenerateHandler exposes the model-backed generation endpoint. The
// orchestrator is nil when no GenAI provider is configured; the route then
// answers 503 so the frontend can fall back to mocks.
type GenerateHandler struct {
	orchestrator *generation.Orchestrator
	memory       *memory.Service
}

// NewGenerateHandler builds a generation handler. Both dependencies may be
// used per-request with the caller's scoped context.
func NewGenerateHandler(orchestrator *generation.Orchestrator, memoryService *memory.Service) *GenerateHandler {
	return &GenerateHandler{orchestrator: orchestrator, memory: memoryService}
}

type generateRequestBody struct {
	Kind string          `json:"kind"`
	Spec json.RawMessage `json:"spec"`
}

type generateMCQResponse struct {
	Kind      string                   `json:"kind"`
	Questions []generation.MCQQuestion `json:"questions"`
	Provider  string                   `json:"provider"`
	Model     string                   `json:"model"`
}

// Generate handles POST /api/v1/generate. Only kind "mcq" is implemented;
// "problem" and "interview" reuse this route as their orchestration lands.
func (h *GenerateHandler) Generate(w http.ResponseWriter, r *http.Request) {
	if h.orchestrator == nil {
		response.Error(w, http.StatusServiceUnavailable, "generation_unconfigured",
			"Generation is not configured on this server. Set CODEGYM_GENAI_API_KEY to enable it.")
		return
	}

	var body generateRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.")
		return
	}

	switch generation.Kind(body.Kind) {
	case generation.KindMCQ:
		h.generateMCQ(w, r, body.Spec)
	case generation.KindProblem, generation.KindInterview:
		response.Error(w, http.StatusNotImplemented, "kind_not_implemented",
			fmt.Sprintf("Generation kind %q is not implemented yet; only \"mcq\" is available.", body.Kind))
	default:
		response.Error(w, http.StatusBadRequest, "invalid_kind",
			"kind must be one of \"mcq\", \"problem\", or \"interview\".")
	}
}

func (h *GenerateHandler) generateMCQ(w http.ResponseWriter, r *http.Request, rawSpec json.RawMessage) {
	var spec generation.MCQSpec
	if len(rawSpec) > 0 {
		if err := json.Unmarshal(rawSpec, &spec); err != nil {
			response.Error(w, http.StatusBadRequest, "invalid_spec", "spec must be an object with topic, count, and difficulty fields.")
			return
		}
	}

	questions, result, err := generation.GenerateMCQSet(r.Context(), h.orchestrator, spec)
	if err != nil {
		if r.Context().Err() != nil {
			return // client went away; nothing useful to write
		}
		log.Printf("mcq generation failed: %v", err)
		response.Error(w, http.StatusBadGateway, "generation_failed",
			"The model did not return a usable MCQ set. Try again or adjust the topic.")
		h.recordEvent(r, memory.RecordEventInput{
			Source:  "generate",
			Type:    "generation_failed",
			Summary: "MCQ set generation failed.",
			Payload: mustJSON(map[string]any{
				"format":         "mcq",
				"topic":          spec.Topic,
				"schema_version": 1,
			}),
		})
		return
	}

	h.recordEvent(r, memory.RecordEventInput{
		Source:  "generate",
		Type:    "mcq_set_generated",
		Summary: fmt.Sprintf("Generated a %d-question MCQ set.", len(questions)),
		Payload: mustJSON(map[string]any{
			"format":         "mcq",
			"topic":          spec.Topic,
			"difficulty":     spec.Difficulty,
			"question_count": len(questions),
			"provider":       result.Provider,
			"model":          result.Model,
			"schema_version": 1,
		}),
	})

	response.JSON(w, http.StatusOK, generateMCQResponse{
		Kind:      string(generation.KindMCQ),
		Questions: questions,
		Provider:  result.Provider,
		Model:     result.Model,
	})
}

// recordEvent appends a memory event on a best-effort basis; generation
// results must not fail because event persistence hiccuped.
func (h *GenerateHandler) recordEvent(r *http.Request, input memory.RecordEventInput) {
	if h.memory == nil {
		return
	}
	if _, err := h.memory.RecordEvent(r.Context(), input); err != nil {
		log.Printf("could not record %s.%s memory event: %v", input.Source, input.Type, err)
	}
}

func mustJSON(value map[string]any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return encoded
}
