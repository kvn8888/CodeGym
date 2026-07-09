package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSMiddlewareAllowsExactOrigin(t *testing.T) {
	handler := CORSMiddleware([]string{"https://code-gym-rho.vercel.app"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	request := httptest.NewRequest(http.MethodOptions, "/api/v1/generate", nil)
	request.Header.Set("Origin", "https://code-gym-rho.vercel.app")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", recorder.Code)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "https://code-gym-rho.vercel.app" {
		t.Fatalf("Access-Control-Allow-Origin = %q", got)
	}
	if got := recorder.Header().Get("Vary"); got != "Origin" {
		t.Fatalf("Vary = %q, want Origin", got)
	}
}

func TestCORSMiddlewareAllowsVercelPreviewWildcard(t *testing.T) {
	handler := CORSMiddleware([]string{"https://*-kvn8888s-projects.vercel.app"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	request := httptest.NewRequest(http.MethodOptions, "/api/v1/generate", nil)
	request.Header.Set("Origin", "https://code-bxvtwkxtn-kvn8888s-projects.vercel.app")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", recorder.Code)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "https://code-bxvtwkxtn-kvn8888s-projects.vercel.app" {
		t.Fatalf("Access-Control-Allow-Origin = %q", got)
	}
}

func TestCORSMiddlewareRejectsWildcardNearMisses(t *testing.T) {
	handler := CORSMiddleware([]string{"https://*-kvn8888s-projects.vercel.app"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, origin := range []string{
		"https://evil.vercel.app",
		"http://code-bxvtwkxtn-kvn8888s-projects.vercel.app",
		"https://kvn8888s-projects.vercel.app",
		"https://nested.code-bxvtwkxtn-kvn8888s-projects.vercel.app",
	} {
		t.Run(origin, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodOptions, "/api/v1/generate", nil)
			request.Header.Set("Origin", origin)
			recorder := httptest.NewRecorder()

			handler.ServeHTTP(recorder, request)

			if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "" {
				t.Fatalf("Access-Control-Allow-Origin = %q, want empty", got)
			}
		})
	}
}

func TestOriginAllowList(t *testing.T) {
	allowList := newOriginAllowList([]string{
		"https://code-gym-rho.vercel.app",
		"https://*-kvn8888s-projects.vercel.app",
	})

	cases := []struct {
		origin string
		want   bool
	}{
		{"https://code-gym-rho.vercel.app", true},
		{"https://code-bxvtwkxtn-kvn8888s-projects.vercel.app", true},
		{"https://evil.vercel.app", false},
		{"http://code-bxvtwkxtn-kvn8888s-projects.vercel.app", false},
		{"https://kvn8888s-projects.vercel.app", false},
		{"https://nested.code-bxvtwkxtn-kvn8888s-projects.vercel.app", false},
	}

	for _, testCase := range cases {
		t.Run(testCase.origin, func(t *testing.T) {
			if got := allowList.Allows(testCase.origin); got != testCase.want {
				t.Fatalf("Allows(%q) = %v, want %v", testCase.origin, got, testCase.want)
			}
		})
	}
}
