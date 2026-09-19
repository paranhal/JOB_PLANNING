package model

import (
	"strings"
	"testing"
)

func TestFormatASWorkTitle(t *testing.T) {
	if got := FormatASWorkTitle("가나도서관", "R2608-001"); got != "[AS]가나도서관_R2608-001" {
		t.Fatalf("as: %q", got)
	}
	if got := FormatASWorkTitle("가나도서관", ""); got != "[AS]가나도서관" {
		t.Fatalf("no num: %q", got)
	}
}

func TestFormatSalesWorkTitle(t *testing.T) {
	if got := FormatSalesWorkTitle("방문미팅 · 세종 RFID"); got != "[영업]방문미팅 · 세종 RFID" {
		t.Fatalf("sales: %q", got)
	}
	if got := FormatSalesWorkTitle("[영업]이미있음"); got != "[영업]이미있음" {
		t.Fatalf("already: %q", got)
	}
}

func TestWBCategorySalesNotAdmin(t *testing.T) {
	if got := WBCategory(WBSourceSalesActivity); got != WBSourceSalesActivity {
		t.Fatalf("sales_activity → %q", got)
	}
	if got := WBCategory(WorkPrefixSales); got != WBSourceSalesActivity {
		t.Fatalf("sales 접두어가 admin 으로 떨어졌다: %q", got)
	}
	if got := WBCategoryLabel(WBCategory(WorkPrefixSales)); got != "영업" {
		t.Fatalf("sales 라벨: %q", got)
	}
	if got := WBCategoryLabel(WBCategory(WBSourceSalesActivity)); got != "영업" {
		t.Fatalf("sales_activity 라벨: %q", got)
	}
	if got := WBCategory(""); got != WBWorkAdmin {
		t.Fatalf("빈 source: %q", got)
	}
	if got := (WBCard{Category: WBSourceSalesActivity}).Label(); got != "[영업]" {
		t.Fatalf("카드 라벨: %q", got)
	}
}

func TestWBWorkTypeSalesBucketAndLabel(t *testing.T) {
	if got := WBWorkTypeBucket(WBWorkSales); got != WBWorkSales {
		t.Fatalf("bucket sales → %q", got)
	}
	if WBWorkTypeBucket(WBWorkSales) == WBWorkAdmin {
		t.Fatal("sales 가 admin 으로 빨려 들어갔다")
	}
	if got := WBWorkTypeLabel(WBWorkSales); got != "영업활동" {
		t.Fatalf("label: %q", got)
	}
	badge := WBCategoryLabel(WBCategory(WBSourceSalesActivity))
	if !strings.Contains(badge, "영업") || badge == "행정" {
		t.Fatalf("목록 배지: %q", badge)
	}
	if WBWorkTypeLabel(WBWorkSales) == "행정업무" {
		t.Fatal("상세 업무 구분이 행정업무다")
	}
}

func TestFormatMaintenance(t *testing.T) {
	if got := FormatMaintenanceWorkTitle("새롬동도서관", "2026-08-05 · KLAS"); got != "[점검]새롬동도서관_2026-08-05 · KLAS" {
		t.Fatalf("title: %q", got)
	}
	if got := FormatMaintenanceDescription("KLAS", "새롬동도서관"); got != "[KLAS]새롬동도서관 정기점검" {
		t.Fatalf("desc: %q", got)
	}
	if got := FormatMaintenanceDescription("", "테스트"); got != "[점검]테스트 정기점검" {
		t.Fatalf("empty product: %q", got)
	}
}

func TestWBCardDetailHref(t *testing.T) {
	as := WBCard{
		Kind: "task", TaskID: "wt_1", RefID: "wt_1",
		Category: WBSourceAS, SourceHref: "/as/as_99",
	}
	if got := as.DetailHref(); got != "/as/as_99/action" {
		t.Fatalf("배치 AS 열기(조치): %q", got)
	}
	if got := as.EditHref(); got != "/workboard/tasks/wt_1/edit" {
		t.Fatalf("배치 AS 수정: %q", got)
	}
	mnt := WBCard{
		Kind: "task", TaskID: "wt_2",
		Category:   WBSourceMaintenance,
		SourceHref: "/maintenance/mpl_1?month=8&view=list",
		ActionHref: "/maintenance/visits/v1/action",
	}
	if got := mnt.DetailHref(); got != "/maintenance/visits/v1/action" {
		t.Fatalf("배치 점검 조치(방문): %q", got)
	}
	if got := mnt.EditHref(); got != "/workboard/tasks/wt_2/edit" {
		t.Fatalf("배치 점검 수정(업무등록): %q", got)
	}
	admin := WBCard{Kind: "task", TaskID: "wt_3", Category: WBWorkAdmin}
	if got := admin.DetailHref(); got != "/workboard/tasks/wt_3" {
		t.Fatalf("행정 조치: %q", got)
	}
	if got := admin.EditHref(); got != "/workboard/tasks/wt_3/edit" {
		t.Fatalf("행정 수정(등록): %q", got)
	}
	palette := WBCard{Kind: WBSourceAS, Category: WBSourceAS, RefID: "as_1", SourceHref: "/as/as_1"}
	if got := palette.DetailHref(); got != "" {
		t.Fatalf("미배치 AS: %q", got)
	}
	if got := palette.EditHref(); got != "" {
		t.Fatalf("미배치 수정: %q", got)
	}
}

func TestDurationHelpers(t *testing.T) {
	if got := NormalizeDurationMin(40); got != 40 {
		t.Fatalf("minute unit: %d", got)
	}
	if got := NormalizeDurationMin(25); got != 25 {
		t.Fatalf("25min: %d", got)
	}
	if got := DurationFromTimes("07:00", "07:25"); got != 25 {
		t.Fatalf("dur: %d", got)
	}
	if got := DurationFromTimes("07:00", "08:00"); got != 60 {
		t.Fatalf("dur60: %d", got)
	}
	if got := FormatHHMMMinutes(7*60 + 15); got != "07:15" {
		t.Fatalf("fmt: %q", got)
	}
}

func TestWorkTaskCustomerLabel(t *testing.T) {
	if got := (WorkTask{OrgName: "충남교육청", CustomerName: "직접입력"}).CustomerLabel(); got != "충남교육청" {
		t.Fatalf("기관명 우선: %q", got)
	}
	if got := (WorkTask{CustomerName: "충남교육청"}).CustomerLabel(); got != "충남교육청" {
		t.Fatalf("직접입력: %q", got)
	}
	if got := (WorkTask{Title: "과업심의 자료 제출"}).CustomerLabel(); got != "" {
		t.Fatalf("거래처 없으면 업무명을 쓰지 않음: %q", got)
	}
}
