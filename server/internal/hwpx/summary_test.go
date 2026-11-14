package hwpx

import (
	"encoding/xml"
	"strings"
	"testing"
)

func TestBuildWeeklySummary(t *testing.T) {
	data, err := BuildWeeklySummary("주간업무 발표 요약", "11월 2주차", []SummaryTeam{{Name: "도서관사업팀", Items: []SummaryItem{{
		Name: "WP-061 & <유사>", Assignee: "홍길동", Done: []string{"점검 완료"}, Plan: []string{"설치 예정"}, Need: "자재\n지원",
	}}}})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := unzipEntries(data)
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].Name != "mimetype" || string(entries[0].Body) != "application/hwp+zip" {
		t.Fatalf("첫 항목은 mimetype 이어야 한다: %s", entries[0].Name)
	}
	var sec string
	for _, e := range entries {
		if strings.HasSuffix(e.Name, ".xml") || strings.HasSuffix(e.Name, ".hpf") {
			if err := xml.Unmarshal(e.Body, new(interface{})); err != nil {
				t.Fatalf("%s XML 오류: %v", e.Name, err)
			}
		}
		if e.Name == "Contents/section0.xml" {
			sec = string(e.Body)
		}
	}
	for _, want := range []string{"주간업무 발표 요약", "■ 도서관사업팀", "WP-061 &amp; &lt;유사&gt;", "금주 추진내역", "- 점검 완료", "차주 추진계획", "- 설치 예정", "지원 필요사항: 자재 지원"} {
		if !strings.Contains(sec, want) {
			t.Fatalf("section0 에 %q 없음", want)
		}
	}
	if _, err := BuildWeeklySummary("x", "", []SummaryTeam{{Name: "빈팀"}}); err == nil {
		t.Fatal("줄이 없으면 오류여야 한다")
	}
}
