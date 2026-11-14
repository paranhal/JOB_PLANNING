package repository

import (
	"encoding/json"
	"strings"
	"testing"
)

// 95-192: Encode 가 새 필드(팀 전체 금주·차주)와 알 수 없는 최상위 키를 버리지 않는다.
func TestCompanyWeeklyDoc95_192_RoundTripKeepsTeamAndUnknownKeys(t *testing.T) {
	raw := `{"progress":{"done":3,"total":10,"pct":30,"cycle":"반기","memo":"","updated_by":"a","updated_at":"x"},` +
		`"rows":{"401":{"prev":"가"}},"team_done":[{"text":"팀 금주","manual":true}],"team_plan":[{"text":"팀 차주","manual":true}],"future_key":{"a":1}}`
	doc := ParseCompanyWeeklyDoc(raw)
	if len(doc.TeamDone) != 1 || doc.TeamDone[0].Text != "팀 금주" || len(doc.TeamPlan) != 1 || doc.TeamPlan[0].Text != "팀 차주" {
		t.Fatalf("팀 전체를 읽지 못함 %+v", doc)
	}
	out := EncodeCompanyWeeklyDoc(doc)
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &top); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"progress", "rows", "team_done", "team_plan", "future_key"} {
		if _, ok := top[k]; !ok {
			t.Fatalf("Encode 가 %q 를 버렸다: %s", k, out)
		}
	}
	// 진행율 저장 경로처럼 Parse → 일부만 바꿔 → Encode 해도 팀 전체가 남는다
	doc = ParseCompanyWeeklyDoc(out)
	doc.Progress.Memo = "바뀜"
	again := ParseCompanyWeeklyDoc(EncodeCompanyWeeklyDoc(doc))
	if len(again.TeamDone) != 1 || again.Progress == nil || again.Progress.Memo != "바뀜" || !strings.Contains(EncodeCompanyWeeklyDoc(again), "future_key") {
		t.Fatalf("왕복 뒤 팀 전체/미지 키 손실 %+v", again)
	}
	// 옛 형식(행 맵만)과 비어 있는 팀 전체는 키를 만들지 않는다
	if s := EncodeCompanyWeeklyDoc(ParseCompanyWeeklyDoc(`{"401":{"prev":"가"}}`)); strings.Contains(s, "team_") {
		t.Fatalf("팀 전체가 없는데 키가 생겼다 %s", s)
	}
}
