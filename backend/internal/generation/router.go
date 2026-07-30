package generation

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
)

// NamedGenerator is a Generator with a stable provider id for routing.
type NamedGenerator struct {
	Name      string
	Generator Generator
}

// Router implements Generator by trying providers in priority order with
// fallback on provider/transport failures (not on semantic validation — that
// stays in the MCQ/notes repair loops against the same provider).
type Router struct {
	providers []NamedGenerator
}

// NewRouter builds a multi-provider Generator. Providers must be non-nil and
// uniquely named; empty names are rejected.
func NewRouter(providers []NamedGenerator) (*Router, error) {
	if len(providers) == 0 {
		return nil, errors.New("generation router requires at least one provider")
	}
	seen := make(map[string]bool, len(providers))
	cleaned := make([]NamedGenerator, 0, len(providers))
	for _, provider := range providers {
		name := strings.ToLower(strings.TrimSpace(provider.Name))
		if name == "" {
			return nil, errors.New("generation router provider name is required")
		}
		if provider.Generator == nil {
			return nil, fmt.Errorf("generation router provider %q has nil generator", name)
		}
		if seen[name] {
			return nil, fmt.Errorf("generation router duplicate provider %q", name)
		}
		seen[name] = true
		cleaned = append(cleaned, NamedGenerator{Name: name, Generator: provider.Generator})
	}
	return &Router{providers: cleaned}, nil
}

// ProviderNames returns registered provider ids in priority order.
func (r *Router) ProviderNames() []string {
	if r == nil {
		return nil
	}
	names := make([]string, 0, len(r.providers))
	for _, provider := range r.providers {
		names = append(names, provider.Name)
	}
	return names
}

// Generate tries eligible providers until one succeeds.
func (r *Router) Generate(ctx context.Context, request GenerateRequest) (GenerateResult, error) {
	if r == nil || len(r.providers) == 0 {
		return GenerateResult{}, errors.New("generation router has no providers")
	}

	eligible := r.selectProviders(request.ModelPolicy)
	if len(eligible) == 0 {
		return GenerateResult{}, &ProviderError{
			Message: "no eligible GenAI providers for this request",
		}
	}

	var lastErr error
	for index, provider := range eligible {
		if ctx.Err() != nil {
			return GenerateResult{}, ctx.Err()
		}

		log.Printf("genai route provider=%s attempt=%d/%d", provider.Name, index+1, len(eligible))
		result, err := provider.Generator.Generate(ctx, request)
		if err == nil {
			if strings.TrimSpace(result.Provider) == "" {
				result.Provider = provider.Name
			}
			if index > 0 {
				log.Printf("genai route succeeded after fallback provider=%s", provider.Name)
			}
			return result, nil
		}

		// Invalid model output is returned to the caller so validate/repair can
		// retry the same provider; do not burn other credit pools on it.
		var invalid *InvalidOutputError
		if errors.As(err, &invalid) {
			if strings.TrimSpace(result.Provider) == "" {
				// result is empty on error path; stamp provider on a synthetic return
			}
			log.Printf("genai route provider=%s invalid_output (no fallback): %s", provider.Name, DiagnosticMessage(err))
			return GenerateResult{}, err
		}

		lastErr = err
		log.Printf("genai route provider=%s failed class=%s detail=%s",
			provider.Name, DiagnosticClass(err), DiagnosticMessage(err))
		if index+1 < len(eligible) {
			log.Printf("genai route fallback next=%s", eligible[index+1].Name)
		}
	}

	if lastErr == nil {
		lastErr = errors.New("all GenAI providers failed")
	}
	return GenerateResult{}, &ProviderError{
		Message: fmt.Sprintf("all GenAI providers failed (%s)", strings.Join(r.namesOf(eligible), ", ")),
		Err:     lastErr,
	}
}

// Stream routes a conversational turn to the first eligible streaming-capable
// provider. A provider may be retried only before it emits any text; once a
// delta reaches the client, fallback would risk duplicating the answer.
func (r *Router) Stream(ctx context.Context, request StreamRequest, emit func(StreamDelta) error) (StreamResult, error) {
	if r == nil || len(r.providers) == 0 {
		return StreamResult{}, errors.New("generation router has no providers")
	}
	eligible := r.selectProviders(request.ModelPolicy)
	if len(eligible) == 0 {
		return StreamResult{}, &ProviderError{Message: "no eligible GenAI providers for this request"}
	}

	var lastErr error
	for index, provider := range eligible {
		streamer, ok := provider.Generator.(Streamer)
		if !ok {
			continue
		}
		emitted := false
		result, err := streamer.Stream(ctx, request, func(delta StreamDelta) error {
			if delta.Content != "" {
				emitted = true
			}
			return emit(delta)
		})
		if err == nil {
			if strings.TrimSpace(result.Provider) == "" {
				result.Provider = provider.Name
			}
			return result, nil
		}
		lastErr = err
		log.Printf("genai stream provider=%s failed class=%s detail=%s",
			provider.Name, DiagnosticClass(err), DiagnosticMessage(err))
		if emitted || ctx.Err() != nil {
			return StreamResult{}, err
		}
		if index+1 < len(eligible) {
			log.Printf("genai stream fallback next=%s", eligible[index+1].Name)
		}
	}
	if lastErr == nil {
		lastErr = errors.New("no configured provider supports streaming")
	}
	return StreamResult{}, &ProviderError{Message: "all streaming providers failed", Err: lastErr}
}

func (r *Router) selectProviders(policy ModelPolicy) []NamedGenerator {
	allowed := normalizeNameSet(policy.AllowedProviders)
	preferred := strings.ToLower(strings.TrimSpace(policy.PreferredProvider))

	byName := make(map[string]NamedGenerator, len(r.providers))
	for _, provider := range r.providers {
		byName[provider.Name] = provider
	}

	eligible := make([]NamedGenerator, 0, len(r.providers))
	appendIf := func(name string) {
		provider, ok := byName[name]
		if !ok {
			return
		}
		if len(allowed) > 0 && !allowed[name] {
			return
		}
		for _, existing := range eligible {
			if existing.Name == name {
				return
			}
		}
		eligible = append(eligible, provider)
	}

	if preferred != "" {
		appendIf(preferred)
	}
	for _, provider := range r.providers {
		appendIf(provider.Name)
	}
	return eligible
}

func normalizeNameSet(names []string) map[string]bool {
	if len(names) == 0 {
		return nil
	}
	out := make(map[string]bool, len(names))
	for _, name := range names {
		name = strings.ToLower(strings.TrimSpace(name))
		if name != "" {
			out[name] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (r *Router) namesOf(providers []NamedGenerator) []string {
	names := make([]string, 0, len(providers))
	for _, provider := range providers {
		names = append(names, provider.Name)
	}
	return names
}
