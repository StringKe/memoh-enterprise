package serviceauth

import (
	"crypto/ed25519"
	"errors"
	"testing"
	"time"
)

func TestRotationVerifierAcceptsActiveAndPreviousKeys(t *testing.T) {
	now := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
	_, activePrivate, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	activePublic, err := PublicKeyFromPrivate(activePrivate)
	if err != nil {
		t.Fatal(err)
	}
	_, previousPrivate, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	previousPublic, err := PublicKeyFromPrivate(previousPrivate)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewRotationVerifier("active", activePublic, map[string]ed25519.PublicKey{"previous": previousPublic})
	if err != nil {
		t.Fatal(err)
	}
	verifier.SetNow(func() time.Time { return now })

	activeSigner, err := NewSigner("active", map[string]ed25519.PrivateKey{"active": activePrivate})
	if err != nil {
		t.Fatal(err)
	}
	activeSigner.SetNow(func() time.Time { return now })
	previousSigner, err := NewSigner("previous", map[string]ed25519.PrivateKey{"previous": previousPrivate})
	if err != nil {
		t.Fatal(err)
	}
	previousSigner.SetNow(func() time.Time { return now })

	for name, signer := range map[string]*Signer{"active": activeSigner, "previous": previousSigner} {
		token, err := signer.Sign(Claims{
			Audience:  AudienceServer,
			Subject:   name,
			Scopes:    []string{ScopeServerContext},
			ExpiresAt: now.Add(time.Minute),
		})
		if err != nil {
			t.Fatal(err)
		}
		claims, err := verifier.Verify(token)
		if err != nil {
			t.Fatalf("%s verify: %v", name, err)
		}
		if claims.KeyID != name {
			t.Fatalf("claims.KeyID = %q, want %q", claims.KeyID, name)
		}
	}
}

func TestRotationVerifierRejectsUnknownKid(t *testing.T) {
	now := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
	_, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	_, verifierPrivate, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	verifierPublic, err := PublicKeyFromPrivate(verifierPrivate)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := NewSigner("unknown", map[string]ed25519.PrivateKey{"unknown": privateKey})
	if err != nil {
		t.Fatal(err)
	}
	signer.SetNow(func() time.Time { return now })
	verifier, err := NewVerifier(map[string]ed25519.PublicKey{"active": verifierPublic})
	if err != nil {
		t.Fatal(err)
	}
	verifier.SetNow(func() time.Time { return now })
	token, err := signer.Sign(Claims{
		Audience:  AudienceServer,
		Subject:   "server-1",
		Scopes:    []string{ScopeServerContext},
		ExpiresAt: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifier.Verify(token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("unknown kid error = %v", err)
	}
}
