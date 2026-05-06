package serviceauth

import (
	"crypto/ed25519"
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Signer struct {
	activeKeyID string
	privateKey  ed25519.PrivateKey
	now         func() time.Time
}

func NewSigner(activeKeyID string, privateKeys map[string]ed25519.PrivateKey) (*Signer, error) {
	activeKeyID = strings.TrimSpace(activeKeyID)
	if activeKeyID == "" {
		return nil, errors.New("active key id is required")
	}
	privateKey := privateKeys[activeKeyID]
	if len(privateKey) != ed25519.PrivateKeySize {
		return nil, ErrInvalidKey
	}
	return &Signer{
		activeKeyID: activeKeyID,
		privateKey:  privateKey,
		now:         time.Now,
	}, nil
}

func NewSignerFromConfig(activeKeyID string, privateKeys map[string]string) (*Signer, error) {
	parsed := make(map[string]ed25519.PrivateKey, len(privateKeys))
	for keyID, raw := range privateKeys {
		privateKey, err := ParsePrivateKey(raw)
		if err != nil {
			return nil, err
		}
		parsed[keyID] = privateKey
	}
	return NewSigner(activeKeyID, parsed)
}

func (s *Signer) SetNow(now func() time.Time) {
	if now != nil {
		s.now = now
	}
}

func (s *Signer) Sign(claims Claims) (string, error) {
	if s == nil {
		return "", errors.New("service auth signer is not configured")
	}
	now := s.now().UTC()
	if claims.Issuer == "" {
		claims.Issuer = Issuer
	}
	if claims.IssuedAt.IsZero() {
		claims.IssuedAt = now
	}
	if claims.ExpiresAt.IsZero() {
		return "", errors.New("service token expires_at is required")
	}
	if !claims.ExpiresAt.After(claims.IssuedAt) {
		return "", errors.New("service token expires_at must be after issued_at")
	}
	if claims.ExpiresAt.Sub(claims.IssuedAt) > MaxServiceTokenTTL {
		return "", errors.New("service token ttl exceeds 15m")
	}
	if strings.TrimSpace(claims.Audience) == "" {
		return "", errors.New("service token audience is required")
	}
	if strings.TrimSpace(claims.Subject) == "" {
		return "", errors.New("service token subject is required")
	}
	if len(claims.Scopes) == 0 {
		return "", errors.New("service token scopes are required")
	}
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, jwtClaimsFromClaims(claims))
	token.Header["kid"] = s.activeKeyID
	return token.SignedString(s.privateKey)
}
