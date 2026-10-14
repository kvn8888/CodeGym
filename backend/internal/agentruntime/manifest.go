package agentruntime

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maxManifestBytes = 64 * 1024

func resultManifestInstructions() string {
	return fmt.Sprintf(
		"Before your final response, write %s as a single JSON object. The minimum valid file is exactly {\"version\":%d,\"completed\":true}. completed is your claim that you finished; it is not the verdict, and backend verification alone decides success. Do not wrap the JSON in Markdown or append another JSON value. Optional telemetry fields are summary (string), artifacts (workspace-relative string array), and checks (array of objects with name, passed, and optional detail).",
		ManifestRelativePath, ManifestVersion,
	)
}

func LoadManifest(workingDirectory string) ManifestClaim {
	path := filepath.Join(workingDirectory, filepath.FromSlash(ManifestRelativePath))
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return ManifestClaim{Status: ManifestAbsent, Error: "result manifest is absent"}
	}
	if err != nil {
		return ManifestClaim{Status: ManifestMalformed, Error: fmt.Sprintf("open result manifest: %v", err)}
	}
	defer func() { _ = file.Close() }()

	payload, err := io.ReadAll(io.LimitReader(file, maxManifestBytes+1))
	if err != nil {
		return ManifestClaim{Status: ManifestMalformed, Error: fmt.Sprintf("read result manifest: %v", err)}
	}
	if len(payload) > maxManifestBytes {
		return ManifestClaim{Status: ManifestMalformed, Error: "result manifest exceeds 64 KiB"}
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	var fields map[string]json.RawMessage
	if err := decoder.Decode(&fields); err != nil {
		return ManifestClaim{Status: ManifestMalformed, Error: fmt.Sprintf("decode result manifest: %v", err)}
	}
	if err := requireJSONEOF(decoder); err != nil {
		return ManifestClaim{Status: ManifestMalformed, Error: err.Error()}
	}
	var manifest ResultManifest
	version, ok := fields["version"]
	if !ok {
		return ManifestClaim{Status: ManifestMalformed, Error: "result manifest field version is required"}
	}
	if err := json.Unmarshal(version, &manifest.Version); err != nil {
		return ManifestClaim{Status: ManifestMalformed, Error: fmt.Sprintf("decode result manifest field version: %v", err)}
	}
	if manifest.Version != ManifestVersion {
		return ManifestClaim{Status: ManifestMalformed, Error: fmt.Sprintf("unsupported result manifest version %d", manifest.Version)}
	}
	completed, ok := fields["completed"]
	if !ok {
		return ManifestClaim{Status: ManifestMalformed, Error: "result manifest field completed is required"}
	}
	if err := json.Unmarshal(completed, &manifest.Completed); err != nil {
		return ManifestClaim{Status: ManifestMalformed, Error: fmt.Sprintf("decode result manifest field completed: %v", err)}
	}
	_ = json.Unmarshal(fields["summary"], &manifest.Summary)
	var artifacts []string
	_ = json.Unmarshal(fields["artifacts"], &artifacts)
	for _, artifact := range artifacts {
		if strings.TrimSpace(artifact) == "" || filepath.IsAbs(artifact) || escapesRoot(artifact) {
			continue
		}
		manifest.Artifacts = append(manifest.Artifacts, artifact)
	}
	_ = json.Unmarshal(fields["checks"], &manifest.Checks)
	return ManifestClaim{Status: ManifestPresent, Manifest: &manifest}
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("decode result manifest: multiple JSON values")
		}
		return fmt.Errorf("decode result manifest: %w", err)
	}
	return nil
}

func escapesRoot(path string) bool {
	clean := filepath.Clean(filepath.FromSlash(path))
	return clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator))
}
