package handler

import (
	"crypto/rand"
	"fmt"

	"customer-support/internal/passwd"
	"customer-support/internal/repository"
)

const resetPasswordLen = 12

func randomResetPassword() (string, error) {
	const lower = "abcdefghijkmnpqrstuvwxyz"
	const upper = "ABCDEFGHJKLMNPQRSTUVWXYZ"
	const digits = "23456789"
	const symbols = "!@#$%&*"
	all := lower + upper + digits + symbols
	pick := func(set string) (byte, error) {
		var b [1]byte
		if _, err := rand.Read(b[:]); err != nil {
			return 0, err
		}
		return set[int(b[0])%len(set)], nil
	}
	out := make([]byte, resetPasswordLen)
	var err error
	if out[0], err = pick(lower); err != nil {
		return "", err
	}
	if out[1], err = pick(upper); err != nil {
		return "", err
	}
	if out[2], err = pick(digits); err != nil {
		return "", err
	}
	if out[3], err = pick(symbols); err != nil {
		return "", err
	}
	for i := 4; i < resetPasswordLen; i++ {
		if out[i], err = pick(all); err != nil {
			return "", err
		}
	}
	for i := len(out) - 1; i > 0; i-- {
		var b [1]byte
		if _, err := rand.Read(b[:]); err != nil {
			return "", err
		}
		j := int(b[0]) % (i + 1)
		out[i], out[j] = out[j], out[i]
	}
	return string(out), nil
}

// HashPassword bcrypt 해시를 만든다. 기존 SHA-256 단독 해시를 대체한다.
func HashPassword(pw string) string {
	return passwd.Hash(pw)
}

func passwordTooShort(pw string) bool {
	return passwd.TooShort(pw)
}

func passwordMinLenMsg() string {
	return fmt.Sprintf("비밀번호는 %d자 이상이어야 합니다.", passwd.MinLen)
}

func verifyPassword(hash, pw string) bool {
	return passwd.Verify(hash, pw)
}

func upgradeLegacyPassword(hash, pw string, save func(string) error) {
	if !passwd.NeedsRehash(hash) || save == nil {
		return
	}
	nh := passwd.Hash(pw)
	if nh == "" {
		return
	}
	_ = save(nh)
}

func verifyAndUpgradeSetting(repo *repository.SettingsRepo, key, pw string) bool {
	if repo == nil || pw == "" {
		return false
	}
	hash, _ := repo.Get(key)
	if !passwd.Verify(hash, pw) {
		return false
	}
	upgradeLegacyPassword(hash, pw, func(nh string) error {
		return repo.Set(key, nh)
	})
	return true
}
