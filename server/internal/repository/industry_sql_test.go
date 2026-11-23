package repository

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func TestIndustryContainsSQL95_219(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "ind.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows := map[string]string{
		"c1": "도서관", "c2": "학교", "c3": "도서관,학교", "c4": "", "c5": "public", "c6": "공공기관", "c7": "학교/도서관",
		"c8": "민간", "c9": "교육청,학교", "c10": "School", "c11": "도서관, 학교 ",
	}
	for id, ind := range rows {
		if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, industry) VALUES (?,?,?,1,?)`, id, id, id, ind); err != nil {
			t.Fatal(err)
		}
	}
	ids := func(key string) string {
		clause, args := IndustryContainsSQL("c.industry", key)
		rs, err := db.Query(`SELECT c.customer_id FROM customers c WHERE `+clause, args...)
		if err != nil {
			t.Fatal(err)
		}
		defer rs.Close()
		var out []string
		for rs.Next() {
			var id string
			_ = rs.Scan(&id)
			out = append(out, id)
		}
		sort.Strings(out)
		return strings.Join(out, ",")
	}
	for _, c := range []struct{ key, want string }{
		{"도서관", "c1,c11,c3,c7"},       // 단일·다중·슬래시·공백
		{"library", "c1,c11,c3,c7"},   // 별칭 키
		{"학교", "c10,c11,c2,c3,c7,c9"}, // School(별칭 대소문자)도 포함
		{"공공기관", "c5,c6"},             // 별칭 public 포함
		{"공공", ""},                    // 부분 일치 안 함
		{"민간", "c8"},
		{"교육청", "c9"},                     // 목록 밖 값도 같은 규칙
		{model.IndustryUnspecified, "c4"}, // 미지정 조건 불변
		{"", "c4"},
	} {
		if got := ids(c.key); got != c.want {
			t.Errorf("IndustryContainsSQL(%q) → %q want %q", c.key, got, c.want)
		}
	}
	// 단일 업종 데이터의 결과는 종전 industryMatchSQL(IN) 과 같다(골든)
	for _, key := range []string{"도서관", "학교", "공공기관", "민간"} {
		single := map[string]bool{}
		for id, ind := range rows {
			if len(model.ParseIndustries(ind)) == 1 {
				single[id] = true
			}
		}
		old, oargs := industryMatchSQL("c", key)
		rs, _ := db.Query(`SELECT c.customer_id FROM customers c WHERE `+old, oargs...)
		var oldIDs []string
		for rs.Next() {
			var id string
			_ = rs.Scan(&id)
			oldIDs = append(oldIDs, id)
		}
		rs.Close()
		clause, args := IndustryContainsSQL("c.industry", key)
		rs2, _ := db.Query(`SELECT c.customer_id FROM customers c WHERE `+clause, args...)
		newSet := map[string]bool{}
		for rs2.Next() {
			var id string
			_ = rs2.Scan(&id)
			newSet[id] = true
		}
		rs2.Close()
		for _, id := range oldIDs {
			if !newSet[id] {
				t.Errorf("%s: 종전 결과 %s 가 새 조건에서 빠졌다", key, id)
			}
		}
	}
}
