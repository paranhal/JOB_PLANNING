package passwd

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"log"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const cost = bcrypt.DefaultCost

// Hash bcrypt 해시를 만든다. 솔트·키 스트레칭 포함.
func Hash(password string) string {
	b, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		log.Printf("bcrypt 해시 실패: %v", err)
		return ""
	}
	return string(b)
}

// Verify bcrypt 또는 구 SHA-256(hex) 해시를 검증한다.
func Verify(hash, password string) bool {
	hash = strings.TrimSpace(hash)
	if hash == "" || password == "" {
		return false
	}
	if IsBcrypt(hash) {
		return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
	}
	sum := sha256.Sum256([]byte(password))
	got := fmt.Sprintf("%x", sum)
	if len(hash) != len(got) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(strings.ToLower(hash)), []byte(got)) == 1
}

// IsBcrypt bcrypt 해시인지.
func IsBcrypt(hash string) bool {
	h := strings.TrimSpace(hash)
	return strings.HasPrefix(h, "$2a$") || strings.HasPrefix(h, "$2b$") || strings.HasPrefix(h, "$2y$")
}

// NeedsRehash 구 SHA-256이면 재해시 대상.
func NeedsRehash(hash string) bool {
	return strings.TrimSpace(hash) != "" && !IsBcrypt(hash)
}

// SHA256Legacy 구 저장 형식(테스트·마이그레이션 재현용).
func SHA256Legacy(password string) string {
	sum := sha256.Sum256([]byte(password))
	return fmt.Sprintf("%x", sum)
}
