package handler

import (
	"testing"

	"customer-support/internal/model"
)

func TestProductKeysForOrgLibraryOnly(t *testing.T) {
	if len(productKeysForOrg(model.OrgIDLibrary)) != 3 {
		t.Fatal("O01 제품 키")
	}
	if productKeysForOrg("O02") != nil || productKeysForOrg("O03") != nil {
		t.Fatal("O02/O03 은 제품 키 없음")
	}
}
