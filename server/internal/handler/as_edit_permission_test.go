package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func TestCanEditASReceiptMatrix(t *testing.T) {
	as := &model.ASReceipt{
		Status: "in_progress", CreatedByUserID: "u-sup",
		AssignedTo: "기술본인", AssignedUserID: "u-tech", OrgID: "O01",
	}
	h := &ASHandler{}
	closed := *as
	closed.Status = "completed"

	ownerSup := echoCtxRole(model.RoleSupport, "지원본인")
	ownerSup.Set("user_id", "u-sup")
	if !canEditASReceipt(ownerSup, as) {
		t.Fatal("등록 지원은 수정 가능")
	}

	ownerTech := echoCtxRole(model.RoleTech, "기술본인")
	ownerTech.Set("user_id", "u-tech")
	if !canEditASReceipt(ownerTech, as) {
		t.Fatal("배정 기술(user id)은 수정 가능")
	}

	byName := echoCtxRole(model.RoleTech, "이름담당")
	byName.Set("user_id", "u-name")
	named := &model.ASReceipt{Status: "in_progress", AssignedTo: "이름담당", CreatedByUserID: "other"}
	if !canEditASReceipt(byName, named) {
		t.Fatal("배정 이름 일치는 수정 가능")
	}

	peer := echoCtxRole(model.RoleSupport, "다른지원")
	peer.Set("user_id", "u-peer")
	if canEditASReceipt(peer, as) {
		t.Fatal("같은 조직 다른 지원은 불가")
	}
	peerTech := echoCtxRole(model.RoleTech, "다른기술")
	peerTech.Set("user_id", "u-peer2")
	if canEditASReceipt(peerTech, as) {
		t.Fatal("같은 조직 다른 기술은 불가")
	}
	peerSales := echoCtxRole(model.RoleSales, "다른영업")
	peerSales.Set("user_id", "u-peer3")
	if canEditASReceipt(peerSales, as) {
		t.Fatal("같은 조직 다른 영업은 불가")
	}

	oa := echoCtxRole(model.RoleOrgAdmin, "팀장")
	oa.Set("user_id", "oa")
	oa.Set("org_id", "O01")
	if !canEditASReceipt(oa, as) {
		t.Fatal("자기 조직 조직관리자는 가능")
	}
	oa.Set("sim_org_id", "O01")
	oa.Set("view_org_id", "O02")
	// org_admin은 조직 전환이 없어 currentOrg=home. sim만 켜면 inHome이 달라질 수 있어 비전만 전 조직.
	vis := echoCtxRole(model.RoleVisionAdmin, "비전")
	vis.Set("user_id", "vs")
	if !canEditASReceipt(vis, as) {
		t.Fatal("비전 관리자는 가능")
	}

	ro := echoCtxReadOnly(model.RoleSupport, "지원본인")
	ro.Set("user_id", "u-sup")
	if canEditASReceipt(ro, as) {
		t.Fatal("읽기 전용은 불가")
	}
	view := echoCtxRole(model.RoleOrgAdmin, "팀장")
	view.Set("user_id", "oa")
	view.Set("view_as_user_id", "u-sup")
	if canEditASReceipt(view, as) {
		t.Fatal("시점 보기는 불가")
	}

	if !h.canModifyAS(ownerSup, &closed) {
		t.Fatal("본인 완료 건은 잠금 없이 가능")
	}
	if !h.canModifyAS(oa, &closed) {
		t.Fatal("조직관리자 완료 건은 잠금 없이 가능")
	}
	if h.canModifyAS(vis, &closed) {
		t.Fatal("비전 관리자 완료 건은 잠금 필요")
	}
	if h.canModifyAS(peer, &closed) {
		t.Fatal("타인 완료 건은 불가")
	}
}

func TestASEditPermissionHTTP(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "as-edit-perm.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO orgs(org_id,org_no,org_name,sort_order) VALUES('O02','O02','다른팀',2)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c1','도서관','도서관',1,'O01'),('c2','다른관','다른관',1,'O02')`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	users.EnsureAdmin(HashPassword("admin"))
	mustUser := func(u *model.User) *model.User {
		t.Helper()
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
		got, err := users.GetByUsername(u.Username)
		if err != nil || got == nil {
			t.Fatal(err)
		}
		return got
	}
	sup1 := mustUser(&model.User{Username: "sup1", PasswordHash: HashPassword("pw"), FullName: "지원하나", Role: model.RoleSupport, IsActive: true, OrgID: "O01", ProfileDone: true})
	sup2 := mustUser(&model.User{Username: "sup2", PasswordHash: HashPassword("pw"), FullName: "지원둘", Role: model.RoleSupport, IsActive: true, OrgID: "O01", ProfileDone: true})
	tech1 := mustUser(&model.User{Username: "tech1", PasswordHash: HashPassword("pw"), FullName: "기술하나", Role: model.RoleTech, IsActive: true, OrgID: "O01", ProfileDone: true})
	sales1 := mustUser(&model.User{Username: "sales1", PasswordHash: HashPassword("pw"), FullName: "영업하나", Role: model.RoleSales, IsActive: true, OrgID: "O01", ProfileDone: true})
	oa := mustUser(&model.User{Username: "oa1", PasswordHash: HashPassword("pw"), FullName: "조직장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01", ProfileDone: true})
	vs := mustUser(&model.User{Username: "vs1", PasswordHash: HashPassword("pw"), FullName: "비전", Role: model.RoleVisionAdmin, IsActive: true, OrgID: "O01", ProfileDone: true})
	ro := mustUser(&model.User{Username: "ro1", PasswordHash: HashPassword("pw"), FullName: "읽기지원", Role: model.RoleSupport, IsActive: true, OrgID: "O01", ProfileDone: true, IsReadOnly: true})

	asRepo := repository.NewASRepo(db)
	makeAS := func(cust, symptom, assigned, assignedUID, createdUID, org, status string) *model.ASReceipt {
		t.Helper()
		rec := &model.ASReceipt{
			CustomerID: cust, Symptom: symptom, AssignedTo: assigned, AssignedUserID: assignedUID,
			Status: status, ReceiptDatetime: time.Now(), OrgID: org, CreatedByUserID: createdUID,
			ReceivedBy: "지원하나", ActionTaken: "조치함", ResultCode: model.ResultDone,
		}
		if err := asRepo.Create(rec); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`UPDATE as_receipts SET status=?, assigned_to=?, assigned_user_id=?, created_by_user_id=? WHERE as_id=?`,
			status, assigned, assignedUID, createdUID, rec.ASID); err != nil {
			t.Fatal(err)
		}
		return rec
	}
	openSup := makeAS("c1", "지원접수", "기술하나", tech1.UserID, sup1.UserID, "O01", "in_progress")
	openName := makeAS("c1", "이름배정", "기술하나", "", sup1.UserID, "O01", "in_progress")
	openSales := makeAS("c1", "영업접수", "", "", sales1.UserID, "O01", "in_progress")
	otherOrg := makeAS("c2", "다른조직", "", "", sup1.UserID, "O02", "in_progress")
	closedSup := makeAS("c1", "완료본인", "기술하나", tech1.UserID, sup1.UserID, "O01", "completed")

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.Use(h.Auth.InjectViewAs)
	g.Use(h.Auth.GuardViewAsWrite)
	g.GET("/as/:id", h.AS.Show)
	g.GET("/as/:id/edit", h.AS.Edit)
	g.POST("/as/:id/edit", h.AS.UpdateReceipt)
	g.POST("/as/:id/customer-direct", h.AS.UpdateCustomerDirect)

	sup1CK := jwtCookieUser(t, sup1)
	sup2CK := jwtCookieUser(t, sup2)
	techCK := jwtCookieUser(t, tech1)
	salesCK := jwtCookieUser(t, sales1)
	oaCK := jwtCookieUser(t, oa)
	vsCK := jwtCookieUser(t, vs)
	roCK := jwtCookieUser(t, ro)

	assertEditOK := func(name, id string, ck *http.Cookie) {
		t.Helper()
		show := doGetWith(t, e, "/as/"+id, ck)
		if show.Code != http.StatusOK {
			t.Fatalf("%s 상세 %d", name, show.Code)
		}
		if !strings.Contains(show.Body.String(), "접수 정보 수정") {
			t.Fatalf("%s 상세에 수정 링크가 없다", name)
		}
		edit := doGetWith(t, e, "/as/"+id+"/edit", ck)
		if edit.Code != http.StatusOK {
			t.Fatalf("%s 수정 화면 %d loc=%s", name, edit.Code, edit.Header().Get("Location"))
		}
		if !strings.Contains(edit.Body.String(), `id="as-submit-btn"`) {
			t.Fatalf("%s 수정 화면에 저장 단추가 없다(취소만 있는 화면)", name)
		}
	}
	assertEditDenied := func(name, id string, ck *http.Cookie) {
		t.Helper()
		show := doGetWith(t, e, "/as/"+id, ck)
		if show.Code == http.StatusOK && strings.Contains(show.Body.String(), "접수 정보 수정") {
			t.Fatalf("%s 상세에 수정 링크가 있으면 안 된다", name)
		}
		edit := doGetWith(t, e, "/as/"+id+"/edit", ck)
		if edit.Code == http.StatusOK {
			t.Fatalf("%s 가 수정 화면을 열면 안 된다", name)
		}
		if edit.Code == http.StatusSeeOther {
			loc := edit.Header().Get("Location")
			if !strings.Contains(loc, "/as/"+id) || !strings.Contains(loc, "err=") {
				t.Fatalf("%s 거부 이동 loc=%s", name, loc)
			}
			follow := doGetWith(t, e, loc, ck)
			if follow.Code == http.StatusOK && !strings.Contains(follow.Body.String(), asReceiptEditDeniedMsg) && !strings.Contains(follow.Body.String(), "읽기 전용") {
				t.Fatalf("%s 안내에 거부 문구가 없다 body 일부=%s", name, follow.Body.String()[:min(400, follow.Body.Len())])
			}
		}
		form := url.Values{
			"customer_id": {"c1"}, "symptom": {"침입시도"}, "received_by": {"x"},
			"receipt_datetime": {time.Now().Format("2006-01-02T15:04")},
		}
		post := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "http://localhost/as/"+id+"/edit", strings.NewReader(form.Encode()))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		req.AddCookie(ck)
		e.ServeHTTP(post, req)
		if post.Code == http.StatusOK {
			t.Fatalf("%s 저장이 성공하면 안 된다", name)
		}
		direct := httptest.NewRecorder()
		dreq := httptest.NewRequest(http.MethodPost, "http://localhost/as/"+id+"/customer-direct",
			strings.NewReader(url.Values{"customer_direct": {"1"}, "org_name_direct": {"침입기관"}}.Encode()))
		dreq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		dreq.AddCookie(ck)
		e.ServeHTTP(direct, dreq)
		if direct.Code == http.StatusOK {
			t.Fatalf("%s 직접입력이 성공하면 안 된다", name)
		}
		again, _ := asRepo.GetByID(repository.OrgAll, id)
		if again != nil && again.Symptom == "침입시도" {
			t.Fatalf("%s 가 증상을 바꿨다", name)
		}
	}

	assertEditOK("등록 지원", openSup.ASID, sup1CK)
	assertEditOK("배정 기술 id", openSup.ASID, techCK)
	assertEditOK("배정 기술 이름", openName.ASID, techCK)
	assertEditOK("등록 영업", openSales.ASID, salesCK)
	assertEditOK("조직관리자 자기 조직", openSup.ASID, oaCK)
	assertEditOK("비전", openSup.ASID, vsCK)

	assertEditDenied("다른 지원", openSup.ASID, sup2CK)
	assertEditDenied("다른 지원-영업건", openSales.ASID, sup2CK)
	assertEditDenied("읽기 전용", openSup.ASID, roCK)

	otherShow := doGetWith(t, e, "/as/"+otherOrg.ASID, oaCK)
	if otherShow.Code != http.StatusNotFound && otherShow.Code != http.StatusForbidden {
		if otherShow.Code == http.StatusOK && strings.Contains(otherShow.Body.String(), "접수 정보 수정") {
			t.Fatal("조직관리자가 다른 조직 건을 수정하면 안 된다")
		}
		if otherShow.Code == http.StatusOK {
			t.Fatalf("다른 조직 상세 status=%d", otherShow.Code)
		}
	}
	otherEdit := doGetWith(t, e, "/as/"+otherOrg.ASID+"/edit", oaCK)
	if otherEdit.Code == http.StatusOK {
		t.Fatal("조직관리자가 다른 조직 수정 화면을 열었다")
	}

	assertEditOK("본인 완료", closedSup.ASID, sup1CK)
	assertEditOK("조직관리자 완료", closedSup.ASID, oaCK)
	vsClosed := doGetWith(t, e, "/as/"+closedSup.ASID+"/edit", vsCK)
	if vsClosed.Code == http.StatusOK {
		t.Fatal("비전 관리자가 잠금 없이 완료 건 수정 화면을 열었다")
	}
	assertEditDenied("타인 완료", closedSup.ASID, sup2CK)

	oaForm := url.Values{
		"customer_id": {"c1"}, "symptom": {"조직장수정"}, "received_by": {"조직장"},
		"receipt_datetime": {time.Now().Format("2006-01-02T15:04")}, "assigned_to": {"기술하나"},
	}
	oaPost := httptest.NewRecorder()
	oreq := httptest.NewRequest(http.MethodPost, "http://localhost/as/"+closedSup.ASID+"/edit", strings.NewReader(oaForm.Encode()))
	oreq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	oreq.AddCookie(oaCK)
	e.ServeHTTP(oaPost, oreq)
	if oaPost.Code != http.StatusSeeOther {
		t.Fatalf("조직관리자 완료 저장 %d", oaPost.Code)
	}
	got, _ := asRepo.GetByID(repository.OrgAll, closedSup.ASID)
	if got == nil || got.Symptom != "조직장수정" {
		t.Fatalf("조직관리자 완료 수정 미반영 %+v", got)
	}
	var n int
	var before, after, uname, at string
	if err := db.QueryRow(`SELECT COUNT(*), COALESCE(before_json,''), COALESCE(after_json,''), COALESCE(user_name,''), COALESCE(occurred_at,'')
		FROM data_change_logs WHERE table_name='as_receipts' AND entity_id=? AND action='update'
		ORDER BY occurred_at DESC LIMIT 1`, closedSup.ASID).Scan(&n, &before, &after, &uname, &at); err != nil || n < 1 {
		t.Fatalf("완료 수정 이력 n=%d err=%v", n, err)
	}
	if !strings.Contains(before, "완료본인") || !strings.Contains(after, "조직장수정") {
		t.Fatalf("이력 이전/이후 값이 없다 before=%s after=%s", before, after)
	}
	if strings.TrimSpace(uname) == "" || strings.TrimSpace(at) == "" {
		t.Fatalf("이력 수정자/일시 없음 name=%s at=%s", uname, at)
	}

	viewCK := jwtCookieUser(t, oa)
	editView := httptest.NewRecorder()
	vreq := httptest.NewRequest(http.MethodGet, "http://localhost/as/"+openSup.ASID+"/edit", nil)
	vreq.AddCookie(viewCK)
	vreq.AddCookie(&http.Cookie{Name: "view_as_user_id", Value: sup2.UserID, Path: "/"})
	e.ServeHTTP(editView, vreq)
	if editView.Code == http.StatusOK {
		t.Fatal("시점 보기에서 수정 화면이 열렸다")
	}
}
