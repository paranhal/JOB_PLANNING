package handler

import (
	"encoding/json"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func newCoopServer(t *testing.T) (*echo.Echo, *repository.CoopRepo, *model.User, *model.User, *model.User, *model.User) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "coophttp.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O02','O02','미디어','business',1,20)`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	oa1 := &model.User{Username: "oa1", PasswordHash: "x", FullName: "도서팀장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	tech1 := &model.User{Username: "t1", PasswordHash: "x", FullName: "도서기술", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	oa2 := &model.User{Username: "oa2", PasswordHash: "x", FullName: "미디어팀장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O02", ProfileDone: true}
	cl := &model.User{Username: "cl", PasswordHash: "x", FullName: "씨레벨", Role: model.RoleCLevel, IsActive: true, IsReadOnly: true, ProfileDone: true}
	for _, u := range []*model.User{oa1, tech1, oa2, cl} {
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
	g.GET("/cooperations/inbox", h.Coop.ListInbox)
	g.GET("/cooperations/sent", h.Coop.ListSent)
	g.GET("/cooperations/new", h.Coop.New)
	g.POST("/cooperations", h.Coop.Create)
	g.GET("/cooperations/:id", h.Coop.Show)
	g.GET("/cooperations/:id/project.json", h.Coop.ProjectJSON)
	g.POST("/cooperations/:id/accept", h.Coop.Accept)
	g.POST("/cooperations/:id/reject", h.Coop.Reject)
	g.POST("/admin/users", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	return e, repository.NewCoopRepo(db), oa1, tech1, oa2, cl
}

func TestCoopHTTPFlow(t *testing.T) {
	e, repo, oa1, tech1, oa2, cl := newCoopServer(t)
	t1 := jwtCookieUserClaims(t, tech1)
	oa2ck := jwtCookieUserClaims(t, oa2)
	clck := jwtCookieUserClaims(t, cl)
	oa1ck := jwtCookieUserClaims(t, oa1)

	self := postForm947(t, e, "/cooperations", url.Values{
		"target_org_id": {model.OrgIDLibrary}, "kind": {"admin"}, "title": {"자기"}, "body": {"x"},
	}, t1)
	if self.Code == http.StatusSeeOther && !strings.Contains(self.Header().Get("Location"), "err=") {
		t.Fatal("자기 팀")
	}
	create := postForm947(t, e, "/cooperations", url.Values{
		"target_org_id": {"O02"}, "kind": {"admin"}, "title": {"HTTP협조"}, "body": {"내용입니다"},
	}, t1)
	if create.Code != http.StatusSeeOther {
		t.Fatalf("create %d %s", create.Code, create.Body.String())
	}
	sent := doGetWith(t, e, "/cooperations/sent", t1)
	if sent.Code != http.StatusOK || !strings.Contains(sent.Body.String(), "HTTP협조") {
		t.Fatalf("sent %d", sent.Code)
	}
	if doGetWith(t, e, "/cooperations/new", clck).Code != http.StatusForbidden {
		t.Fatal("clevel new")
	}
	items, _ := repo.ListSent(model.CoopActor{UserID: tech1.UserID, OrgID: tech1.OrgID, Role: tech1.Role})
	if len(items) == 0 {
		t.Fatal("items")
	}
	id := items[0].CoopID
	inbox := doGetWith(t, e, "/cooperations/inbox", oa2ck)
	if inbox.Code != http.StatusOK || !strings.Contains(inbox.Body.String(), "HTTP협조") {
		t.Fatal("inbox")
	}
	if doGetWith(t, e, "/cooperations/"+id, jwtCookieUserClaims(t, oa1)).Code == http.StatusNotFound {
		t.Fatal("요청 팀장")
	}
	if postForm947(t, e, "/cooperations/"+id+"/reject", url.Values{"reject_reason": {""}}, oa2ck).Code == http.StatusSeeOther {
		got, _ := repo.Get(id)
		if got.Status == model.CoopRejected {
			t.Fatal("빈 거부")
		}
	}
	acc := postForm947(t, e, "/cooperations/"+id+"/accept", url.Values{}, oa2ck)
	if acc.Code != http.StatusSeeOther {
		t.Fatalf("accept %d", acc.Code)
	}
	if postForm947(t, e, "/cooperations/"+id+"/accept", url.Values{}, clck).Code != http.StatusForbidden {
		t.Fatal("clevel accept")
	}
	show := doGetWith(t, e, "/cooperations/"+id, clck)
	if show.Code != http.StatusOK {
		t.Fatalf("clevel show %d", show.Code)
	}
	pj := doGetWith(t, e, "/cooperations/"+id+"/project.json", oa2ck)
	if pj.Code != http.StatusNotFound {
		var m map[string]any
		_ = json.Unmarshal(pj.Body.Bytes(), &m)
		raw := pj.Body.String()
		if strings.Contains(raw, "contract_amount") || strings.Contains(raw, "order_amount") {
			t.Fatal(raw)
		}
	}
	_ = oa1ck
	res := FilterNav(visionMenuCtx(), NavCoopInbox, NavBadges{CoopInbox: 2})
	if !containsHref(collectNavHrefs(res.Groups), "/cooperations/inbox") || !containsHref(collectNavHrefs(res.Groups), "/cooperations/sent") {
		t.Fatal("nav")
	}
}

func TestCoopNavParentNoHref(t *testing.T) {
	if dups := navDuplicateHrefs(); len(dups) > 0 {
		t.Fatalf("중복 href: %v", dups)
	}
}
