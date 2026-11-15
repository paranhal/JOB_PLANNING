package repository

import (
	"strings"
	"testing"

	"customer-support/internal/model"
)

func TestFormatWorkDateRange95_205(t *testing.T) {
	cases := []struct {
		name  string
		dates []string
		want  string
	}{
		{"없음", nil, ""},
		{"빈 값만", []string{"", " "}, ""},
		{"단일", []string{"2026-11-09"}, "11/9"},
		{"같은 날 중복", []string{"2026-11-09", "2026-11-09"}, "11/9"},
		{"연속 3일", []string{"2026-11-11", "2026-11-09", "2026-11-10"}, "11/9~11/11"},
		{"금→월 주말 건너뜀", []string{"2026-11-06", "2026-11-09"}, "11/6~11/9"},
		{"금→토→월", []string{"2026-11-06", "2026-11-07", "2026-11-09"}, "11/6~11/9"},
		{"하루 건너뜀은 비연속", []string{"2026-11-09", "2026-11-11"}, "11/9·11/11"},
		{"월→수 평일 건너뜀", []string{"2026-11-09", "2026-11-11", "2026-11-12"}, "11/9·11/11·11/12"},
		{"4일 이상 비연속은 외 N일", []string{"2026-11-02", "2026-11-04", "2026-11-06", "2026-11-10", "2026-11-12"}, "11/2·11/4·11/6 외 2일"},
		{"월 경계", []string{"2026-10-30", "2026-11-02"}, "10/30~11/2"},
		{"연 경계", []string{"2026-12-31", "2027-01-01", "2027-01-04"}, "12/31~1/4"},
		{"시각이 붙은 날짜", []string{"2026-11-09 10:00:00", "2026-11-10"}, "11/9~11/10"},
		{"잘못된 날짜는 건너뜀", []string{"abc", "2026-11-09"}, "11/9"},
	}
	for _, c := range cases {
		if got := formatWorkDateRange(c.dates); got != c.want {
			t.Errorf("%s: %q want %q", c.name, got, c.want)
		}
	}
}

func lines205(ls []model.CompanyWeeklyAutoLine) []string {
	var out []string
	for _, l := range ls {
		out = append(out, l.Text)
	}
	return out
}

func has205(ls []model.CompanyWeeklyAutoLine, text string) int {
	n := 0
	for _, l := range ls {
		if l.Text == text {
			n++
		}
	}
	return n
}

// 묶음: 같은 상위 → 1줄 대표제목(기간), 상위 없는 같은 정규화 제목 → 1줄, 다른 제목·다른 사업은 합치지 않는다.
func TestCompanyWeekly95_205_GroupDone(t *testing.T) {
	r := draft205(t, seed205(t))
	want := map[string]int{
		"제안서 작성(11/9~11/11)":     1, // 같은 상위(반복) 3건
		"사업 제안서 작성(11/11~11/12)": 1, // 상위 없는 같은 정규화 제목 3건(공백 차이 포함)
		"다른 제목 업무(11/12)":         1, // 제목이 달라 합치지 않음
		"입찰 대응(11/12~11/13)":      1, // 하위 일정 → 상위 제목
	}
	for text, n := range want {
		if got := has205(r.AutoDone, text); got != n {
			t.Errorf("금주 %q = %d줄 want %d\n%s", text, got, n, dump205(r))
		}
	}
	if len(r.AutoDone) != len(want) {
		t.Errorf("금주 줄 수 %d want %d\n%s", len(r.AutoDone), len(want), dump205(r))
	}
	for _, l := range r.AutoDone {
		if strings.Contains(l.Text, "3회") {
			t.Errorf("완료 줄은 N회 대신 기간: %q", l.Text)
		}
	}
}

// 차주: 이월은 현행 N회(이월), 같은 문구 중복 제거, 예정일 있으면 기간, 구역별 Key 는 서로 달라야 한다.
func TestCompanyWeekly95_205_GroupPlanCarry(t *testing.T) {
	r := draft205(t, seed205(t))
	if got := has205(r.AutoPlan, "신구대조표 작성 3회(이월)"); got != 1 {
		t.Errorf("상위가 다른 같은 제목 이월 2묶음은 같은 문구 1줄만: %d줄\n%s", got, dump205(r))
	}
	if has205(r.AutoPlan, "입찰서류 준비 2회(이월)") != 1 {
		t.Errorf("같은 업무 2건 등록은 1줄\n%s", dump205(r))
	}
	if has205(r.AutoPlan, "제안서 작성 2회(이월)") != 1 || has205(r.AutoPlan, "제안서 작성(11/17)") != 1 {
		t.Errorf("같은 상위의 이월 2건 + 차주 1건\n%s", dump205(r))
	}
	if has205(r.AutoPlan, "현장 실사(11/17·11/18·11/20)") != 1 {
		t.Errorf("비연속 예정일 표기\n%s", dump205(r))
	}
	keys := map[string]bool{}
	for _, l := range append(append([]model.CompanyWeeklyAutoLine{}, r.AutoDone...), r.AutoPlan...) {
		if keys[l.Key] {
			t.Errorf("줄 Key 중복 %q\n%s", l.Key, dump205(r))
		}
		keys[l.Key] = true
	}
	if !keys["grp:plan:p:P4"] || !keys["grp:carry:p:P4"] {
		t.Errorf("구역별 grp Key 가 아니다\n%s", dump205(r))
	}
}

// 묶음 대상이 없는 데이터는 현행과 같다(낱줄 Key task:ID·날짜 괄호 유지).
func TestCompanyWeekly95_205_NoGroupUnchanged(t *testing.T) {
	r := draft205(t, seed205(t))
	for _, l := range r.AutoDone {
		if l.Key == "task:Z1" && l.Text != "다른 제목 업무(11/12)" {
			t.Fatalf("낱줄 변경: %q", l.Text)
		}
	}
	ls := groupWeeklyAdminLines(weeklySecDone, []adminLine{
		{ID: "A", Title: "가", Date: "2026-11-09", WorkType: "admin"},
		{ID: "B", Title: "나", Date: "2026-11-10", WorkType: "admin"},
		{ID: "AS1", Title: "AS 후속 학교", Date: "2026-11-10", WorkType: "as"},
	})
	if len(ls) != 3 || ls[0].GroupKey != "" || ls[1].GroupKey != "" || ls[2].Title != "AS 후속 학교" {
		t.Fatalf("묶음 대상 없음: %+v", ls)
	}
}

func TestDedupeAutoLines95_205(t *testing.T) {
	in := []model.CompanyWeeklyAutoLine{
		{Key: "a", Text: "·가 나(이월)"}, {Key: "b", Text: "가   나(이월)"}, {Key: "c", Text: "다"}, {Key: "d", Text: ""},
	}
	out := dedupeAutoLines(in)
	if len(out) != 3 || out[0].Key != "a" || out[1].Key != "c" {
		t.Fatalf("%+v", out)
	}
}

// 재집계: 묶음 줄을 직접 입력으로 바꾸거나 지운 뒤에도 같은 Key 로 유지·숨김이 지켜진다.
func TestMergeGroupKeyStable95_205(t *testing.T) {
	r := draft205(t, seed205(t))
	auto := AutoToLines(r.AutoDone)
	var gk string
	for _, l := range auto {
		if strings.HasPrefix(l.Text, "제안서 작성(") {
			gk = l.Key
		}
	}
	if gk != "grp:done:p:P1" {
		t.Fatalf("묶음 Key %q", gk)
	}
	manual := []CompanyWeeklyLine{{Key: gk, Text: "제안서 작성 일부 수정", Manual: true}, {Text: "직접 입력 줄", Manual: true}}
	merged := MergeWeeklyLinesKeyed(auto, manual, nil)
	var gotEdited, gotManual bool
	for _, l := range merged {
		if l.Key == gk && l.Text == "제안서 작성 일부 수정" && l.Manual {
			gotEdited = true
		}
		if l.Text == "직접 입력 줄" && l.Manual {
			gotManual = true
		}
	}
	if !gotEdited || !gotManual {
		t.Fatalf("직접 입력 전환·수동 줄 불변 실패: %+v", merged)
	}
	hidden := MergeWeeklyLinesKeyed(auto, nil, []string{gk})
	for _, l := range hidden {
		if l.Key == gk {
			t.Fatalf("숨긴 묶음 줄이 다시 나왔다")
		}
	}
	// 다시 집계해도 자동 줄 수는 그대로
	if again := MergeWeeklyLinesKeyed(auto, merged, nil); len(again) != len(merged) {
		t.Fatalf("재집계 줄 수 %d → %d", len(merged), len(again))
	}
}

// 활동 행(기술영업 등)도 문구가 완전히 같은 자동 줄은 1줄.
func TestKeyedToAutoDedupesSameText95_205(t *testing.T) {
	out := keyedToAuto([]keyedLine{
		{Key: "task:1", Text: "[사업] 제안 자료 정리"}, {Key: "task:2", Text: "[사업]  제안 자료 정리"}, {Key: "task:3", Text: "다른 줄"},
	})
	if len(out) != 2 || out[0].Key != "task:1" || out[1].Key != "task:3" {
		t.Fatalf("%+v", out)
	}
}
