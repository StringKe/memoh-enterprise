package serviceauth

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"strings"
)

func ParsePrivateKey(raw string) (ed25519.PrivateKey, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, ErrInvalidKey
	}
	if block, _ := pem.Decode([]byte(value)); block != nil {
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		privateKey, ok := key.(ed25519.PrivateKey)
		if !ok {
			return nil, ErrInvalidKey
		}
		return privateKey, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	switch len(decoded) {
	case ed25519.PrivateKeySize:
		return ed25519.PrivateKey(decoded), nil
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(decoded), nil
	default:
		return nil, ErrInvalidKey
	}
}

func ParsePublicKey(raw string) (ed25519.PublicKey, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, ErrInvalidKey
	}
	if block, _ := pem.Decode([]byte(value)); block != nil {
		key, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		publicKey, ok := key.(ed25519.PublicKey)
		if !ok {
			return nil, ErrInvalidKey
		}
		return publicKey, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	if len(decoded) != ed25519.PublicKeySize {
		return nil, ErrInvalidKey
	}
	return ed25519.PublicKey(decoded), nil
}

func PublicKeyFromPrivate(privateKey ed25519.PrivateKey) (ed25519.PublicKey, error) {
	if len(privateKey) != ed25519.PrivateKeySize {
		return nil, ErrInvalidKey
	}
	publicKey, ok := privateKey.Public().(ed25519.PublicKey)
	if !ok {
		return nil, ErrInvalidKey
	}
	return publicKey, nil
}

func KeyID(publicKey ed25519.PublicKey) string {
	sum := sha256.Sum256(publicKey)
	return hex.EncodeToString(sum[:8])
}
