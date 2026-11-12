package model

import (
	"reflect"
	"testing"
)

func TestWeeklyInputLines(t *testing.T) {
	done, next := WeeklyInputLines(ProgressReport{
		Has: true, Phase: "구축", Package: "현장 설치",
		Plans: []ReportPlan{
			{Name: "홍천 지역", Basis: BasisSite, SitesDone: 4, SitesTotal: 10},
			{Name: "횡성 지역", Basis: BasisSite, SitesTotal: 5},
			{Name: "원주 지역", Basis: BasisSite, SitesDone: 3, SitesTotal: 3, Confirmed: true},
			{Name: "분모없음", Basis: BasisSite},
		},
		PackageQty: &ReportPackageQty{Name: "환경조사", Unit: "식", Done: 30, Total: 120},
	})
	wantDone := []string{"- 홍천 지역 10곳 중 4곳 완료(40%)", "- 원주 지역 3곳 완료", "- 환경조사 120식 중 30식 완료(25%)"}
	wantNext := []string{"- 홍천 지역 남은 6곳 진행", "- 횡성 지역 5곳 착수", "- 환경조사 남은 90식 진행"}
	if !reflect.DeepEqual(done, wantDone) {
		t.Fatalf("금주 %v", done)
	}
	if !reflect.DeepEqual(next, wantNext) {
		t.Fatalf("차주 %v", next)
	}
	if d, n := WeeklyInputLines(ProgressReport{}); d != nil || n != nil {
		t.Fatal("구조가 없으면 줄이 없어야 한다")
	}
	many := ProgressReport{Has: true}
	for i := 0; i < 9; i++ {
		many.Plans = append(many.Plans, ReportPlan{Name: "계획", Basis: BasisSite, SitesDone: 1, SitesTotal: 2})
	}
	if d, _ := WeeklyInputLines(many); len(d) != WeeklyInputMax {
		t.Fatalf("상한 %d", len(d))
	}
}
