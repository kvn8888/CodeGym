package agentruntime

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Network policy for agent preparation runs. The rule is allowlist-first and
// language-agnostic: loopback plus listed registry hosts pass in any file,
// every other URL host is recorded as a violation. No per-language or
// per-framework patterns are added here; the list below grows only with
// registry hosts a demo stack needs to resolve dependencies.
var networkAllowlistHosts = []string{
	"registry.npmjs.org",
	"repo.maven.apache.org",
	"repo1.maven.org",
	"proxy.golang.org",
	"sum.golang.org",
}

var loopbackHosts = []string{
	"127.0.0.1",
	"localhost",
	"::1",
}

const (
	maxScannedFileBytes  = 1 << 20
	maxNetworkViolations = 32
)

var networkURLPattern = regexp.MustCompile(`(?i)https?://(\[[^\]/]+\]|[A-Za-z0-9.-]+)`)

// skippedNetworkDirs are materialized or version-control directories whose
// contents are not agent-authored workspace source.
func skippedNetworkDir(relative string) bool {
	return relative == ".git" || relative == "node_modules"
}

// skippedNetworkFile mirrors the benchmark scanner's treatment of
// runtime-owned agent state: transient tool output is not workspace source
// and must not produce policy findings.
func skippedNetworkFile(relative string) bool {
	lower := strings.ToLower(filepath.ToSlash(relative))
	return lower == "opencode.db" ||
		strings.Contains(lower, "/opencode.db") ||
		strings.HasPrefix(lower, ".opencode/") ||
		strings.HasPrefix(lower, ".local/share/opencode/")
}

func networkHostAllowed(host string) bool {
	host = strings.ToLower(strings.Trim(strings.TrimSpace(host), "[]"))
	for _, allowed := range loopbackHosts {
		if host == allowed {
			return true
		}
	}
	for _, allowed := range networkAllowlistHosts {
		if host == allowed {
			return true
		}
	}
	return false
}

// ScanWorkspaceNetworkHosts reports one violation per file and host for URL
// literals pointing outside the allowlist. It judges nothing; findings are
// telemetry the backend verifier owns the verdict on.
func ScanWorkspaceNetworkHosts(root string) ([]string, error) {
	violations := make([]string, 0)
	seen := make(map[string]struct{})
	overflow := 0
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if skippedNetworkDir(relative) {
				return filepath.SkipDir
			}
			return nil
		}
		if skippedNetworkFile(relative) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Size() > maxScannedFileBytes {
			return nil
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if len(payload) > maxScannedFileBytes {
			return nil
		}
		for _, candidate := range payload {
			if candidate == 0 {
				return nil
			}
		}
		for _, match := range networkURLPattern.FindAllStringSubmatch(string(payload), -1) {
			host := strings.ToLower(match[1])
			if networkHostAllowed(host) {
				continue
			}
			key := filepath.ToSlash(relative) + "\x00" + host
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			if len(violations) >= maxNetworkViolations {
				overflow++
				continue
			}
			violations = append(violations, fmt.Sprintf("non-allowlisted network host %s in %s", host, filepath.ToSlash(relative)))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if overflow > 0 {
		violations = append(violations, fmt.Sprintf("and %d more network policy violations", overflow))
	}
	return violations, nil
}

// checkShellNetworkPolicy denies shell commands reaching non-allowlisted
// network hosts. Package-manager and registry traffic passes by
// construction; only offending destinations fail.
func checkShellNetworkPolicy(command string) error {
	for _, match := range networkURLPattern.FindAllStringSubmatch(command, -1) {
		host := strings.ToLower(match[1])
		if networkHostAllowed(host) {
			continue
		}
		return fmt.Errorf("network policy denied shell command reaching non-allowlisted host %s", host)
	}
	return nil
}

// AppendNetworkPolicyViolations scans the working directory after a run and
// records findings on telemetry. A scan failure is recorded fail-closed. It
// never judges the run; the backend verifier owns the verdict.
func AppendNetworkPolicyViolations(telemetry *Telemetry, workingDirectory string) {
	if telemetry == nil {
		return
	}
	violations, err := ScanWorkspaceNetworkHosts(workingDirectory)
	if err != nil {
		telemetry.PolicyViolations = append(telemetry.PolicyViolations, fmt.Sprintf("network policy scan failed: %v", err))
		return
	}
	telemetry.PolicyViolations = append(telemetry.PolicyViolations, violations...)
}
