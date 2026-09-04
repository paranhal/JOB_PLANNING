package handler

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestSalesHTTP_SupplyDealTypeListAndAutoStage(t *testing.T) {
	e := newSalesServer(t)

	rec := doForm(t, e, "/sales", url.Values{
		"name":              {"세종 RFID 증설"},
		"is_tentative_name": {"1"},
		"deal_type":         {"build"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("구축 등록 status=%d", rec.Code)
	}
	buildID := salesIDFromRedirect(t, rec.Header().Get("Location"))

	rec = doForm(t, e, "/sales", url.Values{
		"name":            {"감열지 단품"},
		"deal_type":       {"supply"},
		"expected_amount": {"1200000"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("단품 등록 status=%d", rec.Code)
	}
	supplyID := salesIDFromRedirect(t, rec.Header().Get("Location"))

	list := doGet(t, e, "/sales")
	if list.Code != http.StatusOK {
		t.Fatalf("/sales status=%d", list.Code)
	}
	lb := list.Body.String()
	if !strings.Contains(lb, "구축·입찰") || !strings.Contains(lb, "단품·납품") {
		t.Fatal("유형 탭이 없다")
	}
	if !strings.Contains(lb, "세종 RFID") {
		t.Fatal("기본 목록에 구축 건이 없다")
	}
	if strings.Contains(lb, "감열지 단품") {
		t.Fatal("기본 목록에 단품이 섞였다")
	}
	if strings.Contains(lb, "deal=build") {
		t.Fatal("기본 FilterQ 에 deal=build 가 붙었다")
	}

	buildShow := doGet(t, e, "/sales/"+buildID)
	if buildShow.Code != http.StatusOK {
		t.Fatalf("구축 상세 status=%d", buildShow.Code)
	}
	bb := buildShow.Body.String()
	if !strings.Contains(bb, "25%") || !strings.Contains(bb, "제안 진행") {
		t.Fatalf("구축 상세가 달라졌다: %s", clipBody(bb))
	}

	supplyList := doGet(t, e, "/sales?deal=supply")
	if supplyList.Code != http.StatusOK {
		t.Fatalf("단품 목록 status=%d", supplyList.Code)
	}
	sl := supplyList.Body.String()
	if !strings.Contains(sl, "감열지 단품") {
		t.Fatal("단품 탭에 단품 건이 없다")
	}
	if strings.Contains(sl, "세종 RFID") {
		t.Fatal("단품 탭에 구축 건이 보였다")
	}
	if strings.Contains(sl, "제안 진행") {
		t.Fatal("단품 단계에 제안 진행이 나왔다")
	}
	if !strings.Contains(sl, "문의 접수") || !strings.Contains(sl, "견적 제출") ||
		!strings.Contains(sl, "납품 완료") || !strings.Contains(sl, "취소·실주") {
		t.Fatalf("단품 5단계가 없다: %s", clipBody(sl))
	}

	kanban := doGet(t, e, "/sales?deal=supply&display=kanban")
	if kanban.Code != http.StatusOK {
		t.Fatalf("단품 칸반 status=%d", kanban.Code)
	}
	kb := kanban.Body.String()
	if strings.Count(kb, "flex-1 basis-0") != 5 {
		t.Fatalf("단품 칸반 열=%d", strings.Count(kb, "flex-1 basis-0"))
	}

	show := doGet(t, e, "/sales/"+supplyID)
	if show.Code != http.StatusOK {
		t.Fatalf("단품 상세 status=%d", show.Code)
	}
	sb := show.Body.String()
	if strings.Contains(sb, "확률") || strings.Contains(sb, "확도 %") || strings.Contains(sb, "25%") || strings.Contains(sb, "40%") {
		t.Fatalf("단품에 확도가 보인다: %s", clipBody(sb))
	}
	if !strings.Contains(sb, "견적 제출") || !strings.Contains(sb, "1,200,000원") {
		t.Fatalf("견적 금액이 안 뜬다: %s", clipBody(sb))
	}
	if !strings.Contains(sb, "정보 확정도") {
		t.Fatal("정보 확정도가 없다")
	}
	if strings.Contains(sb, "제안 진행") {
		t.Fatal("단품 pill 에 제안 진행이 있다")
	}
	if strings.Contains(sb, "사업관리로 등록") {
		t.Fatal("단품에 승격 버튼이 있다")
	}

	form := doGet(t, e, "/sales/new?deal=supply")
	if form.Code != http.StatusOK {
		t.Fatalf("단품 등록 폼 status=%d", form.Code)
	}
	fb := form.Body.String()
	if strings.Contains(fb, "확도 %") || strings.Contains(fb, "제안 진행") {
		t.Fatalf("단품 폼에 구축 단계/확도가 있다: %s", clipBody(fb))
	}
	if !strings.Contains(fb, "견적 금액") {
		t.Fatal("단품 폼에 견적 금액이 없다")
	}

	rec = doForm(t, e, "/sales/"+supplyID, url.Values{
		"name":            {"감열지 단품"},
		"po_no":           {"PO-11"},
		"expected_amount": {"1200000"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("발주 저장 status=%d", rec.Code)
	}
	afterPO := doGet(t, e, "/sales/"+supplyID)
	if !strings.Contains(afterPO.Body.String(), "수주 ·") {
		t.Fatalf("발주 후 수주가 아니다: %s", clipBody(afterPO.Body.String()))
	}

	rec = doForm(t, e, "/sales/"+supplyID, url.Values{
		"name":            {"감열지 단품"},
		"po_no":           {"PO-11"},
		"delivered_at":    {"2026-09-04"},
		"expected_amount": {"1200000"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("납품 저장 status=%d", rec.Code)
	}
	afterD := doGet(t, e, "/sales/"+supplyID)
	if !strings.Contains(afterD.Body.String(), "납품 완료 ·") {
		t.Fatalf("납품 후 단계가 아니다: %s", clipBody(afterD.Body.String()))
	}

	mix := doFormJSON(t, e, "/sales/"+supplyID+"/stage", url.Values{"stage": {"proposal"}})
	if mix.Code != http.StatusBadRequest {
		t.Fatalf("구축 단계 혼입 status=%d %s", mix.Code, mix.Body.String())
	}

	lost := doFormJSON(t, e, "/sales/"+supplyID+"/stage", url.Values{"stage": {"dropped"}})
	if lost.Code != http.StatusBadRequest || !strings.Contains(lost.Body.String(), "실패 사유") {
		t.Fatalf("실주 사유 없이 옮겨졌다: %d %s", lost.Code, lost.Body.String())
	}

	pipe := doGet(t, e, "/sales/pipeline?deal=supply")
	if pipe.Code != http.StatusOK {
		t.Fatalf("단품 파이프라인 status=%d", pipe.Code)
	}
	if strings.Count(pipe.Body.String(), "flex-1 basis-0") != 5 {
		t.Fatalf("단품 파이프라인 열=%d", strings.Count(pipe.Body.String(), "flex-1 basis-0"))
	}
}
