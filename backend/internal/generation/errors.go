package generation

import (
	"errors"
	"fmt"
	"strings"
)

const diagnosticMaxChars = 300

// ProviderError marks failures returned by or while calling the configured
// model provider. It is safe for logs: callers should pass sanitized provider
// messages only and never include request headers or API keys.
type ProviderError struct {
	StatusCode int
	Message    string
	Err        error
}

func (e *ProviderError) Error() string {
	message := strings.TrimSpace(e.Message)
	if message == "" && e.Err != nil {
		message = e.Err.Error()
	}
	if message == "" {
		message = "provider call failed"
	}
	if e.StatusCode > 0 {
		return fmt.Sprintf("provider_error: status=%d message=%q", e.StatusCode, TruncateDiagnostic(message))
	}
	return fmt.Sprintf("provider_error: message=%q", TruncateDiagnostic(message))
}

func (e *ProviderError) Unwrap() error {
	return e.Err
}

// InvalidOutputError marks model output that arrived successfully but could not
// be parsed or validated into the requested schema.
type InvalidOutputError struct {
	Reason    string
	RawOutput string
	Err       error
}

func (e *InvalidOutputError) Error() string {
	reason := strings.TrimSpace(e.Reason)
	if reason == "" && e.Err != nil {
		reason = e.Err.Error()
	}
	if reason == "" {
		reason = "model output failed validation"
	}
	if strings.TrimSpace(e.RawOutput) == "" {
		return fmt.Sprintf("invalid_output: reason=%q", TruncateDiagnostic(reason))
	}
	return fmt.Sprintf("invalid_output: reason=%q raw_output=%q", TruncateDiagnostic(reason), TruncateDiagnostic(e.RawOutput))
}

func (e *InvalidOutputError) Unwrap() error {
	return e.Err
}

func DiagnosticClass(err error) string {
	var providerErr *ProviderError
	if errors.As(err, &providerErr) {
		return "provider_error"
	}
	var invalidErr *InvalidOutputError
	if errors.As(err, &invalidErr) {
		return "invalid_output"
	}
	return "unknown"
}

func DiagnosticMessage(err error) string {
	var providerErr *ProviderError
	if errors.As(err, &providerErr) {
		message := providerErr.Message
		if message == "" && providerErr.Err != nil {
			message = providerErr.Err.Error()
		}
		if providerErr.StatusCode > 0 {
			return fmt.Sprintf("status=%d message=%q", providerErr.StatusCode, TruncateDiagnostic(message))
		}
		return fmt.Sprintf("message=%q", TruncateDiagnostic(message))
	}

	var invalidErr *InvalidOutputError
	if errors.As(err, &invalidErr) {
		reason := invalidErr.Reason
		if reason == "" && invalidErr.Err != nil {
			reason = invalidErr.Err.Error()
		}
		if invalidErr.RawOutput != "" {
			return fmt.Sprintf("reason=%q raw_output=%q", TruncateDiagnostic(reason), TruncateDiagnostic(invalidErr.RawOutput))
		}
		return fmt.Sprintf("reason=%q", TruncateDiagnostic(reason))
	}

	if err == nil {
		return ""
	}
	return TruncateDiagnostic(err.Error())
}

func TruncateDiagnostic(value string) string {
	return truncateDiagnostic(value, diagnosticMaxChars)
}

func truncateDiagnostic(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) <= max {
		return value
	}
	if max <= 0 {
		return ""
	}
	return value[:max] + "..."
}
