package repository

import "testing"

func TestGuessProductLabel202(t *testing.T) {
	cases := []struct {
		name   string
		groups []string
		text   string
		want   string
	}{
		{"고객 자산이 한 제품군뿐이면 그 제품군", []string{"RFID자동화", "RFID자동화"}, "원인 모름", "RFID자동화"},
		{"한 제품군이 키워드보다 우선", []string{"자료관리"}, "무인반납기 오류", "자료관리"},
		{"KLAS 키워드", nil, "KLAS 접속이 안 됩니다", "KLAS"},
		{"KDevice 키워드", nil, "kdevice 프로그램 재설치", "KLAS"},
		{"자료관리 키워드", []string{"자료관리", "RFID자동화"}, "자료관리 화면 오류", "KLAS"},
		{"RFID 키워드", nil, "RFID 리더 인식 불량", "RFID"},
		{"무인반납·자가대출·스마트도서관", nil, "스마트도서관 자가대출 불가", "RFID"},
		{"둘 다 있으면 KLAS·RFID", []string{"자료관리", "RFID자동화"}, "KLAS 와 RFID 연동 오류", "KLAS·RFID"},
		{"대소문자·공백", nil, "  Klas   서버  ", "KLAS"},
		{"모르면 —", nil, "프린터 용지 걸림", "—"},
		{"빈 입력", nil, "", "—"},
		{"제품군 없음 라벨은 단일 제품군으로 치지 않는다", []string{}, "그냥 점검", "—"},
		// 95-204: 사전은 확정 전까지 그대로다. 아래 단어는 사용자 확인 전이라 추가하지 않았고(book_stats·opac·무인회원증 등) 지금은 「—」이다.
		{"청구기호출력기는 KLAS(확정)", nil, "청구기호출력기 인쇄 불량", "KLAS"},
		{"미확정 단어 book_stats", nil, "book_stats 집계 오류", "—"},
		{"미확정 단어 opac", nil, "OPAC 검색이 느림", "—"},
		{"미확정 단어 무인회원증", nil, "무인회원증 발급 오류", "—"},
	}
	for _, c := range cases {
		if got := GuessProductLabel(c.groups, c.text); got != c.want {
			t.Errorf("%s: %q (기대 %q)", c.name, got, c.want)
		}
	}
}

func TestMntLabels202(t *testing.T) {
	if MntProductLabel("앤로보틱스") != "RFID(앤로보틱스)" || MntProductLabel("KLAS") != "KLAS" || MntProductLabel("") != "—" {
		t.Fatal("제품 라벨")
	}
	if got := MntVisitTitle("성환도서관", "앤로보틱스", 3); got != "성환도서관 RFID(앤로보틱스) 정기점검 3회" {
		t.Fatalf("작업명 %q", got)
	}
	if got := MntVisitTitle("성환도서관", "", 0); got != "성환도서관 정기점검 1회" {
		t.Fatalf("제품 없는 작업명 %q", got)
	}
}

// 95-204 T1: 일괄 연결 대상 판정(후보 자산 수·제품 불일치).
func TestLinkEligibility204(t *testing.T) {
	one := func(group string) AssetCandidates {
		return AssetCandidates{N: 1, OneAssetID: "A1", OneAssetName: "장비", OneGroup: group}
	}
	cases := []struct {
		name string
		c    AssetCandidates
		pt   string
		mnt  bool
		want string
	}{
		{"자산 0개", AssetCandidates{}, "", false, LinkSkipNoAsset},
		{"자산 2개 이상", AssetCandidates{N: 2}, "", false, LinkSkipMulti},
		{"AS 자산 1개", one("자료관리"), "", false, ""},
		{"AS 는 제품 불일치 규칙 없음", one("RFID자동화"), "KLAS", false, ""},
		{"점검 KLAS ↔ 자료관리", one("자료관리"), "KLAS", true, ""},
		{"점검 앤로보틱스 ↔ RFID자동화", one("RFID자동화"), "앤로보틱스", true, ""},
		{"점검 KLAS ↔ RFID자동화 불일치", one("RFID자동화"), "KLAS", true, LinkSkipMismatch},
		{"점검 앤로보틱스 ↔ 자료관리 불일치", one("자료관리"), "앤로보틱스", true, LinkSkipMismatch},
		{"점검 제품 없음은 막지 않는다", one("자료관리"), "", true, ""},
		{"다른 제품군은 막지 않는다", one("전산장비"), "KLAS", true, ""},
		{"제품군 없는 자산은 막지 않는다", one(""), "앤로보틱스", true, ""},
	}
	for _, c := range cases {
		if got := LinkEligibility(c.c, c.pt, c.mnt); got != c.want {
			t.Errorf("%s: %q (기대 %q)", c.name, got, c.want)
		}
	}
}

func TestNormalizeUnlinkedSort204(t *testing.T) {
	for _, c := range []struct{ tab, in, want string }{
		{"as", "recv_asc", "recv_asc"}, {"as", "recv_desc", ""}, {"as", "", ""}, {"as", "visit_asc", ""},
		{"as", "customer;DROP", ""}, {"mnt", "visit_asc", "visit_asc"}, {"mnt", "visit_desc", ""}, {"mnt", "recv_asc", ""},
		{"mnt", " round ", "round"}, {"mnt", "state", "state"}, {"as", "state", ""}, {"mnt", "cand", "cand"},
	} {
		if got := NormalizeUnlinkedSort(c.tab, c.in); got != c.want {
			t.Errorf("%s %q → %q (기대 %q)", c.tab, c.in, got, c.want)
		}
	}
}
