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
	decoder.DisallowUnknownFields()
	var manifest ResultManifest
	if err := decoder.Decode(&manifest); err != nil {
		return ManifestClaim{Status: ManifestMalformed, Error: fmt.Sprintf("decode result manifest: %v", err)}
	}
	if err := requireJSONEOF(decoder); err != nil {
		return ManifestClaim{Status: ManifestMalformed, Error: err.Error()}
	}
	if manifest.Version != ManifestVersion {
		return ManifestClaim{Status: ManifestMalformed, Error: fmt.Sprintf("unsupported result manifest version %d", manifest.Version)}
	}
	for _, artifact := range manifest.Artifacts {
		if strings.TrimSpace(artifact) == "" || filepath.IsAbs(artifact) || escapesRoot(artifact) {
			return ManifestClaim{Status: ManifestMalformed, Error: fmt.Sprintf("invalid artifact path %q", artifact)}
		}
	}
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
