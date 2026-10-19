package account

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"strings"
)

const Iterations = 600000
const verifierPrefix = "pbkdf2-sha256$v1$600000$"

// A supported dummy verifier forces identical KDF work for an unknown address.
const DummyVerifier = verifierPrefix + "AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

func Random(n int) (string, error) {
	b := make([]byte, n)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func Digest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
func HashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, e := rand.Read(salt); e != nil {
		return "", e
	}
	key, e := pbkdf2.Key(sha256.New, password, salt, Iterations, 32)
	if e != nil {
		return "", e
	}
	return verifierPrefix + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(key), nil
}
func VerifyPassword(password, verifier string) bool {
	if !strings.HasPrefix(verifier, verifierPrefix) {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(verifier, verifierPrefix), "$")
	if len(parts) != 2 {
		return false
	}
	salt, e := base64.RawStdEncoding.Strict().DecodeString(parts[0])
	if e != nil || len(salt) != 16 {
		return false
	}
	want, e := base64.RawStdEncoding.Strict().DecodeString(parts[1])
	if e != nil || len(want) != 32 {
		return false
	}
	got, e := pbkdf2.Key(sha256.New, password, salt, Iterations, 32)
	return e == nil && subtle.ConstantTimeCompare(got, want) == 1
}
