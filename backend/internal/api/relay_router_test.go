package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/agentrelay"
)

type relayOperationChecker struct{}

func (relayOperationChecker) OperationActive(context.Context, string, string, string) (bool, error) {
	return true, nil
}

type relayRouterTransport struct{}

func (relayRouterTransport) RelayChatCompletion(context.Context, []byte, bool) (*http.Response, error) {
	panic("models route must not call chat completion upstream")
}
func (relayRouterTransport) RelayModel() string                          { return "provider-deployment" }
func (relayRouterTransport) RelayProvider() string                       { return "azure" }
func (relayRouterTransport) RedactProviderSecrets(payload []byte) []byte { return payload }

func TestAgentRelayRouteUsesOperationAuthOutsideProtectedChain(t *testing.T) {
	tokens, err := agentrelay.NewService(agentrelay.NewInMemoryStore(), agentrelay.ServiceConfig{
		TokenSecret: "router-test-secret-at-least-thirty-two-bytes",
		TokenTTL:    5 * time.Minute, DefaultMaxTotalTokens: 10_000,
		DefaultMaxCostUSDMicros: 1_000_000, DefaultMaxWallClock: 10 * time.Minute,
		OperationChecker: relayOperationChecker{},
	})
	if err != nil {
		t.Fatal(err)
	}
	issued, err := tokens.Issue(t.Context(), agentrelay.IssueInput{
		OperationID: "operation-a", WorkspaceID: "workspace-a", UserID: "user-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	relayHandler, err := agentrelay.NewHTTPHandler(tokens, relayRouterTransport{}, "codegym-agent")
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(Dependencies{AgentRelay: relayHandler})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/agent-relay/v1/models", nil)
	request.Header.Set("Authorization", "Bearer "+issued.Token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("relay models response = %d %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/agent-relay/v1/models", nil)
	request.Header.Set("Authorization", "Bearer dev:user-a:workspace-a")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("normal user token reached relay: %d %s", response.Code, response.Body.String())
	}
}
