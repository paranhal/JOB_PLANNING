package handler

import (
	"testing"

	"customer-support/internal/model"
)

func TestApplyAssetBusinessRulesThirdPartySkipsProject(t *testing.T) {
	a := &model.Asset{
		ProductCategory: "rfid",
		InstallerType:   "other",
		IsManaged:       true,
	}
	applyAssetBusinessRules(a)
	if a.ManagementType != model.ManagementTypeThirdParty {
		t.Fatalf("관리유형: %q", a.ManagementType)
	}
	if a.IsManaged {
		t.Fatal("타사장비는 관리 대상이 아니어야 함")
	}
	if a.ProjectID != "" {
		t.Fatalf("타사장비는 사업에 자동 넣으면 안 됨: %q", a.ProjectID)
	}
}

func TestApplyAssetBusinessRulesRFIDHasNoDefaultProject(t *testing.T) {
	a := &model.Asset{ProductCategory: "rfid", InstallerType: "self"}
	applyAssetBusinessRules(a)
	if a.ProjectID != "" {
		t.Fatalf("RFID여도 사업을 자동으로 넣으면 안 됨: %q", a.ProjectID)
	}
	if a.ManagementType == model.ManagementTypeThirdParty {
		t.Fatal("자사 설치에 타사장비를 넣으면 안 됨")
	}
}

func TestApplyAssetBusinessRulesThirdPartyKeepsRelatedProject(t *testing.T) {
	a := &model.Asset{
		ProductCategory: "other",
		ManagementType:  model.ManagementTypeThirdParty,
		ProjectID:       "WPSEED01",
		IsManaged:       true,
	}
	applyAssetBusinessRules(a)
	if a.ProjectID != "WPSEED01" {
		t.Fatalf("연관 사업 선택은 유지: %q", a.ProjectID)
	}
	if a.IsManaged {
		t.Fatal("타사장비는 관리 대상이 아니어야 함")
	}
}
