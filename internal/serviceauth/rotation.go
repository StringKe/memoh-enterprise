package serviceauth

import "crypto/ed25519"

func NewRotationVerifier(activeKeyID string, activePublicKey ed25519.PublicKey, previousPublicKeys map[string]ed25519.PublicKey) (*Verifier, error) {
	keys := make(map[string]ed25519.PublicKey, len(previousPublicKeys)+1)
	keys[activeKeyID] = activePublicKey
	for keyID, publicKey := range previousPublicKeys {
		keys[keyID] = publicKey
	}
	return NewVerifier(keys)
}
