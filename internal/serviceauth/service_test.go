package serviceauth

import (
	"crypto/ed25519"
	"errors"
	"testing"
	"time"
)

func TestServiceTokenVerification(t *testing.T) {
	now := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
	_, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := PublicKeyFromPrivate(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := NewSigner("active", map[string]ed25519.PrivateKey{"active": privateKey})
	if err != nil {
		t.Fatal(err)
	}
	signer.SetNow(func() time.Time { return now })
	verifier, err := NewVerifier(map[string]ed25519.PublicKey{"active": publicKey})
	if err != nil {
		t.Fatal(err)
	}
	verifier.SetNow(func() time.Time { return now.Add(time.Minute) })

	token, err := signer.Sign(Claims{
		Audience:  AudienceAgentRunner,
		Subject:   "server-1",
		Scopes:    []string{ScopeRunnerRun},
		ExpiresAt: now.Add(15 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	claims, err := verifier.Verify(token)
	if err != nil {
		t.Fatal(err)
	}
	if err := RequireScope(claims, AudienceAgentRunner, ScopeRunnerRun, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := RequireScope(claims, AudienceConnector, ScopeRunnerRun, now.Add(time.Minute)); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("wrong audience error = %v", err)
	}
	if err := RequireScope(claims, AudienceAgentRunner, ScopeRunnerCancel, now.Add(time.Minute)); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("wrong scope error = %v", err)
	}
	if _, err := verifier.Verify(""); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("missing token error = %v", err)
	}
}

func TestServiceTokenRejectsExpiredAndWrongSigner(t *testing.T) {
	now := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
	_, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := PublicKeyFromPrivate(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	_, otherPrivateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := NewSigner("active", map[string]ed25519.PrivateKey{"active": privateKey})
	if err != nil {
		t.Fatal(err)
	}
	signer.SetNow(func() time.Time { return now })
	verifier, err := NewVerifier(map[string]ed25519.PublicKey{"active": publicKey})
	if err != nil {
		t.Fatal(err)
	}
	verifier.SetNow(func() time.Time { return now.Add(2 * time.Minute) })

	expired, err := signer.Sign(Claims{
		Audience:  AudienceAgentRunner,
		Subject:   "server-1",
		Scopes:    []string{ScopeRunnerRun},
		ExpiresAt: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifier.Verify(expired); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expired token error = %v", err)
	}

	wrongSigner, err := NewSigner("active", map[string]ed25519.PrivateKey{"active": otherPrivateKey})
	if err != nil {
		t.Fatal(err)
	}
	wrongSigner.SetNow(func() time.Time { return now })
	token, err := wrongSigner.Sign(Claims{
		Audience:  AudienceAgentRunner,
		Subject:   "server-1",
		Scopes:    []string{ScopeRunnerRun},
		ExpiresAt: now.Add(15 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifier.Verify(token); err == nil {
		t.Fatal("wrong signer token was accepted")
	}
}
