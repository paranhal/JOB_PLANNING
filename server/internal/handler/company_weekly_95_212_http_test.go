package handler

import (
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// 95-212: 주 1차 마감 표시와 23:55 안내 띠. 보고 주 10/19 의 마감 = 10/23(금) 24:00 = 10/24(토) 00:00 KST.

func withWeeklyNow(t *testing.T, now time.Time) {
	t.Helper()
	old := weeklyNow
	weeklyNow = func() time.Time { return now }
	t.Cleanup(func() { weeklyNow = old })
}

func kst95212(d, h, m int) time.Time { return time.Date(2026, 10, d, h, m, 0, 0, model.KST()) }

func TestHTTP95_212_CloseIndicator(t *testing.T) {
	v := setup205(t)
	oa := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
	cl := jwtCookieRole(t, model.RoleCLevel)

	// 마감 전(이번 주): 안내 문구 + 남은 시간. 쓰기 권한자에게만 안내 띠 요소.
	withWeeklyNow(t, kst95212(21, 10, 0))
	body := v.do(t, "GET", w205+"&edit=0", nil, oa).Body.String()
	if !strings.Contains(body, "1차 마감") || !strings.Contains(body, "10/23(금) 24:00") || !strings.Contains(body, "남은 시간") {
		t.Fatalf("마감 전 표기 없음")
	}
	if !strings.Contains(body, "data-weekly-close-banner") || !strings.Contains(body, `data-close-state="pending"`) {
		t.Fatal("쓰기 권한자 안내 띠 요소가 없다")
	}
	if !strings.Contains(body, "저장 버튼을 눌러 데이터를 저장하세요") {
		t.Fatal("안내 띠 문구가 스크립트에 없다")
	}
	ro := v.do(t, "GET", w205+"&edit=0", nil, cl).Body.String()
	if !strings.Contains(ro, "1차 마감") || strings.Contains(ro, "data-weekly-close-banner") {
		t.Fatal("읽기 전용은 안내문만(띠 없음)이어야 한다")
	}

	// 서버 시각을 속성으로 내려준다(브라우저 시계와 무관): 서버 now=10/23 23:56
	withWeeklyNow(t, kst95212(23, 23, 56))
	body = v.do(t, "GET", w205+"&edit=0", nil, oa).Body.String()
	nowMs := kst95212(23, 23, 56).UnixMilli()
	if !strings.Contains(body, `data-server-now-ms="`+itoa95212(nowMs)+`"`) || !strings.Contains(body, `data-close-at-ms="`+itoa95212(kst95212(24, 0, 0).UnixMilli())+`"`) {
		t.Fatal("서버 시각/마감 시각 속성 오류")
	}

	// 마감 후(기록됨): 「1차 마감됨」 + 다시 집계 안내(쓰기 권한자)
	r := v.h.Stats.repo
	if res := r.RunWeeklyClose1(kst95212(24, 0, 0)); len(res) == 0 {
		t.Fatal("마감 실행 결과 없음")
	}
	withWeeklyNow(t, kst95212(24, 9, 0))
	body = v.do(t, "GET", w205+"&edit=0", nil, oa).Body.String()
	if !strings.Contains(body, "1차 마감됨") || !strings.Contains(body, "다시 집계") || strings.Contains(body, "data-weekly-close-banner") {
		t.Fatal("마감 후 표기 오류")
	}
	ro = v.do(t, "GET", w205+"&edit=0", nil, cl).Body.String()
	if !strings.Contains(ro, "1차 마감됨") || strings.Contains(ro, "「다시 집계」를 누르세요") {
		t.Fatal("읽기 전용은 마감 시각만, 다시 집계 안내 없음")
	}
	// 지난 주(마감 기록 없음)는 표시하지 않는다
	body = v.do(t, "GET", "/stats/company-weekly?week=2026-10-12&org=O03&edit=0", nil, oa).Body.String()
	if strings.Contains(body, "data-weekly-close") {
		t.Fatal("기록 없는 과거 주에는 마감 표시가 없어야 한다")
	}
	_ = repository.WeeklyCloseCreated
}

func itoa95212(n int64) string {
	b := []byte{}
	if n == 0 {
		return "0"
	}
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// 마감이 만든 보고서는 확정 보고서와 마찬가지로 화면에서 편집·저장·다시 집계를 막지 않는다.
func TestHTTP95_212_EditStillWorksAfterClose(t *testing.T) {
	v := setup205(t)
	oa := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
	v.h.Stats.repo.RunWeeklyClose1(kst95212(24, 0, 0))
	withWeeklyNow(t, kst95212(24, 9, 0))
	if r := v.do(t, "GET", w205+"&edit=1", nil, oa); r.Code != 200 || !strings.Contains(r.Body.String(), "weekly-form") {
		t.Fatalf("마감 후 편집 화면 %d", r.Code)
	}
	g, _ := v.h.Stats.repo.GetCompanyWeeklyReport("O03", "2026-10-26")
	if repository.ParseCompanyWeeklyDoc(g.ContentJSON).Close1At() == "" {
		t.Fatal("close1_at 이 기록되지 않았다")
	}
}
