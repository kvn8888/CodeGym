// Command environment-isolation-check compares secret fingerprints across the
// CodeGym dev, stg, and prd Doppler configs without emitting secret values.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/kvn8888/codegym/backend/internal/environment"
)

var (
	checkedEnvironments = []environment.Name{environment.Dev, environment.Stg, environment.Prd}
	checkedSecrets      = []string{
		"NEON_CONNECTION_STRING",
		"DAYTONA_API_KEY",
		"CODEGYM_RELAY_TOKEN_SECRET",
	}
)

const defaultFallbackPath = "/private/tmp/codegym-doppler-fallback"

type secretFetcher func(context.Context, environment.Name) (map[string]string, error)

type fingerprint [sha256.Size]byte

type reportRow struct {
	secret       string
	fingerprints map[environment.Name]fingerprint
	missing      map[environment.Name]bool
	overlaps     []string
}

func main() {
	project := flag.String("project", "codegym", "Doppler project to inspect")
	fallback := flag.String("fallback", defaultFallbackPath, "encrypted Doppler fallback path")
	timeout := flag.Duration("timeout", 45*time.Second, "overall Doppler read timeout")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	fetch := dopplerFetcher(*project, *fallback)
	unsafe, err := checkIsolation(ctx, os.Stdout, fetch)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if unsafe {
		os.Exit(1)
	}
}

func dopplerFetcher(project, fallbackPath string) secretFetcher {
	return func(ctx context.Context, codegymEnvironment environment.Name) (map[string]string, error) {
		var output bytes.Buffer
		command := exec.CommandContext(
			ctx,
			"doppler", "secrets", "download",
			"--no-file", "--format", "json", "--silent", "--no-check-version",
			"--project", project, "--config", string(codegymEnvironment),
			"--fallback", fallbackPath,
		)
		command.Stdout = &output
		// Doppler diagnostics are intentionally discarded. The checker never
		// forwards subprocess output because a future CLI version could include
		// sensitive context in an error message.
		command.Stderr = io.Discard
		if err := command.Run(); err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return nil, fmt.Errorf("isolation check timed out reading Doppler config %s", codegymEnvironment)
			}
			return nil, fmt.Errorf("could not read Doppler config %s (subprocess output suppressed)", codegymEnvironment)
		}

		secrets := map[string]string{}
		if err := json.Unmarshal(output.Bytes(), &secrets); err != nil {
			return nil, fmt.Errorf("could not decode Doppler config %s (response suppressed)", codegymEnvironment)
		}
		return secrets, nil
	}
}

// checkIsolation hashes fetched values immediately. Only fixed-size hashes and
// missing-value markers cross the reporting boundary.
func checkIsolation(ctx context.Context, output io.Writer, fetch secretFetcher) (bool, error) {
	if fetch == nil {
		return false, errors.New("isolation check requires a secret fetcher")
	}
	fingerprints := make(map[environment.Name]map[string]fingerprint, len(checkedEnvironments))
	missing := make(map[environment.Name]map[string]bool, len(checkedEnvironments))
	for _, codegymEnvironment := range checkedEnvironments {
		secrets, err := fetch(ctx, codegymEnvironment)
		if err != nil {
			return false, err
		}
		fingerprints[codegymEnvironment] = make(map[string]fingerprint, len(checkedSecrets))
		missing[codegymEnvironment] = make(map[string]bool, len(checkedSecrets))
		for _, name := range checkedSecrets {
			value, ok := secrets[name]
			if !ok || value == "" {
				missing[codegymEnvironment][name] = true
				continue
			}
			fingerprints[codegymEnvironment][name] = sha256.Sum256([]byte(value))
		}
	}

	rows := make([]reportRow, 0, len(checkedSecrets))
	unsafe := false
	for _, name := range checkedSecrets {
		row := reportRow{
			secret: name, fingerprints: make(map[environment.Name]fingerprint, len(checkedEnvironments)),
			missing: make(map[environment.Name]bool, len(checkedEnvironments)),
		}
		for _, codegymEnvironment := range checkedEnvironments {
			row.fingerprints[codegymEnvironment] = fingerprints[codegymEnvironment][name]
			row.missing[codegymEnvironment] = missing[codegymEnvironment][name]
			if row.missing[codegymEnvironment] {
				unsafe = true
			}
		}
		row.overlaps = sharedEnvironmentGroups(row)
		if len(row.overlaps) > 0 {
			unsafe = true
		}
		rows = append(rows, row)
	}
	if err := writeReport(output, rows); err != nil {
		return false, err
	}
	return unsafe, nil
}

func sharedEnvironmentGroups(row reportRow) []string {
	groups := map[fingerprint][]environment.Name{}
	for _, codegymEnvironment := range checkedEnvironments {
		if row.missing[codegymEnvironment] {
			continue
		}
		value := row.fingerprints[codegymEnvironment]
		groups[value] = append(groups[value], codegymEnvironment)
	}
	overlaps := make([]string, 0)
	for _, group := range groups {
		if len(group) < 2 {
			continue
		}
		values := make([]string, 0, len(group))
		for _, codegymEnvironment := range group {
			values = append(values, string(codegymEnvironment))
		}
		overlaps = append(overlaps, strings.Join(values, "="))
	}
	sort.Strings(overlaps)
	return overlaps
}

// writeReport cannot accept secret values: reportRow contains only hashes,
// canonical names, and booleans. This type boundary is the output safety guard.
func writeReport(output io.Writer, rows []reportRow) error {
	writer := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "SECRET\tDEV SHA-256\tSTG SHA-256\tPRD SHA-256\tOVERLAP"); err != nil {
		return err
	}
	for _, row := range rows {
		values := make([]string, 0, len(checkedEnvironments))
		missingEnvironments := make([]string, 0)
		for _, codegymEnvironment := range checkedEnvironments {
			if row.missing[codegymEnvironment] {
				values = append(values, "MISSING")
				missingEnvironments = append(missingEnvironments, string(codegymEnvironment))
				continue
			}
			values = append(values, displayFingerprint(row.fingerprints[codegymEnvironment]))
		}
		status := strings.Join(row.overlaps, ",")
		if len(missingEnvironments) > 0 {
			missingStatus := "missing:" + strings.Join(missingEnvironments, ",")
			if status == "" {
				status = missingStatus
			} else {
				status += ";" + missingStatus
			}
		}
		if status == "" {
			status = "none"
		}
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n", row.secret, values[0], values[1], values[2], status); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func displayFingerprint(value fingerprint) string {
	return hex.EncodeToString(value[:4])
}
