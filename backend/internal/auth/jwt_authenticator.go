package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/auth0/go-jwt-middleware/v3/jwks"
	auth0validator "github.com/auth0/go-jwt-middleware/v3/validator"
)

type Auth0AuthenticatorConfig struct {
	Domain           string
	IssuerURL        string
	Audience         string
	AllowedClockSkew time.Duration
	HTTPClient       *http.Client
}

type tokenValidator interface {
	ValidateToken(ctx context.Context, tokenString string) (any, error)
}

type Auth0Authenticator struct {
	validator tokenValidator
}

func NewAuth0Authenticator(config Auth0AuthenticatorConfig) (*Auth0Authenticator, error) {
	if strings.TrimSpace(config.Audience) == "" {
		return nil, fmt.Errorf("auth0 audience is required")
	}

	issuerURL, err := auth0IssuerURL(config)
	if err != nil {
		return nil, err
	}

	providerOptions := []any{jwks.WithIssuerURL(issuerURL)}
	if config.HTTPClient != nil {
		providerOptions = append(providerOptions, jwks.WithCustomClient(config.HTTPClient))
	}

	provider, err := jwks.NewCachingProvider(providerOptions...)
	if err != nil {
		return nil, fmt.Errorf("create auth0 jwks provider: %w", err)
	}

	options := []auth0validator.Option{
		auth0validator.WithKeyFunc(provider.KeyFunc),
		auth0validator.WithAlgorithm(auth0validator.RS256),
		auth0validator.WithIssuer(issuerURL.String()),
		auth0validator.WithAudience(strings.TrimSpace(config.Audience)),
		auth0validator.WithCustomClaims(func() *auth0ProfileClaims {
			return &auth0ProfileClaims{}
		}),
	}
	if config.AllowedClockSkew > 0 {
		options = append(options, auth0validator.WithAllowedClockSkew(config.AllowedClockSkew))
	}

	validator, err := auth0validator.New(options...)
	if err != nil {
		return nil, fmt.Errorf("create auth0 jwt validator: %w", err)
	}

	return newAuth0Authenticator(validator, config), nil
}

func newAuth0Authenticator(validator tokenValidator, config Auth0AuthenticatorConfig) *Auth0Authenticator {
	return &Auth0Authenticator{
		validator: validator,
	}
}

func (a *Auth0Authenticator) Authenticate(ctx context.Context, bearerToken string) (Principal, error) {
	token := strings.TrimSpace(bearerToken)
	if token == "" {
		return Principal{}, ErrUnauthenticated
	}

	claims, err := a.validator.ValidateToken(ctx, token)
	if err != nil {
		return Principal{}, fmt.Errorf("%w: invalid auth0 token: %v", ErrUnauthenticated, err)
	}

	validatedClaims, ok := claims.(*auth0validator.ValidatedClaims)
	if !ok || validatedClaims == nil {
		return Principal{}, fmt.Errorf("%w: unexpected auth0 claims", ErrUnauthenticated)
	}

	userID := strings.TrimSpace(validatedClaims.RegisteredClaims.Subject)
	if userID == "" {
		return Principal{}, fmt.Errorf("%w: missing subject", ErrUnauthenticated)
	}

	defaultTenantID := personalTenantIDForSubject(userID)

	return Principal{
		UserID:          userID,
		DefaultTenantID: defaultTenantID,
		TenantIDs:       []string{defaultTenantID},
		UserMetadata:    userMetadataFromAuth0Claims(validatedClaims),
	}, nil
}

type auth0ProfileClaims struct {
	Email string `json:"email,omitempty"`
	Name  string `json:"name,omitempty"`
}

func (c *auth0ProfileClaims) Validate(context.Context) error {
	return nil
}

func userMetadataFromAuth0Claims(claims *auth0validator.ValidatedClaims) UserMetadata {
	profile, ok := claims.CustomClaims.(*auth0ProfileClaims)
	if !ok || profile == nil {
		return UserMetadata{}
	}

	return UserMetadata{
		Email:       strings.TrimSpace(profile.Email),
		DisplayName: strings.TrimSpace(profile.Name),
	}
}

func auth0IssuerURL(config Auth0AuthenticatorConfig) (*url.URL, error) {
	rawIssuer := strings.TrimSpace(config.IssuerURL)
	if rawIssuer == "" {
		domain := strings.TrimSpace(config.Domain)
		if domain == "" {
			return nil, fmt.Errorf("auth0 domain or issuer url is required")
		}
		if strings.Contains(domain, "://") {
			rawIssuer = domain
		} else {
			rawIssuer = "https://" + domain
		}
	}

	if !strings.HasSuffix(rawIssuer, "/") {
		rawIssuer += "/"
	}

	issuerURL, err := url.Parse(rawIssuer)
	if err != nil {
		return nil, fmt.Errorf("parse auth0 issuer url: %w", err)
	}
	if issuerURL.Scheme != "https" || issuerURL.Host == "" {
		return nil, fmt.Errorf("auth0 issuer url must be an https url")
	}
	return issuerURL, nil
}

func personalTenantIDForSubject(subject string) string {
	var builder strings.Builder
	builder.Grow(len(subject))
	lastDash := false

	for _, r := range strings.ToLower(subject) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}

	slug := strings.Trim(builder.String(), "-")
	if slug == "" {
		slug = "auth0-user"
	}
	if len(slug) > 80 {
		slug = slug[:80]
		slug = strings.TrimRight(slug, "-")
	}

	return "personal-" + slug
}
