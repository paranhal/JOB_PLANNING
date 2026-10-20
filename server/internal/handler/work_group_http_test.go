package handler

import (
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func newWGServer(t *testing.T) (*echo.Echo, *repository.WorkGroupRepo, *model.User, *model.User, *model.User) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "wghttp.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	oa := &model.User{Username: "oa", PasswordHash: "x", FullName: "조직장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	tech := &model.User{Username: "t1", PasswordHash: "x", FullName: "기술원", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	cl := &model.User{Username: "cl", PasswordHash: "x", FullName: "씨레벨", Role: model.RoleCLevel, IsActive: true, IsReadOnly: true, ProfileDone: true}
	for _, u := range []*model.User{oa, tech, cl} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.Use(h.Auth.GuardViewAsWrite)
	g.GET("/work-groups", h.WorkGroup.List)
	g.GET("/work-groups/new", h.WorkGroup.Form)
	g.POST("/work-groups", h.WorkGroup.Create)
	g.GET("/work-groups/:id", h.WorkGroup.Form)
	g.POST("/work-groups/:id/close", h.WorkGroup.Close)
	g.POST("/work-groups/:id/members", h.WorkGroup.AddMember)
	g.POST("/work-groups/:id/leader", h.WorkGroup.SetLeader)
	g.POST("/admin/users", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	return e, repository.NewWorkGroupRepo(db), oa, tech, cl
}

func TestWorkGroupHTTPPermissions(t *testing.T) {
	e, repo, oa, tech, cl := newWGServer(t)
	oaCK := jwtCookieUserClaims(t, oa)
	techCK := jwtCookieUserClaims(t, tech)
	clCK := jwtCookieUserClaims(t, cl)

	list := doGetWith(t, e, "/work-groups", oaCK)
	if list.Code != http.StatusOK {
		t.Fatalf("list %d", list.Code)
	}
	if !strings.Contains(list.Body.String(), "새 업무 그룹") {
		t.Fatal("관리자 생성 단추")
	}
	techList := doGetWith(t, e, "/work-groups", techCK)
	if strings.Contains(techList.Body.String(), "새 업무 그룹") {
		t.Fatal("조직원 생성 단추")
	}
	clList := doGetWith(t, e, "/work-groups", clCK)
	if clList.Code != http.StatusOK {
		t.Fatalf("clevel list %d", clList.Code)
	}

	create := postForm947(t, e, "/work-groups", url.Values{"name": {"HTTP그룹"}}, oaCK)
	if create.Code != http.StatusSeeOther {
		t.Fatalf("create %d %s", create.Code, create.Body.String())
	}
	items, _ := repo.ListGroups(model.OrgIDLibrary, "")
	if len(items) != 1 {
		t.Fatalf("n=%d", len(items))
	}
	id := items[0].GroupID
	if postForm947(t, e, "/work-groups", url.Values{"name": {"임원그룹"}}, clCK).Code != http.StatusForbidden {
		t.Fatal("clevel create")
	}
	if postForm947(t, e, "/work-groups/"+id+"/close", url.Values{}, techCK).Code != http.StatusForbidden {
		t.Fatal("조직원 종료")
	}
	if postForm947(t, e, "/work-groups/"+id+"/leader", url.Values{"user_id": {tech.UserID}}, techCK).Code != http.StatusForbidden {
		t.Fatal("조직원 그룹장")
	}
	add := postForm947(t, e, "/work-groups/"+id+"/members", url.Values{"user_id": {tech.UserID}}, oaCK)
	if add.Code != http.StatusSeeOther {
		t.Fatalf("add %d", add.Code)
	}
	if err := repo.SetLeader(model.WorkGroupActor{UserID: oa.UserID, OrgID: model.OrgIDLibrary, Role: model.RoleOrgAdmin}, id, tech.UserID); err != nil {
		t.Fatal(err)
	}
	form := doGetWith(t, e, "/work-groups/"+id, techCK)
	if form.Code != http.StatusOK {
		t.Fatalf("form %d", form.Code)
	}
	body := form.Body.String()
	if !strings.Contains(body, "구성원 추가") {
		t.Fatal("그룹장 추가 단추")
	}
	if strings.Contains(body, "그룹 종료") || strings.Contains(body, "그룹장 지정") {
		t.Fatal("그룹장에게 관리 단추")
	}
	foreign := doGetWith(t, e, "/work-groups/WG-9999", techCK)
	if foreign.Code != http.StatusNotFound {
		t.Fatalf("없는 그룹 %d", foreign.Code)
	}
}

func TestWorkGroupNavReachable(t *testing.T) {
	res := FilterNav(visionMenuCtx(), NavWorkGroups, NavBadges{})
	if !containsHref(collectNavHrefs(res.Groups), "/work-groups") {
		t.Fatal("메뉴")
	}
	if countNavActive(res.Groups) != 1 {
		t.Fatal("active")
	}
}
