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
