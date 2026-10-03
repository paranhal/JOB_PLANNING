package repository

import (
	"fmt"
	"strings"

	"customer-support/internal/passwd"
)

const SettingVisionAdminMustChange = "vision_admin_password_must_change"

// ApplyVisionAdminPassword 환경변수로 비젼관리자 확인 비밀번호를 넣는다. 평문은 로그에 남기지 않는다. §55.3
func ApplyVisionAdminPassword(r *SettingsRepo, initPW, resetPW string, hashFn func(string) string) (string, error) {
	if r == nil || hashFn == nil {
		return "", fmt.Errorf("설정 저장소를 쓸 수 없습니다")
	}
	initPW = strings.TrimSpace(initPW)
	resetPW = strings.TrimSpace(resetPW)
	cur, err := r.Get(SettingVisionAdminPassword)
	if err != nil {
		return "", err
	}
	if resetPW != "" {
		if passwd.TooShort(resetPW) {
			return "", fmt.Errorf("비젼관리자 초기화 비밀번호가 너무 짧습니다")
		}
		h := hashFn(resetPW)
		if h == "" {
			return "", fmt.Errorf("비젼관리자 비밀번호 해시를 만들지 못했습니다")
		}
		if err := r.Set(SettingVisionAdminPassword, h); err != nil {
			return "", err
		}
		if err := r.Set(SettingVisionAdminMustChange, "1"); err != nil {
			return "", err
		}
		return "비젼관리자 비밀번호를 환경변수로 초기화했습니다. 로그인 후 새 비밀번호를 정하세요.", nil
	}
	if strings.TrimSpace(cur) != "" {
		return "", nil
	}
	if initPW == "" {
		return "", nil
	}
	if passwd.TooShort(initPW) {
		return "", fmt.Errorf("비젼관리자 초기 비밀번호가 너무 짧습니다")
	}
	h := hashFn(initPW)
	if h == "" {
		return "", fmt.Errorf("비젼관리자 비밀번호 해시를 만들지 못했습니다")
	}
	if err := r.Set(SettingVisionAdminPassword, h); err != nil {
		return "", err
	}
	return "비젼관리자 비밀번호를 저장했습니다.", nil
}

func (r *SettingsRepo) VisionAdminMustChange() bool {
	if r == nil {
		return false
	}
	v, _ := r.Get(SettingVisionAdminMustChange)
	return strings.TrimSpace(v) == "1"
}

func (r *SettingsRepo) ClearVisionAdminMustChange() error {
	if r == nil {
		return nil
	}
	return r.Set(SettingVisionAdminMustChange, "")
}
