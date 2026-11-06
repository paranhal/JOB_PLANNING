package handler

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func Test95_66_SplitAndSanitizeMembers(t *testing.T) {
	got := splitMemberIDs([]string{" a,b ", "b;c", "d"})
	if strings.Join(got, ",") != "a,b,c,d" {
		t.Fatalf("split=%v", got)
	}
	team := []model.User{{UserID: "U1"}, {UserID: "U2"}}
	if got := sanitizeMemberIDs([]string{"U2", "U9", "U1", "U2"}, team); strings.Join(got, ",") != "U2,U1" {
		t.Fatalf("sanitize=%v", got)
	}
	if got := filterTasksByMemberIDs(nil, nil, nil, nil); got != nil {
		t.Fatal("빈 ID는 0건")
	}
}

func Test95_66_OrgAdminMemberBoxAndFilter(t *testing.T) {
	e, _, _, us, today, _ := seedEquivBoard(t)
	ack := jwtCookieUser(t, us.admin)
	list := doGetWith(t, e, "/workboard/board?view=day&date="+today+"&auto=0", ack)
	if list.Code != http.StatusOK {
		t.Fatalf("code=%d", list.Code)
	}
	body := list.Body.String()
	if !strings.Contains(body, us.me.FullName) {
		t.Fatal("일정표 담당자 열이 없다")
	}
	if !strings.Contains(body, "날짜본인") || !strings.Contains(body, "관리자행정") {
		t.Fatal("기본 전체에서 조직 업무가 빠졌다")
	}

	tech := doGetWith(t, e, "/workboard/board?view=day&date="+today+"&auto=0", jwtCookieUser(t, us.me)).Body.String()
	if strings.Contains(tech, "aria-label=\"팀원별 보기\"") {
		t.Fatal("기술 담당자에게 팀원별 보기 박스가 있다")
	}

	one := doGetWith(t, e, "/workboard/board?view=day&date="+today+"&auto=0&members="+url.QueryEscape(us.me.UserID), ack).Body.String()
	if !strings.Contains(one, "날짜본인") {
		t.Fatal("members 한 명에 본인 업무 없음")
	}
	if strings.Contains(one, "관리자행정") {
		t.Fatal("members 한 명에 다른 사람 업무가 섞였다")
	}
	if !strings.Contains(one, "구성원전용") {
		t.Fatal("지원 구성원 합집합이 빠졌다")
	}

	both := "/workboard/board?view=day&date=" + today + "&auto=0&members=" + url.QueryEscape(us.me.UserID) + "&members=" + url.QueryEscape(us.admin.UserID)
	uni := doGetWith(t, e, both, ack).Body.String()
	if !strings.Contains(uni, "날짜본인") || !strings.Contains(uni, "관리자행정") {
		t.Fatal("여러 명 합집합 실패")
	}
	if !strings.Contains(uni, "members="+us.me.UserID) && !strings.Contains(uni, "members="+url.QueryEscape(us.me.UserID)) {
		t.Fatal("선택 members 가 링크에 유지되지 않는다")
	}

	legacy := doGetWith(t, e, "/workboard/board?view=day&date="+today+"&auto=0&assignee="+url.QueryEscape(us.me.FullName), ack).Body.String()
	if !strings.Contains(legacy, "날짜본인") {
		t.Fatal("assignee=이름 호환 실패")
	}

	foreign := doGetWith(t, e, "/workboard/board?view=day&date="+today+"&auto=0&members="+url.QueryEscape(us.other.UserID), ack).Body.String()
	if strings.Contains(foreign, "날짜타인") || strings.Contains(foreign, "날짜본인") || strings.Contains(foreign, "관리자행정") {
		t.Fatal("타 조직 members ID 가 격리되지 않았다")
	}
}
