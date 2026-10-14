package practice

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// BrowserFile is one learner-visible file in a browser session view.
type BrowserFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// BrowserSession is the learner-safe view of one practice session: exact
// artifact/environment identity, the allowlisted editable set, current
// overlay contents, and safe instructions. It carries no hidden tests,
// reference solutions, commands, or snapshot internals.
type BrowserSession struct {
	SessionID       string        `json:"session_id"`
	Owner           string        `json:"owner"`
	WorkspaceID     string        `json:"workspace_id"`
	ArtifactID      string        `json:"artifact_id"`
	ArtifactVersion string        `json:"artifact_version"`
	Technology      string        `json:"technology"`
	EnvironmentBase string        `json:"environment_base"`
	NetworkPolicy   string        `json:"network_policy"`
	Simulated       bool          `json:"simulated"`
	Instructions    string        `json:"instructions"`
	Editable        []string      `json:"editable"`
	Files           []BrowserFile `json:"files"`
}

// OpenBrowserSession loads the trusted verified artifact for an exact
// artifact-ID pin and opens an isolated server-side session seeded with a
// private copy of its editable files. ArtifactID is required: technology
// and version alone can select another exercise and are never used for
// browser resolution. The returned view carries safe instructions only.
func (s *Service) OpenBrowserSession(ctx context.Context, owner, workspaceID string, ref ArtifactRef) (BrowserSession, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return BrowserSession{}, fmt.Errorf("practice open browser session: %w", err)
	}
	if strings.TrimSpace(owner) == "" {
		return BrowserSession{}, errors.New("practice open browser session requires an owner")
	}
	if strings.TrimSpace(workspaceID) == "" {
		return BrowserSession{}, errors.New("practice open browser session requires a workspace")
	}
	if strings.TrimSpace(ref.ArtifactID) == "" {
		return BrowserSession{}, errors.New("practice open browser session requires an artifact_id pin; technology/version alone can select another exercise")
	}
	if s.Artifacts == nil {
		return BrowserSession{}, errors.New("practice artifact source is not configured")
	}
	if s.Contents == nil {
		return BrowserSession{}, errors.New("practice content store is not configured")
	}
	artifact, err := s.Artifacts.LoadVerifiedArtifact(ctx, owner, ref)
	if err != nil {
		return BrowserSession{}, fmt.Errorf("practice load artifact: %w", err)
	}
	if err := checkArtifact(artifact, ref); err != nil {
		return BrowserSession{}, err
	}
	// Exact identity resolution: the served record must equal the pin on
	// both ID and version. Versions are reported, never substituted.
	if artifact.Record.ArtifactID != strings.TrimSpace(ref.ArtifactID) {
		return BrowserSession{}, fmt.Errorf("%w: requested %q but served %q", ErrArtifactMismatch, ref.ArtifactID, artifact.Record.ArtifactID)
	}
	if pin := strings.TrimSpace(ref.Version); pin != "" && pin != artifact.Record.ArtifactVersion {
		return BrowserSession{}, fmt.Errorf("practice requested version %q but verified artifact is version %q; versions are never substituted",
			pin, artifact.Record.ArtifactVersion)
	}
	manifest := cloneManifest(artifact.Manifest)
	networkPolicy, err := translateNetworkPolicy(manifest)
	if err != nil {
		return BrowserSession{}, err
	}
	editable := append([]string(nil), manifest.Workspace.LearnerEditable...)
	files := make(map[string]string, len(editable))
	for _, name := range editable {
		content, err := s.Contents.ReadArtifactFile(ctx, artifact.Record.SnapshotRef, name)
		if err != nil {
			return BrowserSession{}, fmt.Errorf("practice read base file %q: %w", name, err)
		}
		files[name] = content
	}
	redact, err := captureSensitive(ctx, s.Contents, artifact.Record.SnapshotRef, manifest)
	if err != nil {
		return BrowserSession{}, err
	}
	state := &sessionState{
		id:              newSessionID(),
		owner:           owner,
		workspaceID:     workspaceID,
		artifactID:      artifact.Record.ArtifactID,
		artifactVersion: artifact.Record.ArtifactVersion,
		technology:      artifact.Record.Technology,
		environmentBase: artifact.Record.EnvironmentBase,
		networkPolicy:   networkPolicy,
		snapshot:        artifact.Record.SnapshotRef,
		builderRunID:    artifact.Record.Provenance.BuilderRunID,
		verifierRunID:   artifact.Record.Provenance.VerifierRunID,
		editable:        editable,
		manifest:        manifest,
		files:           files,
		redact:          redact,
	}
	s.mu.Lock()
	if s.sessions == nil {
		s.sessions = map[string]*sessionState{}
	}
	s.sessions[state.id] = state
	view := state.browserView(s.Simulated)
	s.mu.Unlock()
	return view, nil
}

// browserView projects the learner-safe session view. Callers must hold
// the service lock or a snapshot copy.
func (state *sessionState) browserView(simulated bool) BrowserSession {
	editable := append([]string(nil), state.editable...)
	sort.Strings(editable)
	files := make([]BrowserFile, 0, len(editable))
	for _, name := range editable {
		files = append(files, BrowserFile{Path: name, Content: state.files[name]})
	}
	return BrowserSession{
		SessionID:       state.id,
		Owner:           state.owner,
		WorkspaceID:     state.workspaceID,
		ArtifactID:      state.artifactID,
		ArtifactVersion: state.artifactVersion,
		Technology:      state.technology,
		EnvironmentBase: state.environmentBase,
		NetworkPolicy:   state.networkPolicy,
		Simulated:       simulated,
		Instructions:    instructionsFor(state.artifactID),
		Editable:        editable,
		Files:           files,
	}
}

// instructionsFor returns the learner-safe instructions for one artifact.
// Unknown artifacts get a generic safe text; the greet fixture README is
// never served because it documents hidden tests and solutions.
func instructionsFor(artifactID string) string {
	if artifactID == SampleArtifactID {
		return SampleGreetInstructions
	}
	return "Edit the listed files, then use Run to see program output and Submit to run the backend checks. " +
		"Backend feedback reports only a category and short summary; hidden tests are never shown."
}

// GetBrowserSession returns the learner-safe view of one session for the
// requesting owner/workspace.
func (s *Service) GetBrowserSession(sessionID, owner, workspaceID string) (BrowserSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.requireBrowser(sessionID, owner, workspaceID)
	if err != nil {
		return BrowserSession{}, err
	}
	return state.browserView(s.Simulated), nil
}

// WriteBrowserFiles applies a bulk learner edit to the isolated overlay
// for the requesting owner/workspace. Every path must name a
// learner-editable file exactly after normalization and every file must
// fit the backend-owned size cap. The trusted base is never modified.
func (s *Service) WriteBrowserFiles(sessionID, owner, workspaceID string, files map[string]string) error {
	for name, content := range files {
		if len(content) > s.maxFileBytes() {
			return fmt.Errorf("practice file %q exceeds the %d byte limit", name, s.maxFileBytes())
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.requireBrowser(sessionID, owner, workspaceID)
	if err != nil {
		return err
	}
	for name := range files {
		if _, err := allowlistedPath(state.editable, name); err != nil {
			return err
		}
	}
	for name, content := range files {
		cleaned, err := allowlistedPath(state.editable, name)
		if err != nil {
			return err
		}
		state.files[cleaned] = content
	}
	return nil
}

// SaveBrowserExercise persists the current overlay of one session as a
// durable saved exercise. Identity (owner/workspace, artifact ID/version,
// environment, snapshot/build reference, editable set) comes from backend
// state, not client claims. Files are validated against the allowlist and
// size cap before persistence. The save survives service/store restarts
// via the configured SavedExerciseStore.
func (s *Service) SaveBrowserExercise(ctx context.Context, sessionID, owner, workspaceID string) (SavedExercise, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if s.Saved == nil {
		return SavedExercise{}, ErrSavedStoreMissing
	}
	s.mu.Lock()
	state, err := s.requireBrowser(sessionID, owner, workspaceID)
	if err != nil {
		s.mu.Unlock()
		return SavedExercise{}, err
	}
	files := make(map[string]string, len(state.files))
	for name, content := range state.files {
		files[name] = content
	}
	saved := SavedExercise{
		ID:              newSavedID(),
		Owner:           state.owner,
		WorkspaceID:     state.workspaceID,
		ArtifactID:      state.artifactID,
		ArtifactVersion: state.artifactVersion,
		Technology:      state.technology,
		EnvironmentBase: state.environmentBase,
		NetworkPolicy:   state.networkPolicy,
		SnapshotRef:     state.snapshot,
		BuilderRunID:    state.builderRunID,
		VerifierRunID:   state.verifierRunID,
		Files:           files,
		UpdatedAt:       time.Now().UTC(),
	}
	s.mu.Unlock()
	if err := validateSessionFiles(state.editable, files); err != nil {
		return SavedExercise{}, err
	}
	for name, content := range files {
		if _, err := allowlistedPath(state.editable, name); err != nil {
			return SavedExercise{}, err
		}
		if len(content) > s.maxFileBytes() {
			return SavedExercise{}, fmt.Errorf("practice file %q exceeds the %d byte limit", name, s.maxFileBytes())
		}
	}
	if err := s.Saved.SaveExercise(ctx, saved); err != nil {
		return SavedExercise{}, fmt.Errorf("practice save exercise: %w", err)
	}
	return saved, nil
}

// ListBrowserSaved lists durable saved exercises for one owner/workspace
// pair in most-recently-updated order.
func (s *Service) ListBrowserSaved(ctx context.Context, owner, workspaceID string) ([]SavedSummary, error) {
	if s.Saved == nil {
		return nil, ErrSavedStoreMissing
	}
	saved, err := s.Saved.ListSavedExercises(ctx, owner, workspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]SavedSummary, 0, len(saved))
	for _, item := range saved {
		out = append(out, item.Summary())
	}
	return out, nil
}

// ResumeBrowserSaved resolves one saved exercise by exact artifact
// ID/version, checks compatibility and the saved build reference against
// the live verified artifact, opens a fresh session, and validates and
// restores the saved edits. A missing artifact, an incompatible version,
// or a replaced same-ID/version build returns an explicit mismatch —
// never a silent substitution. No hashes are used: the comparison is the
// stored snapshot/build reference against the live record.
func (s *Service) ResumeBrowserSaved(ctx context.Context, owner, workspaceID, savedID string) (BrowserSession, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(owner) == "" {
		return BrowserSession{}, errors.New("practice resume requires an owner")
	}
	if strings.TrimSpace(workspaceID) == "" {
		return BrowserSession{}, errors.New("practice resume requires a workspace")
	}
	if s.Saved == nil {
		return BrowserSession{}, ErrSavedStoreMissing
	}
	if s.Artifacts == nil {
		return BrowserSession{}, errors.New("practice artifact source is not configured")
	}
	if s.Contents == nil {
		return BrowserSession{}, errors.New("practice content store is not configured")
	}
	saved, err := s.Saved.GetSavedExercise(ctx, savedID)
	if err != nil {
		return BrowserSession{}, err
	}
	if saved.Owner != owner {
		return BrowserSession{}, ErrSavedOwnerMismatch
	}
	if saved.WorkspaceID != workspaceID {
		return BrowserSession{}, errors.New("practice saved exercise belongs to another workspace")
	}
	if strings.TrimSpace(saved.ArtifactID) == "" || strings.TrimSpace(saved.ArtifactVersion) == "" {
		return BrowserSession{}, fmt.Errorf("%w: saved record has no exact identity", ErrArtifactMismatch)
	}
	artifact, err := s.Artifacts.LoadVerifiedArtifact(ctx, owner, ArtifactRef{
		ArtifactID: saved.ArtifactID,
		Technology: saved.Technology,
		Version:    saved.ArtifactVersion,
	})
	if err != nil {
		return BrowserSession{}, fmt.Errorf("practice load artifact: %w", err)
	}
	// Exact identity: another artifact with the same technology/version is
	// never substituted.
	if artifact.Record.ArtifactID != saved.ArtifactID {
		return BrowserSession{}, fmt.Errorf("%w: saved %q but served %q", ErrArtifactMismatch, saved.ArtifactID, artifact.Record.ArtifactID)
	}
	if artifact.Record.ArtifactVersion != saved.ArtifactVersion {
		return BrowserSession{}, fmt.Errorf("practice saved version %q but verified artifact is version %q; versions are never substituted",
			saved.ArtifactVersion, artifact.Record.ArtifactVersion)
	}
	if err := checkArtifact(artifact, ArtifactRef{ArtifactID: saved.ArtifactID, Technology: saved.Technology, Version: saved.ArtifactVersion}); err != nil {
		return BrowserSession{}, err
	}
	// Changed-build detection: #189 allows same-ID/version replacement, so
	// the saved snapshot/build reference must still match the live record.
	if artifact.Record.SnapshotRef != saved.SnapshotRef ||
		artifact.Record.Provenance.BuilderRunID != saved.BuilderRunID ||
		artifact.Record.Provenance.VerifierRunID != saved.VerifierRunID {
		return BrowserSession{}, fmt.Errorf("%w: saved build %q no longer matches the live artifact build",
			ErrBuildChanged, saved.SnapshotRef)
	}
	manifest := cloneManifest(artifact.Manifest)
	networkPolicy, err := translateNetworkPolicy(manifest)
	if err != nil {
		return BrowserSession{}, err
	}
	editable := append([]string(nil), manifest.Workspace.LearnerEditable...)
	base := make(map[string]string, len(editable))
	for _, name := range editable {
		content, err := s.Contents.ReadArtifactFile(ctx, artifact.Record.SnapshotRef, name)
		if err != nil {
			return BrowserSession{}, fmt.Errorf("practice read base file %q: %w", name, err)
		}
		base[name] = content
	}
	// Validate and restore saved edits against the live editable set.
	restored := make(map[string]string, len(editable))
	for _, name := range editable {
		restored[name] = base[name]
	}
	for name, content := range saved.Files {
		cleaned, err := allowlistedPath(editable, name)
		if err != nil {
			return BrowserSession{}, fmt.Errorf("practice saved file %q is not learner-editable in the live artifact", name)
		}
		if len(content) > s.maxFileBytes() {
			return BrowserSession{}, fmt.Errorf("practice saved file %q exceeds the %d byte limit", name, s.maxFileBytes())
		}
		restored[cleaned] = content
	}
	if err := validateSessionFiles(editable, restored); err != nil {
		return BrowserSession{}, err
	}
	redact, err := captureSensitive(ctx, s.Contents, artifact.Record.SnapshotRef, manifest)
	if err != nil {
		return BrowserSession{}, err
	}
	state := &sessionState{
		id:              newSessionID(),
		owner:           owner,
		workspaceID:     workspaceID,
		artifactID:      artifact.Record.ArtifactID,
		artifactVersion: artifact.Record.ArtifactVersion,
		technology:      artifact.Record.Technology,
		environmentBase: artifact.Record.EnvironmentBase,
		networkPolicy:   networkPolicy,
		snapshot:        artifact.Record.SnapshotRef,
		builderRunID:    artifact.Record.Provenance.BuilderRunID,
		verifierRunID:   artifact.Record.Provenance.VerifierRunID,
		editable:        editable,
		manifest:        manifest,
		files:           restored,
		redact:          redact,
	}
	s.mu.Lock()
	if s.sessions == nil {
		s.sessions = map[string]*sessionState{}
	}
	s.sessions[state.id] = state
	view := state.browserView(s.Simulated)
	s.mu.Unlock()
	return view, nil
}

func newSavedID() string {
	return "practice_saved_" + newSessionID()[len("practice_sess_"):]
}
