package signature

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"hash"
)

// Sign returns the lowercase hex-encoded HMAC-SHA256 of message using key.
// Used to sign outgoing webhook payloads sent to merchants.
func Sign(key, message []byte) string {
	return signWith(sha256.New, key, message)
}

// Verify reports whether sig matches the HMAC-SHA256 of message.
// Uses constant-time comparison to prevent timing attacks.
func Verify(key, message []byte, sig string) bool {
	return hmac.Equal([]byte(Sign(key, message)), []byte(sig))
}

// SignSHA512 returns the lowercase hex-encoded HMAC-SHA512 of message.
// DOKU uses SHA512 for its webhook signature scheme.
func SignSHA512(key, message []byte) string {
	return signWith(sha512.New, key, message)
}

// VerifySHA512 reports whether sig matches the HMAC-SHA512 of message.
func VerifySHA512(key, message []byte, sig string) bool {
	return hmac.Equal([]byte(SignSHA512(key, message)), []byte(sig))
}

func signWith(newHash func() hash.Hash, key, message []byte) string {
	mac := hmac.New(newHash, key)
	mac.Write(message)
	return hex.EncodeToString(mac.Sum(nil))
}
