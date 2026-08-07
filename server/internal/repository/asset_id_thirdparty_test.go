package repository

import (
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// 타사 장비는 모델명이 없고 제품명이 한글인 경우가 많다.
// 자산번호는 URL·파일경로에 그대로 쓰이므로 ASCII로 유지돼야 한다.
func TestNormalizeModelCodeASCIIOnly(t *testing.T) {
	cases := []struct {
		name           string
		model, product string
		manufacturer   string
		want           string
	}{
		{"자사 모델코드는 그대로", "EZ-200E2", "무인대출반납기", "앤로보틱스", "EZ-200E2"},
		{"모델명 없으면 제품명 ASCII", "", "CI-8000H 무인민원발급기", "타사", "CI-8000H"},
		{"한글만 있으면 제조사 ASCII", "", "자료검색대", "LG CNS", "LG-CNS"},
		{"전부 한글이면 ETC", "", "자료검색대", "타사제조", "ETC"},
		{"빈 값이면 ETC", "", "", "", "ETC"},
	}

	for _, tc := range cases {
		got := NormalizeModelCode(tc.model, tc.product, tc.manufacturer)
		if got != tc.want {
			t.Errorf("%s: NormalizeModelCode(%q,%q,%q) = %q, want %q",
				tc.name, tc.model, tc.product, tc.manufacturer, got, tc.want)
		}
		for _, r := range got {
			if r > 127 {
				t.Errorf("%s: 결과에 비ASCII 문자 포함: %q", tc.name, got)
				break
			}
		}
	}
}

// 긴 이름을 바이트 단위로 자르면 UTF-8이 깨진다.
func TestNormalizeModelCodeLongNameStaysValid(t *testing.T) {
	long := strings.Repeat("가나다라", 20) + "ABC"
	got := NormalizeModelCode("", long, "")
	if !utf8.ValidString(got) {
		t.Fatalf("UTF-8이 깨졌습니다: %q", got)
	}
}

// 한글 자산번호로 이미 등록된 자산은 참조(AS 접수 등)까지 함께 이관돼야 한다.
func TestMigrateAssetIDsToASCII(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	const oldID = "A자료검색대3-001"
	if _, err := db.Exec(
		`INSERT INTO customers (customer_id, org_name, official_name) VALUES (?,?,?)`,
		"C041-25-001", "테스트도서관", "테스트도서관"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO assets (asset_id, customer_id, product_name, product_type, model_name, manufacturer)
		 VALUES (?,?,?,?,?,?)`,
		oldID, "C041-25-001", "자료검색대", "hw", "", "채움씨앤아이"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO as_receipts (as_id, as_number, customer_id, asset_id, status)
		 VALUES (?,?,?,?,?)`,
		"R2508-001", "R2508-001", "C041-25-001", oldID, "received"); err != nil {
		t.Fatal(err)
	}
	db.Exec(`DELETE FROM id_sequences WHERE seq_key=?`, assetIDASCIIMetaKey)

	if err := migrateAssetIDsToASCII(db); err != nil {
		t.Fatal(err)
	}

	var newID string
	if err := db.QueryRow(`SELECT asset_id FROM assets WHERE product_name='자료검색대'`).Scan(&newID); err != nil {
		t.Fatal(err)
	}
	if newID == oldID {
		t.Fatalf("자산번호가 이관되지 않았습니다: %q", newID)
	}
	for _, r := range newID {
		if r > 127 {
			t.Fatalf("이관 후에도 비ASCII 문자가 남았습니다: %q", newID)
		}
	}

	var refID string
	if err := db.QueryRow(`SELECT asset_id FROM as_receipts WHERE as_id='R2508-001'`).Scan(&refID); err != nil {
		t.Fatal(err)
	}
	if refID != newID {
		t.Fatalf("AS 접수의 자산번호 = %q, want %q", refID, newID)
	}
}

// 자산번호가 달라도 이미지 폴더명이 같아지면 사진이 서로 덮어써진다.
func TestSanitizeAssetIDForPathNoCollision(t *testing.T) {
	a := SanitizeAssetIDForPath("A자료검색대3-001")
	b := SanitizeAssetIDForPath("A로봇3-001")
	if a == b {
		t.Fatalf("서로 다른 자산이 같은 경로로 정규화됨: %q", a)
	}

	// 기존 ASCII 자산번호는 경로가 바뀌면 안 된다.
	if got := SanitizeAssetIDForPath("AEZ-2320HSC3-008"); got != "AEZ-2320HSC3-008" {
		t.Fatalf("ASCII 자산번호 경로가 변경됨: %q", got)
	}
}
