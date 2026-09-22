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
	if strings.Contains(lb, "구축·입찰") || strings.Contains(lb, "단품·납품") {
		t.Fatal("유형 탭이 남았다")
	}
	if !strings.Contains(lb, "세종 RFID") {
		t.Fatal("기본 목록에 구축 건이 없다")
	}
	if !strings.Contains(lb, "감열지 단품") {
		t.Fatal("기본 목록에 단품이 없다")
	}
	if strings.Contains(lb, "deal=build") {
		t.Fatal("기본 FilterQ 에 deal=build 가 붙었다")
	}

	buildShow := doGet(t, e, "/sales/"+buildID)
	if buildShow.Code != http.StatusOK {
		t.Fatalf("구축 상세 status=%d", buildShow.Code)
	}
	bb := buildShow.Body.String()
	if !strings.Contains(bb, "10%") || !strings.Contains(bb, "발굴") {
		t.Fatalf("구축 상세가 달라졌다: %s", clipBody(bb))
	}

	supplyList := doGet(t, e, "/sales?deal=supply")
	if supplyList.Code != http.StatusOK {
		t.Fatalf("단품 목록 status=%d", supplyList.Code)
	}
	sl := supplyList.Body.String()
	_ = sl

	kanban := doGet(t, e, "/sales?display=kanban")
	if kanban.Code != http.StatusOK {
		t.Fatalf("칸반 status=%d", kanban.Code)
	}

	show := doGet(t, e, "/sales/"+supplyID)
	if show.Code != http.StatusOK {
		t.Fatalf("단품 상세 status=%d", show.Code)
	}
	sb := show.Body.String()
	if strings.Contains(sb, "deal_type") && strings.Contains(sb, "radio") {
		t.Fatal("상세에 유형 라디오가 있다")
	}
	if strings.Contains(sb, "발주번호") {
		t.Fatal("상세에 발주번호가 남았다")
	}

	form := doGet(t, e, "/sales/new")
	if form.Code != http.StatusOK {
		t.Fatalf("등록 폼 status=%d", form.Code)
	}
	fb := form.Body.String()
	if strings.Contains(fb, "name=\"deal_type\"") {
		t.Fatal("폼에 deal_type 이 있다")
	}
	if strings.Contains(fb, "확도 %") {
		t.Fatal("폼에 수동 확도가 있다")
	}

	rec = doForm(t, e, "/sales/"+supplyID, url.Values{
		"name":            {"감열지 단품"},
		"expected_amount": {"1200000"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("저장 status=%d", rec.Code)
	}
	afterPO := doGet(t, e, "/sales/"+supplyID)
	if !strings.Contains(afterPO.Body.String(), "발굴") {
		t.Fatalf("저장 후 단계가 바뀌었다: %s", clipBody(afterPO.Body.String()))
	}

	mix := doFormJSON(t, e, "/sales/"+supplyID+"/stage", url.Values{"stage": {"proposal"}})
	if mix.Code != http.StatusBadRequest {
		t.Fatalf("옛 단계 혼입 status=%d %s", mix.Code, mix.Body.String())
	}

	closed := doFormJSON(t, e, "/sales/"+supplyID+"/stage", url.Values{"stage": {"closed"}})
	if closed.Code != http.StatusBadRequest {
		t.Fatalf("종료 끌어놓기 status=%d %s", closed.Code, closed.Body.String())
	}

	pipe := doGet(t, e, "/sales/pipeline?deal=supply")
	if pipe.Code != http.StatusMovedPermanently {
		t.Fatalf("파이프라인 리다이렉트 status=%d", pipe.Code)
	}
	loc := pipe.Header().Get("Location")
	if !strings.Contains(loc, "view=kanban") {
		t.Fatalf("파이프라인 Location=%q", loc)
	}
	kanban = doGet(t, e, loc)
	if kanban.Code != http.StatusOK {
		t.Fatalf("칸반 status=%d loc=%s", kanban.Code, loc)
	}
}
