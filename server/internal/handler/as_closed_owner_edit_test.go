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

func TestASOwnerCanModifyClosedWithoutUnlock(t *testing.T) {
	h := &ASHandler{}
	as := &model.ASReceipt{
		Status: "completed", CreatedByUserID: "u1",
		AssignedTo: "기술본인", AssignedUserID: "u1",
	}
	owner := echoCtxRole(model.RoleTech, "기술본인")
	owner.Set("user_id", "u1")
	if !h.canModifyAS(owner, as) {
		t.Fatal("본인은 완료 건을 잠금 없이 수정한다")
	}
	peer := echoCtxRole(model.RoleTech, "다른기술")
	peer.Set("user_id", "u2")
	if h.canModifyAS(peer, as) {
		t.Fatal("같은 조직 타인은 완료 건을 수정하면 안 된다")
	}
	oa := echoCtxRole(model.RoleOrgAdmin, "팀장")
	oa.Set("user_id", "oa")
	if h.canModifyAS(oa, as) {
		t.Fatal("관리자는 잠금 해제 없이는 불가")
	}
	ro := echoCtxReadOnly(model.RoleTech, "기술본인")
	ro.Set("user_id", "u1")
	if h.canModifyAS(ro, as) {
		t.Fatal("읽기 전용은 불가")
	}
}

func TestASClosedOwnerEditHTTPAndPeers(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "closed-owner.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c1','도서관','도서관',1,'O01')`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	users.EnsureAdmin(HashPassword("admin"))
	if err := users.Create(&model.User{
		Username: "tech1", PasswordHash: HashPassword("pw"), FullName: "기술본인",
		Role: model.RoleTech, IsActive: true, OrgID: "O01", ProfileDone: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := users.Create(&model.User{
		Username: "tech2", PasswordHash: HashPassword("pw"), FullName: "다른기술",
		Role: model.RoleTech, IsActive: true, OrgID: "O01", ProfileDone: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := users.Create(&model.User{
		Username: "oa", PasswordHash: HashPassword("pw"), FullName: "조직장",
		Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01", ProfileDone: true,
	}); err != nil {
		t.Fatal(err)
	}
	t1, _ := users.GetByUsername("tech1")
	t2, _ := users.GetByUsername("tech2")
	oa, _ := users.GetByUsername("oa")
	asRepo := repository.NewASRepo(db)
	recpt := &model.ASReceipt{
		CustomerID: "c1", Symptom: "원래증상", AssignedTo: "기술본인", AssignedUserID: t1.UserID,
		Status: "completed", ReceiptDatetime: time.Now(), OrgID: "O01",
		CreatedByUserID: t1.UserID, ActionTaken: "조치함", ResultCode: model.ResultDone,
	}
	if err := asRepo.Create(recpt); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE as_receipts SET status='completed', assigned_to=?, assigned_user_id=?, created_by_user_id=?, action_taken='조치함' WHERE as_id=?`,
		"기술본인", t1.UserID, t1.UserID, recpt.ASID); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/as/:id", h.AS.Show)
	g.GET("/as/:id/edit", h.AS.Edit)
	g.POST("/as/:id/edit", h.AS.UpdateReceipt)
	g.GET("/as/:id/action", h.AS.Action)
	g.POST("/as/:id/update", h.AS.Update)
	g.POST("/as/:id/unlock-edit", h.AS.UnlockEdit)
	g.POST("/attachments", h.Attachment.Upload)
	g.POST("/attachments/:id/delete", h.Attachment.Delete)

	ownerCK := jwtCookieUser(t, t1)
	peerCK := jwtCookieUser(t, t2)
	oaCK := jwtCookieUser(t, oa)

	show := doGetWith(t, e, "/as/"+recpt.ASID, ownerCK)
	if show.Code != http.StatusOK {
		t.Fatalf("본인 상세 %d", show.Code)
	}
	if !strings.Contains(show.Body.String(), "내용은 수정할 수 있습니다") {
		t.Fatal("본인에게 완료 후 수정 안내가 없다")
	}
	if strings.Contains(show.Body.String(), "읽기 전용") {
		t.Fatal("본인에게 읽기 전용 안내가 남았다")
	}
	if !strings.Contains(show.Body.String(), "접수 정보 수정") {
		t.Fatal("본인에게 접수 수정 버튼이 없다")
	}

	peerShow := doGetWith(t, e, "/as/"+recpt.ASID, peerCK)
	if !strings.Contains(peerShow.Body.String(), "읽기 전용") {
		t.Fatal("타인에게 읽기 전용 안내가 없다")
	}

	edit := doGetWith(t, e, "/as/"+recpt.ASID+"/edit", ownerCK)
	if edit.Code != http.StatusOK {
		t.Fatalf("본인 접수 수정 화면 %d", edit.Code)
	}
	if !strings.Contains(edit.Body.String(), "접수 저장") {
		t.Fatal("본인 수정 화면에 저장 단추가 없다")
	}
	peerEdit := doGetWith(t, e, "/as/"+recpt.ASID+"/edit", peerCK)
	if peerEdit.Code != http.StatusSeeOther && peerEdit.Code != http.StatusForbidden {
		t.Fatalf("타인 수정 화면 status=%d", peerEdit.Code)
	}

	post := httptest.NewRecorder()
	form := url.Values{
		"customer_id":      {"c1"},
		"symptom":          {"고친증상"},
		"received_by":      {"기술본인"},
		"receipt_datetime": {time.Now().Format("2006-01-02T15:04")},
		"assigned_to":      {"기술본인"},
	}
	req := httptest.NewRequest(http.MethodPost, "http://localhost/as/"+recpt.ASID+"/edit", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(ownerCK)
	e.ServeHTTP(post, req)
	if post.Code != http.StatusSeeOther {
		t.Fatalf("본인 저장 status=%d loc=%s", post.Code, post.Header().Get("Location"))
	}
	got, _ := asRepo.GetByID(repository.OrgAll, recpt.ASID)
	if got == nil || got.Symptom != "고친증상" {
		t.Fatalf("증상 미반영 %+v", got)
	}
	if got.Status != "completed" {
		t.Fatalf("완료 상태가 바뀌면 안 된다: %s", got.Status)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM data_change_logs WHERE table_name='as_receipts' AND entity_id=? AND action='update'`, recpt.ASID).Scan(&n); err != nil || n < 1 {
		t.Fatalf("변경 이력 n=%d err=%v", n, err)
	}

	peerPost := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "http://localhost/as/"+recpt.ASID+"/edit", strings.NewReader(form.Encode()))
	req2.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req2.AddCookie(peerCK)
	e.ServeHTTP(peerPost, req2)
	if peerPost.Code != http.StatusSeeOther && peerPost.Code != http.StatusForbidden {
		t.Fatalf("타인 저장 status=%d", peerPost.Code)
	}
	if peerPost.Code == http.StatusSeeOther && !strings.Contains(peerPost.Header().Get("Location"), "err=") && !strings.Contains(peerPost.Header().Get("Location"), "읽기") {
		again, _ := asRepo.GetByID(repository.OrgAll, recpt.ASID)
		if again != nil && again.Symptom != "고친증상" {
			t.Fatal("타인이 증상을 바꿨다")
		}
	}

	oaShow := doGetWith(t, e, "/as/"+recpt.ASID, oaCK)
	if !strings.Contains(oaShow.Body.String(), "읽기 전용") && !strings.Contains(oaShow.Body.String(), "잠금") {
		t.Fatal("관리자 미해제 화면에 잠금 안내가 없다")
	}

	ro := jwtCookieReadOnly(t, model.RoleTech)
	roPost := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodPost, "http://localhost/as/"+recpt.ASID+"/edit", strings.NewReader(form.Encode()))
	req3.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req3.AddCookie(ro)
	e.ServeHTTP(roPost, req3)
	if roPost.Code == http.StatusOK {
		t.Fatal("읽기 전용이 저장에 성공했다")
	}
}

func TestWorkCompletedAssigneeCanEdit(t *testing.T) {
	e, repo := newWorkTodayServer(t)
	id := findWorkTaskIDByTitle(t, repo, "관리자완료")
	form := url.Values{
		"scope":    {"register"},
		"title":    {"관리자완료-수정"},
		"status":   {model.WBTaskComplete},
		"due_date": {time.Now().Format("2006-01-02")},
	}
	rec := workFormCookie(t, e, "/workboard/tasks/"+id+"/update", form, jwtCookie(t))
	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusOK {
		t.Fatalf("완료 행정업무 수정 status=%d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
	got, err := repo.GetTask(id)
	if err != nil || got == nil {
		t.Fatal(err)
	}
	if got.Title != "관리자완료-수정" && !strings.Contains(got.Title, "관리자완료") {
		t.Fatalf("완료 후 제목=%s", got.Title)
	}
}

func TestSalesClosedOwnerUpdateKeepsWrite(t *testing.T) {
	e, db := newSalesServerDB(t)
	users := repository.NewUserRepo(db)
	if err := users.Create(&model.User{
		Username: "s1", PasswordHash: HashPassword("pw"), FullName: "영업하나",
		Role: model.RoleSales, IsActive: true, OrgID: "O01",
	}); err != nil {
		t.Fatal(err)
	}
	s1, _ := users.GetByUsername("s1")
	repo := repository.NewSalesRepo(db)
	p := &model.SalesProject{
		Name: "끝난사업", OrgID: "O01", SalesOwner: "영업하나", SalesOwnerID: s1.UserID,
		Stage: model.SalesStage4Closed, Status: model.SalesStatusContracted, DealType: model.SalesDealBuild,
		CreatedByUserID: s1.UserID,
	}
	if err := repo.Create(p); err != nil {
		t.Fatal(err)
	}
	page := doGetWith(t, e, "/sales/"+p.SalesID+"/edit", jwtCookieUser(t, s1))
	if page.Code != http.StatusOK {
		t.Fatalf("종료 사업 수정 화면 %d", page.Code)
	}
}
