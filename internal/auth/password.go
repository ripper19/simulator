package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

// argon2 parameters (per OWASP guidance: m=64MiB, t=3, p=4).
const (
	argonMemory  = 64 * 1024
	argonTime    = 3
	argonThreads = 4
)

// HashPassword hashes a password using argon2id with a random salt, returning an
// encoded PHC-style string.
func HashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, 32)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash)), nil
}

// VerifyPassword reports whether password matches the encoded hash, reading the
// argon2 parameters from the PHC string so parameters can evolve over time.
func VerifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	memory, timeN, threads, ok := parseParams(parts[3])
	if !ok {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, timeN, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

func parseParams(s string) (memory uint32, timeN uint32, threads uint8, ok bool) {
	for _, kv := range strings.Split(s, ",") {
		k, v, found := strings.Cut(kv, "=")
		if !found {
			return 0, 0, 0, false
		}
		n, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			return 0, 0, 0, false
		}
		switch k {
		case "m":
			memory = uint32(n)
		case "t":
			timeN = uint32(n)
		case "p":
			threads = uint8(n)
		}
	}
	return memory, timeN, threads, memory > 0 && timeN > 0 && threads > 0
}
