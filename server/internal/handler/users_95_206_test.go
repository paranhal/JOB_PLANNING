package handler

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// 95-206: 사용자 관리 — 로그인 아이디 항상 표시, 「수정」 열 오른쪽 고정·이름 클릭, 덜 중요한 열 숨김, 조회 전용 안내.

type env206 struct {
	e     *echo.Echo
	users *repository.UserRepo
	vs    *model.User
	oa    *model.User
	cl    *model.User
}

func setup206(t *testing.T) env206 {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "u206.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,sort_order) VALUES('O02','O02','이팀',2)`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/admin/users", h.Auth.UserList)
	g.GET("/users", h.Auth.UserList)
	g.POST("/users/:id/update", h.Auth.UserUpdate)
	g.POST("/users/:id/username", h.Auth.UserRenameUsername)
	mk := func(u *model.User) *model.User {
		u.PasswordHash, u.IsActive, u.ProfileDone = HashPassword("password10"), true, true
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
		return u
	}
	v := env206{e: e, users: users}
	v.vs = mk(&model.User{Username: "vs206", Email: "vs206@example.test", Mobile: "010-1111-1111", FullName: "비젼206", Role: model.RoleVisionAdmin})
	v.oa = mk(&model.User{Username: "oa206", Email: "oa206@example.test", FullName: "일팀장", Role: model.RoleOrgAdmin, OrgID: "O01"})
	mk(&model.User{Username: "mail206", Email: "mail206@example.test", FullName: "메일있음", Role: model.RoleTech, OrgID: "O01"})
	mk(&model.User{Username: "nomail206", FullName: "메일없음", Role: model.RoleTech, OrgID: "O01", Mobile: "010-0000-0000"})
	mk(&model.User{Username: "away206", Email: "away206@example.test", FullName: "이팀숨김", Role: model.RoleTech, OrgID: "O02"})
	v.cl = mk(&model.User{Username: "cl206", FullName: "경영진", Role: model.RoleCLevel, IsReadOnly: true})
	// 아이디가 비어 있는 행(옛 데이터): 표시는 「(아이디 없음)」
	if _, err := db.Exec(`UPDATE users SET username='' WHERE full_name='이팀숨김'`); err != nil {
		t.Fatal(err)
	}
	return v
}

func (v env206) get(t *testing.T, u *model.User) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/admin/users", nil)
	if u.OrgID != "" {
		req.AddCookie(jwtOrgCookie(t, u))
	} else {
		req.AddCookie(visionCookie(t, u))
	}
	rec := httptest.NewRecorder()
	v.e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /admin/users %d", rec.Code)
	}
	return rec.Body.String()
}

// 메일이 있는 사용자도, 없는 사용자도 「아이디: …」가 보인다. 비어 있으면 「(아이디 없음)」.
func TestUsers95_206_UsernameAlwaysShown(t *testing.T) {
	v := setup206(t)
	body := v.get(t, v.vs)
	for _, want := range []string{"아이디: mail206", "아이디: nomail206", "아이디: oa206", "아이디: (아이디 없음)"} {
		if !strings.Contains(body, want) {
			t.Errorf("%q 가 목록에 없다", want)
		}
	}
	// 메일은 보조로 계속 보인다. 예전의 「@아이디」 단독 표기는 없다.
	if !strings.Contains(body, "mail206@example.test") {
		t.Error("메일이 사라졌다")
	}
	if strings.Contains(body, ">@nomail206") || strings.Contains(body, "@nomail206 ·") {
		t.Error("옛 @아이디 표기가 남았다")
	}
	// 모든 데이터 행에 아이디 줄이 있다
	rows := regexp.MustCompile(`<tr class="group border-t`).FindAllString(body, -1)
	ids := strings.Count(body, "data-user-id>아이디: ")
	if len(rows) == 0 || len(rows) != ids {
		t.Errorf("행 %d · 아이디 줄 %d", len(rows), ids)
	}
	// 검색 속성(data-user)은 그대로
	if !strings.Contains(body, `data-user="mail206"`) {
		t.Error("검색용 data-user 가 바뀌었다")
	}
}

// 수정 진입: 비젼관리자·조직관리자에게는 오른쪽 고정 「수정」 열·이름 버튼·서랍이 있고, 조회 전용(C-Level)에는 없다.
func TestUsers95_206_EditEntryByPermission(t *testing.T) {
	v := setup206(t)
	vis := v.get(t, v.vs)
	for _, want := range []string{`class="edit px-3 py-1`, `data-users-editcol`, `sticky right-0`, `class="edit-name`, `id="drawer"`, `id="edit-form"`, `hidden 2xl:table-cell`} {
		if !strings.Contains(vis, want) {
			t.Errorf("비젼관리자 화면에 %q 없음", want)
		}
	}
	if strings.Contains(vis, "data-users-readonly") {
		t.Error("비젼관리자에게 조회 전용 안내가 나온다")
	}
	// 열 숨김은 최근 활동만: 팀장 열과 팀장 지정 영역은 숨기지 않는다
	if strings.Contains(vis, `hidden 2xl:table-cell">팀장`) {
		t.Error("팀장 열을 숨겼다")
	}
	if strings.Count(vis, "hidden 2xl:table-cell") != 1+strings.Count(vis, `<tr class="group border-t`) {
		t.Errorf("숨김 열이 최근 활동(머리+행)이 아니다")
	}
	// 조직관리자: 자기 조직만, 수정 진입 있음
	oa := v.get(t, v.oa)
	if !strings.Contains(oa, `class="edit px-3 py-1`) || !strings.Contains(oa, `id="drawer"`) {
		t.Error("조직관리자에게 수정 진입이 없다")
	}
	if !strings.Contains(oa, "아이디: mail206") || strings.Contains(oa, "이팀숨김") || strings.Contains(oa, "away206") {
		t.Error("조직관리자 목록 범위가 바뀌었다")
	}
	// 조회 전용(C-Level): 수정 버튼·이름 버튼·서랍 없음, 아이디는 보임, 안내 한 줄
	cl := v.get(t, v.cl)
	if strings.Contains(cl, `class="edit px-3`) || strings.Contains(cl, `class="edit-name`) || strings.Contains(cl, `id="drawer"`) || strings.Contains(cl, `id="edit-form"`) {
		t.Error("조회 전용에 수정 진입이 있다")
	}
	if !strings.Contains(cl, "data-users-readonly") || !strings.Contains(cl, "조회 전용입니다") {
		t.Error("조회 전용 안내가 없다")
	}
	if !strings.Contains(cl, "아이디: mail206") {
		t.Error("조회 전용에도 아이디는 보여야 한다")
	}
}
