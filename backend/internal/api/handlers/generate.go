package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/kvn8888/codegym/backend/internal/api/response"
	"github.com/kvn8888/codegym/backend/internal/generation"
	"github.com/kvn8888/codegym/backend/internal/intake"
	"github.com/kvn8888/codegym/backend/internal/memory"
	"github.com/kvn8888/codegym/backend/internal/problems"
	"github.com/kvn8888/codegym/backend/internal/session"
)

// GenerateHandler exposes the model-backed generation endpoint. The
// orchestrator is nil when no GenAI provider is configured; the route then
// answers 503 so the frontend can fall back to mocks.
type GenerateHandler struct {
	orchestrator           *generation.Orchestrator
	memory                 *memory.Service
	profiles               *generation.ProfileSynthesizer
	refreshOnSetCompletion bool
	intakes                *intake.Service
	problems               *problems.Service
	sessions               *session.Service
}

// NewGenerateHandler builds a generation handler. Both dependencies may be
// used per-request with the caller's scoped context.
func NewGenerateHandler(orchestrator *generation.Orchestrator, memoryService *memory.Service, profiles *generation.ProfileSynthesizer, refreshOnSetCompletion bool, intakes *intake.Service, problemService *problems.Service, sessions *session.Service) *GenerateHandler {
	return &GenerateHandler{
		orchestrator: orchestrator, memory: memoryService, profiles: profiles,
		refreshOnSetCompletion: refreshOnSetCompletion, intakes: intakes, problems: problemService, sessions: sessions,
	}
}

type generateRequestBody struct {
	Kind     string          `json:"kind"`
	Spec     json.RawMessage `json:"spec"`
	IntakeID string          `json:"intake_id,omitempty"`
}

type generateMCQResponse struct {
	Kind      string                   `json:"kind"`
	Questions []generation.MCQQuestion `json:"questions"`
	Provider  string                   `json:"provider"`
	Model     string                   `json:"model"`
}

type generateProblemResponse struct {
	Kind      string           `json:"kind"`
	ProblemID string           `json:"problem_id"`
	Problem   problems.Problem `json:"problem"`
	Provider  string           `json:"provider"`
	Model     string           `json:"model"`
}

type evaluateFreeResponseBody struct {
	QuestionID     string `json:"question_id"`
	Question       string `json:"question"`
	Concept        string `json:"concept"`
	ExpectedAnswer string `json:"expected_answer"`
	Rubric         string `json:"rubric"`
	Answer         string `json:"answer"`
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
		h.generateMCQ(w, r, body.Spec, body.IntakeID)
	case generation.KindProblem:
		h.generateProblem(w, r, body.Spec, body.IntakeID)
	case generation.KindInterview:
		response.Error(w, http.StatusNotImplemented, "kind_not_implemented",
			fmt.Sprintf("Generation kind %q is not implemented yet.", body.Kind))
	default:
		response.Error(w, http.StatusBadRequest, "invalid_kind",
			"kind must be one of \"mcq\", \"problem\", or \"interview\".")
	}
}

func (h *GenerateHandler) generateProblem(w http.ResponseWriter, r *http.Request, rawSpec json.RawMessage, intakeID string) {
	if h.problems == nil {
		response.Error(w, http.StatusServiceUnavailable, "problem_store_unconfigured", "Problem storage is not configured.")
		return
	}
	if h.sessions != nil {
		pending, err := h.sessions.HasPendingMemoryUpdate(r.Context())
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "memory_status_failed", "Could not verify memory freshness.")
			return
		}
		if pending {
			response.Error(w, http.StatusConflict, "memory_update_required", "Retry the saved memory update before generating the next coding problem.")
			return
		}
	}
	var spec generation.ProblemSpec
	if len(rawSpec) > 0 {
		if err := json.Unmarshal(rawSpec, &spec); err != nil {
			response.Error(w, http.StatusBadRequest, "invalid_spec", "spec must be an object with topic, prompt, and difficulty fields.")
			return
		}
	}
	normalizedSpec, err := generation.NormalizeProblemSpec(spec)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_spec", err.Error())
		return
	}
	spec = normalizedSpec
	if h.intakes != nil && strings.TrimSpace(intakeID) != "" {
		intakeContext, err := h.intakes.Context(r.Context(), intakeID)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "invalid_intake", "intake_id is not available in this workspace.")
			return
		}
		spec.IntakeContext = intakeContext
	}
	definition, result, err := generation.GenerateProblem(r.Context(), h.orchestrator, spec)
	if err != nil {
		if r.Context().Err() != nil {
			return
		}
		log.Printf("problem generation failed class=%s detail=%s", generation.DiagnosticClass(err), generation.DiagnosticMessage(err))
		response.Error(w, http.StatusBadGateway, "generation_failed", "The model did not return a usable coding problem. Try again or adjust the topic.")
		return
	}
	problem, err := h.problems.PersistGenerated(r.Context(), definition)
	if err != nil {
		log.Printf("generated problem persistence failed: %v", err)
		response.Error(w, http.StatusInternalServerError, "problem_persistence_failed", "The generated problem could not be saved.")
		return
	}
	h.recordEvent(r, memory.RecordEventInput{
		Source:  memory.SourceGenerate,
		Type:    memory.TypeProblemGenerated,
		Summary: "Generated a personalized coding problem.",
		Payload: mustJSON(map[string]any{
			"problem_id": problem.ID, "topic": spec.Topic, "difficulty": spec.Difficulty,
			"concept": problem.Subcategory, "provider": result.Provider, "model": result.Model,
			"schema_version": 1,
		}),
	})
	response.JSON(w, http.StatusOK, generateProblemResponse{
		Kind: string(generation.KindProblem), ProblemID: problem.ID, Problem: problem,
		Provider: result.Provider, Model: result.Model,
	})
}

func (h *GenerateHandler) generateMCQ(w http.ResponseWriter, r *http.Request, rawSpec json.RawMessage, intakeID string) {
	var spec generation.MCQSpec
	if len(rawSpec) > 0 {
		if err := json.Unmarshal(rawSpec, &spec); err != nil {
			response.Error(w, http.StatusBadRequest, "invalid_spec", "spec must be an object with topic, count, and difficulty fields.")
			return
		}
	}
	if h.intakes != nil && strings.TrimSpace(intakeID) != "" {
		intakeContext, err := h.intakes.Context(r.Context(), intakeID)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "invalid_intake", "intake_id is not available in this workspace.")
			return
		}
		spec.IntakeContext = intakeContext
	}

	questions, result, err := generation.GenerateMCQSet(r.Context(), h.orchestrator, spec)
	if err != nil {
		if r.Context().Err() != nil {
			return // client went away; nothing useful to write
		}
		log.Printf("mcq generation failed class=%s detail=%s", generation.DiagnosticClass(err), generation.DiagnosticMessage(err))
		response.Error(w, http.StatusBadGateway, "generation_failed",
			"The model did not return a usable MCQ set. Try again or adjust the topic.")
		h.recordEvent(r, memory.RecordEventInput{
			Source:  memory.SourceGenerate,
			Type:    memory.TypeGenerationFailed,
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
		Source:  memory.SourceGenerate,
		Type:    memory.TypeMCQSetGenerated,
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

// EvaluateFreeResponse grades one short-answer item through the configured
// provider. Failed evaluation never changes session or memory state; the
// frontend keeps the draft answer available for retry or skip.
func (h *GenerateHandler) EvaluateFreeResponse(w http.ResponseWriter, r *http.Request) {
	if h.orchestrator == nil {
		response.Error(w, http.StatusServiceUnavailable, "generation_unconfigured", "AI evaluation is not configured on this server.")
		return
	}
	var body evaluateFreeResponseBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.")
		return
	}
	evaluation, err := generation.EvaluateFreeResponse(r.Context(), h.orchestrator, generation.FreeResponseEvaluationInput{
		QuestionID: body.QuestionID, Question: body.Question, Concept: body.Concept,
		ExpectedAnswer: body.ExpectedAnswer, Rubric: body.Rubric, Answer: body.Answer,
	})
	if err != nil {
		if r.Context().Err() != nil {
			return
		}
		if generation.DiagnosticClass(err) == "invalid_output" {
			log.Printf("free-response evaluation failed class=%s detail=%s", generation.DiagnosticClass(err), generation.DiagnosticMessage(err))
			response.Error(w, http.StatusBadGateway, "evaluation_failed", "The evaluator did not return a usable result. Retry or skip this question.")
			return
		}
		if strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "at most") {
			response.Error(w, http.StatusBadRequest, "invalid_evaluation_input", err.Error())
			return
		}
		log.Printf("free-response evaluation failed class=%s detail=%s", generation.DiagnosticClass(err), generation.DiagnosticMessage(err))
		response.Error(w, http.StatusBadGateway, "evaluation_failed", "Could not evaluate this answer. Retry or skip this question.")
		return
	}
	response.JSON(w, http.StatusOK, evaluation)
}

type maintainProfileRequestBody struct {
	SessionID string `json:"session_id"`
}

// MaintainProfile runs full profile synthesis after a practice set. The
// response remains the updated Profile so current clients do not need a
// contract migration.
func (h *GenerateHandler) MaintainProfile(w http.ResponseWriter, r *http.Request) {
	var body maintainProfileRequestBody
	if r.Body != nil {
		// An empty or absent body is fine; only malformed JSON is rejected.
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err != io.EOF {
			response.Error(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.")
			return
		}
	}

	if !h.refreshOnSetCompletion {
		profile, err := h.memory.GetProfile(r.Context())
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "memory_profile_failed", "Could not load memory profile.")
			return
		}
		response.JSON(w, http.StatusOK, profile)
		return
	}

	result, err := h.profiles.RefreshProfile(r.Context(), generation.ProfileRefreshInput{
		SessionID: body.SessionID,
		Trigger:   "set-completion",
	})
	if err != nil {
		if r.Context().Err() != nil {
			return
		}
		log.Printf("memory profile synthesis failed: %v", err)
		response.Error(w, http.StatusInternalServerError, "memory_profile_synthesis_failed", "Could not update memory profile.")
		return
	}
	if result.Skipped != "" {
		log.Printf("memory profile synthesis used fallback: %s", result.Skipped)
		if result.Skipped == "generation is not configured" ||
			strings.HasPrefix(result.Skipped, "profile generation failed") ||
			strings.HasPrefix(result.Skipped, "generated profile was invalid") {
			response.Error(w, http.StatusBadGateway, "memory_profile_synthesis_failed", "The result is saved, but memory could not be updated. Retry this update.")
			return
		}
	}

	// Audit trail: one memory event per applied CRUD action so the memory
	// page shows the model editing its own notes.
	for _, action := range result.AppliedActions {
		eventType := map[string]string{
			"create": memory.TypeNoteCreated,
			"update": memory.TypeNoteUpdated,
			"prune":  memory.TypeNotePruned,
		}[action.Op]
		if eventType == "" {
			continue
		}
		h.recordEvent(r, memory.RecordEventInput{
			Source:  memory.SourceMemory,
			Type:    eventType,
			Summary: fmt.Sprintf("Model %sd note %q after a practice round.", action.Op, firstNonEmptyString(action.Note.Title, action.Note.ID)),
			Payload: mustJSON(map[string]any{
				"note_id":        action.Note.ID,
				"tags":           action.Note.Tags,
				"action":         action.Note.Action,
				"session_id":     body.SessionID,
				"schema_version": 1,
			}),
		})
	}
	if h.sessions != nil && strings.TrimSpace(body.SessionID) != "" {
		if _, err := h.sessions.SetMemoryUpdateStatus(r.Context(), body.SessionID, "synced"); err != nil {
			if errors.Is(err, session.ErrNotFound) {
				response.JSON(w, http.StatusOK, result.Profile)
				return
			}
			response.Error(w, http.StatusInternalServerError, "memory_status_failed", "Memory was updated, but its session status could not be saved.")
			return
		}
	}

	response.JSON(w, http.StatusOK, result.Profile)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
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
