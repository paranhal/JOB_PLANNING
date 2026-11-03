package model

import "testing"

func TestSuggestNewKeywordsRules(t *testing.T) {
	got := SuggestNewKeywords("게이트를 교체 했음 12345 완료 오류 불량 점검", []string{"오류"})
	if len(got) == 0 {
		t.Fatal("후보 없음")
	}
	if len(got) > 6 {
		t.Fatalf("최대 6 %d", len(got))
	}
	seen := map[string]bool{}
	for _, c := range got {
		seen[c.Keyword] = true
		if c.Keyword == "했음" || c.Keyword == "완료" || c.Keyword == "12345" || c.Keyword == "오류" {
			t.Fatalf("제외되어야 함 %s", c.Keyword)
		}
	}
	if !seen["게이트"] && !seen["교체"] && !seen["점검"] && !seen["불량"] {
		t.Fatalf("%+v", got)
	}
	if g := GuessKeywordGroup("교체"); g != KWGroupAction {
		t.Fatal(g)
	}
	if g := GuessKeywordGroup("불량"); g != KWGroupCause {
		t.Fatal(g)
	}
	if g := GuessKeywordGroup("끊김"); g != KWGroupSymptom {
		t.Fatal(g)
	}
	if g := GuessKeywordGroup("게이트"); g != KWGroupTarget {
		t.Fatal(g)
	}
}

func TestParseKeywordTokens(t *testing.T) {
	p := ParseKeywordTokens("DB(대상), 오류(증상), 재기동")
	if len(p.Err) != 0 || len(p.OK) != 3 {
		t.Fatalf("%+v", p)
	}
	if p.OK[2].Group != KWGroupAction {
		t.Fatal(p.OK[2])
	}
	p = ParseKeywordTokens("이름(조치사항)")
	if len(p.OK) != 1 || p.OK[0].Group != KWGroupAction {
		t.Fatalf("%+v", p)
	}
	p = ParseKeywordTokens("나쁜(없음)")
	if len(p.Err) == 0 {
		t.Fatal("그룹 오류")
	}
}
