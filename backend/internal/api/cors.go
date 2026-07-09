package api

import (
	"net/http"
	"net/url"
	"strings"
)

// CORSMiddleware handles browser preflight and applies explicit origin allow-list
// headers for local frontend development and trusted hosted frontend origins.
func CORSMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	allowList := newOriginAllowList(allowedOrigins)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && allowList.Allows(origin) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-CodeGym-Tenant-ID")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

type originAllowList struct {
	exact    map[string]struct{}
	wildcard []wildcardOrigin
}

type wildcardOrigin struct {
	scheme     string
	hostPrefix string
	hostSuffix string
}

func newOriginAllowList(allowedOrigins []string) originAllowList {
	allowList := originAllowList{
		exact: make(map[string]struct{}, len(allowedOrigins)),
	}
	for _, origin := range allowedOrigins {
		trimmed := strings.TrimSpace(origin)
		if trimmed == "" {
			continue
		}
		if wildcard, ok := parseWildcardOrigin(trimmed); ok {
			allowList.wildcard = append(allowList.wildcard, wildcard)
			continue
		}
		allowList.exact[trimmed] = struct{}{}
	}
	return allowList
}

func (l originAllowList) Allows(origin string) bool {
	if _, ok := l.exact[origin]; ok {
		return true
	}
	if len(l.wildcard) == 0 {
		return false
	}

	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	scheme := strings.ToLower(parsed.Scheme)
	for _, wildcard := range l.wildcard {
		if scheme == wildcard.scheme &&
			strings.HasPrefix(host, wildcard.hostPrefix) &&
			strings.HasSuffix(host, wildcard.hostSuffix) {
			wildcardPart := strings.TrimSuffix(strings.TrimPrefix(host, wildcard.hostPrefix), wildcard.hostSuffix)
			if wildcardPart != "" && !strings.Contains(wildcardPart, ".") {
				return true
			}
		}
	}
	return false
}

func parseWildcardOrigin(pattern string) (wildcardOrigin, bool) {
	parsed, err := url.Parse(pattern)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return wildcardOrigin{}, false
	}
	if parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return wildcardOrigin{}, false
	}

	host := strings.ToLower(parsed.Hostname())
	star := strings.Index(host, "*")
	if star < 0 {
		return wildcardOrigin{}, false
	}
	if strings.Count(host, "*") != 1 {
		return wildcardOrigin{}, false
	}
	if strings.Contains(host[:star], ".") {
		return wildcardOrigin{}, false
	}

	return wildcardOrigin{
		scheme:     strings.ToLower(parsed.Scheme),
		hostPrefix: host[:star],
		hostSuffix: host[star+1:],
	}, true
}
