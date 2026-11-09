package agentrelay

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// TestSandboxFacingRelayContract checks the relay a sandbox will actually
// call: a token issued against the configured signing secret must be
// accepted by the deployed relay endpoint. It never runs unasked: set
// CODEGYM_RUN_RELAY_LIVE=1 and CODEGYM_AGENT_RELAY_URL to the public relay
// base (for example https://codegym.onrender.com/api/v1/agent-relay/v1)
// under Doppler. It makes one tiny request that lists models.
func TestSandboxFacingRelayContract(t *testing.T) {
	if os.Getenv("CODEGYM_RUN_RELAY_LIVE") != "1" {
		t.Skip("set CODEGYM_RUN_RELAY_LIVE=1 and CODEGYM_AGENT_RELAY_URL to check the deployed relay")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("CODEGYM_AGENT_RELAY_URL")), "/")
	if baseURL == "" {
		t.Skip("set CODEGYM_AGENT_RELAY_URL to the public relay base URL")
	}
	secret := strings.TrimSpace(os.Getenv("CODEGYM_RELAY_TOKEN_SECRET"))
	if secret == "" {
		t.Skip("set CODEGYM_RELAY_TOKEN_SECRET via Doppler to issue a token")
	}
	service, err := NewService(NewInMemoryStore(), ServiceConfig{
		TokenSecret:             secret,
		TokenTTL:                2 * time.Minute,
		DefaultMaxTotalTokens:   1_000,
		DefaultMaxCostUSDMicros: 1_000,
		DefaultMaxWallClock:     2 * time.Minute,
		OperationChecker:        allowEveryOperation{},
	})
	if err != nil {
		t.Fatalf("build service: %v", err)
	}
	issued, err := service.Issue(t.Context(), IssueInput{
		OperationID: "relay-contract-check",
		WorkspaceID: "relay-contract-check",
		UserID:      "relay-contract-check",
	})
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, baseURL+"/models", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+issued.Token)
	response, err := (&http.Client{Timeout: 60 * time.Second}).Do(request)
	if err != nil {
		t.Fatalf("call relay: %v", err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	if response.StatusCode != http.StatusOK {
		t.Fatalf("deployed relay rejected a locally issued token: HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	if !strings.Contains(string(body), "data") {
		t.Fatalf("unexpected models payload: %s", strings.TrimSpace(string(body)))
	}
}

// allowEveryOperation reports every operation active; the contract check
// only proves token acceptance, not budget enforcement.
type allowEveryOperation struct{}

func (allowEveryOperation) OperationActive(context.Context, string, string, string) (bool, error) {
	return true, nil
}
