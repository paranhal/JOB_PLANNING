package handler

import (
	"fmt"

	"customer-support/internal/passwd"
	"customer-support/internal/repository"
)

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
