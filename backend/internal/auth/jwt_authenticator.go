package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"
)

// JWTAuthenticator validates a signed bearer token (a JWT) and maps its claims
// to a Principal. It implements the Authenticator interface, so it drops into
// auth.Middleware in place of DevAuthenticator (see cmd/server/main.go and
// docs/auth-identity-tenant.md → "Expected Real Auth Upgrade Path").
//
// ─────────────────────────────────────────────────────────────────────────────
// LEARNING GOAL
// ─────────────────────────────────────────────────────────────────────────────
// Understand how a *stateless* token proves identity with no database lookup.
//
// A JWT is three base64url segments joined by dots:
//
//	header . payload . signature
//
//	header    -> {"alg":"HS256","typ":"JWT"}
//	payload   -> claims, e.g. {"sub":"user_123","tenants":["t_1"],"exp":1750000000}
//	signature -> HMAC_SHA256( base64url(header) + "." + base64url(payload), secret )
//
// Verifying a token = recompute the signature with *your* secret and compare it
// to the token's signature with a constant-time comparison (hmac.Equal). If they
// match, the payload was issued by someone holding the secret and was not
// altered in transit. Then you still must check the token has not expired.
//
// NOTE: We hand-roll HS256 here so you can *see* the mechanics. In production you
// would use a vetted library (github.com/golang-jwt/jwt/v5) and usually an
// asymmetric algorithm (RS256/ES256) so verifiers never hold the signing key.
// Do not ship hand-rolled token crypto to real users.

// jwtClaims is the subset of standard + CodeGym claims we read.
// Registered claim names: https://www.rfc-editor.org/rfc/rfc7519#section-4.1
type jwtClaims struct {
	Subject         string   `json:"sub"`            // -> Principal.UserID
	DefaultTenantID string   `json:"default_tenant"` // -> Principal.DefaultTenantID
	TenantIDs       []string `json:"tenants"`        // -> Principal.TenantIDs
	ExpiresAt       int64    `json:"exp"`            // unix seconds; reject if past
	NotBefore       int64    `json:"nbf"`            // unix seconds; reject if in the future
}

type JWTAuthenticator struct {
	secret []byte
	now    func() time.Time
}

func NewJWTAuthenticator(secret string) *JWTAuthenticator {
	return &JWTAuthenticator{
		secret: []byte(secret),
		now:    time.Now,
	}
}

// Authenticate verifies the signature, checks expiry, and maps claims to a
// Principal. Return ErrUnauthenticated for any token that is missing, malformed,
// not signed by us, or expired. (Wrap it: fmt.Errorf("%w: ...", ErrUnauthenticated)
// so the middleware still renders a 401 — see auth/middleware.go.)
func (a *JWTAuthenticator) Authenticate(_ context.Context, bearerToken string) (Principal, error) {
	token := strings.TrimSpace(bearerToken)
	if token == "" {
		return Principal{}, ErrUnauthenticated
	}

	// ── STEP 1: split "header.payload.signature" ────────────────────────────
	// TODO(you): strings.SplitN(token, ".", 3) — there must be exactly 3 parts,
	// else return ErrUnauthenticated. Keep each segment; you need the exact bytes
	// of "header.payload" (parts[0]+"."+parts[1]) to re-sign in step 2.

	// ── STEP 2: verify the signature ────────────────────────────────────────
	// TODO(you): add `import "encoding/base64"`, then:
	//   expected := a.sign(parts[0] + "." + parts[1])
	//   got, err := base64.RawURLEncoding.DecodeString(parts[2])
	//   if err != nil || !hmac.Equal(expected, got) { return ErrUnauthenticated }
	// Why RawURLEncoding? JWT uses base64url WITHOUT '=' padding.

	// ── STEP 3: decode the claims ───────────────────────────────────────────
	// TODO(you): add `import "encoding/json"`, base64url-decode parts[1], then
	// json.Unmarshal into a jwtClaims. Malformed JSON -> ErrUnauthenticated.

	// ── STEP 4: check time-based validity ───────────────────────────────────
	// TODO(you): now := a.now().UTC().Unix()
	//   reject if claims.ExpiresAt != 0 && now >= claims.ExpiresAt
	//   reject if claims.NotBefore != 0 && now <  claims.NotBefore
	// An unexpired-but-tampered token never reaches here — step 2 caught it.

	// ── STEP 5: map claims -> Principal ─────────────────────────────────────
	// TODO(you): claims.Subject is required (empty -> ErrUnauthenticated).
	// If TenantIDs is empty but DefaultTenantID is set, seed TenantIDs with it
	// so tenant.Middleware can authorize the default tenant. Then return:
	//   Principal{UserID: ..., DefaultTenantID: ..., TenantIDs: ...}, nil

	return Principal{}, fmt.Errorf("%w: JWTAuthenticator.Authenticate not implemented yet", ErrUnauthenticated)
}

// sign computes the HS256 signature over the "header.payload" signing input.
// Provided so step 2 is a one-liner — read it to see exactly what bytes you are
// comparing the token's signature against.
func (a *JWTAuthenticator) sign(signingInput string) []byte {
	mac := hmac.New(sha256.New, a.secret)
	mac.Write([]byte(signingInput))
	return mac.Sum(nil)
}
