package serviceauth

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type jwtClaims struct {
	Scopes                  []string `json:"scope,omitempty"`
	RunID                   string   `json:"run_id,omitempty"`
	LeaseVersion            int64    `json:"lease_version,omitempty"`
	WorkspaceID             string   `json:"workspace_id,omitempty"`
	WorkspaceExecutorTarget string   `json:"workspace_executor_target,omitempty"`
	jwt.RegisteredClaims
}

type Verifier struct {
	publicKeys map[string]ed25519.PublicKey
	now        func() time.Time
}

func NewVerifier(publicKeys map[string]ed25519.PublicKey) (*Verifier, error) {
	if len(publicKeys) == 0 {
		return nil, ErrInvalidKey
	}
	copied := make(map[string]ed25519.PublicKey, len(publicKeys))
	for keyID, publicKey := range publicKeys {
		if strings.TrimSpace(keyID) == "" || len(publicKey) != ed25519.PublicKeySize {
			return nil, ErrInvalidKey
		}
		copied[keyID] = publicKey
	}
	return &Verifier{
		publicKeys: copied,
		now:        time.Now,
	}, nil
}

func NewVerifierFromConfig(activeKeyID string, activePublicKey string, previousPublicKeys map[string]string) (*Verifier, error) {
	keys := make(map[string]ed25519.PublicKey, len(previousPublicKeys)+1)
	active, err := ParsePublicKey(activePublicKey)
	if err != nil {
		return nil, err
	}
	keys[strings.TrimSpace(activeKeyID)] = active
	for keyID, raw := range previousPublicKeys {
		publicKey, err := ParsePublicKey(raw)
		if err != nil {
			return nil, err
		}
		keys[strings.TrimSpace(keyID)] = publicKey
	}
	return NewVerifier(keys)
}

func (v *Verifier) SetNow(now func() time.Time) {
	if now != nil {
		v.now = now
	}
}

func (v *Verifier) Verify(tokenString string) (Claims, error) {
	if v == nil {
		return Claims{}, errors.New("service auth verifier is not configured")
	}
	if strings.TrimSpace(tokenString) == "" {
		return Claims{}, ErrUnauthenticated
	}
	claims := &jwtClaims{}
	token, err := jwt.ParseWithClaims(
		tokenString,
		claims,
		func(token *jwt.Token) (any, error) {
			if token.Method != jwt.SigningMethodEdDSA {
				return nil, ErrInvalidToken
			}
			keyID, _ := token.Header["kid"].(string)
			publicKey := v.publicKeys[keyID]
			if len(publicKey) != ed25519.PublicKeySize {
				return nil, ErrInvalidKey
			}
			return publicKey, nil
		},
		jwt.WithoutClaimsValidation(),
	)
	if err != nil {
		if errors.Is(err, ErrInvalidKey) {
			return Claims{}, ErrUnauthenticated
		}
		return Claims{}, fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}
	if !token.Valid {
		return Claims{}, ErrInvalidToken
	}
	keyID, _ := token.Header["kid"].(string)
	out := claimsToPublicClaims(keyID, claims)
	now := v.now().UTC()
	if out.Issuer != Issuer {
		return Claims{}, ErrUnauthenticated
	}
	if out.ExpiresAt.IsZero() || !now.Before(out.ExpiresAt) {
		return Claims{}, ErrUnauthenticated
	}
	if out.IssuedAt.IsZero() || out.ExpiresAt.Sub(out.IssuedAt) > MaxServiceTokenTTL {
		return Claims{}, ErrUnauthenticated
	}
	return out, nil
}

func jwtClaimsFromClaims(claims Claims) jwtClaims {
	return jwtClaims{
		Scopes:                  claims.Scopes,
		RunID:                   claims.RunID,
		LeaseVersion:            claims.LeaseVersion,
		WorkspaceID:             claims.WorkspaceID,
		WorkspaceExecutorTarget: claims.WorkspaceExecutorTarget,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    claims.Issuer,
			Subject:   claims.Subject,
			Audience:  jwt.ClaimStrings{claims.Audience},
			ExpiresAt: jwt.NewNumericDate(claims.ExpiresAt.UTC()),
			IssuedAt:  jwt.NewNumericDate(claims.IssuedAt.UTC()),
		},
	}
}

func claimsToPublicClaims(keyID string, claims *jwtClaims) Claims {
	out := Claims{
		KeyID:                   keyID,
		Issuer:                  claims.Issuer,
		Subject:                 claims.Subject,
		Scopes:                  append([]string(nil), claims.Scopes...),
		RunID:                   claims.RunID,
		LeaseVersion:            claims.LeaseVersion,
		WorkspaceID:             claims.WorkspaceID,
		WorkspaceExecutorTarget: claims.WorkspaceExecutorTarget,
	}
	if len(claims.Audience) > 0 {
		out.Audience = claims.Audience[0]
	}
	if claims.IssuedAt != nil {
		out.IssuedAt = claims.IssuedAt.Time
	}
	if claims.ExpiresAt != nil {
		out.ExpiresAt = claims.ExpiresAt.Time
	}
	return out
}
