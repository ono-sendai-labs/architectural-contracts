package artifactio

import (
	"crypto/sha256"
	"encoding/hex"
)

// digest returns the canonical digest of artifact bytes: the lowercase
// hexadecimal SHA-256 of data. That single spelling — lowercase hex — is the
// one this package produces and expects everywhere.
func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
