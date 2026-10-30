package memory

import (
	"encoding/json"
	"time"
)

// Profile is the long-lived memory summary for a user within a workspace.
type Profile struct {
	Version      int64              `json:"version"`
	Summary      string             `json:"summary"`
	UpdatedAt    time.Time          `json:"updated_at"`
	NextReviewAt time.Time          `json:"next_review_at"`
	Strengths    []string           `json:"strengths"`
	GrowthEdges  []string           `json:"growth_edges"`
	Skills       []SkillProficiency `json:"skills"`
	Notes        []Note             `json:"notes"`
	Provenance   *ProfileProvenance `json:"provenance,omitempty"`
}

// ProfileProvenance records how a curated profile was produced without making
// operational metadata part of the learner-facing UI.
type ProfileProvenance struct {
	SchemaVersion   int                      `json:"schema_version"`
	RefreshID       string                   `json:"refresh_id,omitempty"`
	RefreshAttempt  int                      `json:"refresh_attempt,omitempty"`
	Trigger         string                   `json:"trigger"`
	InputVersion    int64                    `json:"input_profile_version,omitempty"`
	OutputVersion   int64                    `json:"output_profile_version,omitempty"`
	Provider        string                   `json:"provider"`
	Model           string                   `json:"model"`
	SynthesizedAt   time.Time                `json:"synthesized_at"`
	EvidenceThrough time.Time                `json:"evidence_through"`
	EventCount      int                      `json:"event_count"`
	EvidenceDigest  string                   `json:"evidence_digest,omitempty"`
	EvidenceSources []ProfileEvidenceSource  `json:"evidence_sources,omitempty"`
	Completion      *ProfileCompletionSource `json:"completion,omitempty"`
	NotesStage      *ProfileStageProvenance  `json:"notes_stage,omitempty"`
	ProfileStage    *ProfileStageProvenance  `json:"profile_stage,omitempty"`
	MaintainedNotes []Note                   `json:"maintained_notes_snapshot,omitempty"`
	OperationID     string                   `json:"workflow_operation_id,omitempty"`
}

type ProfileCompletionSource struct {
	EventID      string `json:"event_id"`
	SessionID    string `json:"session_id"`
	ProblemID    string `json:"problem_id"`
	SubmissionID string `json:"submission_id"`
}

type ProfileStageProvenance struct {
	PromptRevision string    `json:"prompt_revision"`
	SchemaName     string    `json:"schema_name"`
	SchemaVersion  string    `json:"schema_version"`
	Provider       string    `json:"provider,omitempty"`
	Model          string    `json:"model,omitempty"`
	StartedAt      time.Time `json:"started_at"`
	CompletedAt    time.Time `json:"completed_at"`
}

// ProcessedCompletion is the durable idempotency marker committed with the
// profile update that applied one graded coding submission.
type ProcessedCompletion struct {
	SubmissionID   string
	SessionID      string
	ProblemID      string
	SourceEventID  string
	RefreshID      string
	ProfileVersion int64
	ProcessedAt    time.Time
}

// ProfileEvidenceSource records durable, non-user-facing source identifiers for
// the evidence included in the latest profile synthesis.
type ProfileEvidenceSource struct {
	Key          string    `json:"key"`
	Source       string    `json:"source"`
	Type         string    `json:"type"`
	EventID      string    `json:"event_id,omitempty"`
	SessionID    string    `json:"session_id,omitempty"`
	QuestionID   string    `json:"question_id,omitempty"`
	SubmissionID string    `json:"submission_id,omitempty"`
	ProblemID    string    `json:"problem_id,omitempty"`
	Round        string    `json:"round,omitempty"`
	Digest       string    `json:"digest,omitempty"`
	OccurredAt   time.Time `json:"occurred_at,omitempty"`
}

// SkillProficiency captures skill-level observations in the user profile.
type SkillProficiency struct {
	ID            string    `json:"id"`
	Label         string    `json:"label"`
	Area          string    `json:"area"`
	Level         int       `json:"level"`
	Confidence    int       `json:"confidence"`
	Trend         string    `json:"trend"`
	LastPracticed time.Time `json:"last_practiced"`
}

// Note stores problem-specific learning notes linked to user activity.
type Note struct {
	ID        string    `json:"id"`
	ProblemID string    `json:"problem_id"`
	Title     string    `json:"title"`
	Summary   string    `json:"summary"`
	CreatedAt time.Time `json:"created_at"`
	Tags      []string  `json:"tags"`
	Action    string    `json:"action"`
}

// Event is an append-only memory event emitted from product surfaces.
type Event struct {
	ID          string          `json:"id"`
	WorkspaceID string          `json:"workspace_id"`
	UserID      string          `json:"user_id"`
	Source      string          `json:"source"`
	Type        string          `json:"type"`
	Summary     string          `json:"summary"`
	Payload     json.RawMessage `json:"payload,omitempty"`
	OccurredAt  time.Time       `json:"occurred_at"`
	CreatedAt   time.Time       `json:"created_at"`
}

// RecordEventInput is the HTTP/service input payload for appending events.
type RecordEventInput struct {
	Source     string          `json:"source"`
	Type       string          `json:"type"`
	Summary    string          `json:"summary"`
	Payload    json.RawMessage `json:"payload,omitempty"`
	OccurredAt time.Time       `json:"occurred_at"`
}

func cloneProfile(profile Profile) Profile {
	profile.Strengths = append([]string(nil), profile.Strengths...)
	profile.GrowthEdges = append([]string(nil), profile.GrowthEdges...)
	profile.Skills = append([]SkillProficiency(nil), profile.Skills...)
	profile.Notes = cloneNotes(profile.Notes)
	if profile.Provenance != nil {
		provenance := *profile.Provenance
		provenance.EvidenceSources = append([]ProfileEvidenceSource(nil), profile.Provenance.EvidenceSources...)
		provenance.MaintainedNotes = cloneNotes(profile.Provenance.MaintainedNotes)
		if profile.Provenance.Completion != nil {
			completion := *profile.Provenance.Completion
			provenance.Completion = &completion
		}
		if profile.Provenance.NotesStage != nil {
			stage := *profile.Provenance.NotesStage
			provenance.NotesStage = &stage
		}
		if profile.Provenance.ProfileStage != nil {
			stage := *profile.Provenance.ProfileStage
			provenance.ProfileStage = &stage
		}
		profile.Provenance = &provenance
	}
	return profile
}

func cloneNotes(notes []Note) []Note {
	cloned := make([]Note, len(notes))
	for index, note := range notes {
		cloned[index] = note
		cloned[index].Tags = append([]string(nil), note.Tags...)
	}
	return cloned
}
