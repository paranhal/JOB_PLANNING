package handler

import (
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func countSubstr(s, sub string) int {
	return strings.Count(s, sub)
}

func Test95_36A_OrgAdminNewReceiptEmptyAssigneePreservesForm(t *testing.T) {
	e, h, asRepo, _, _ := newReceiptPhotoFixture(t)
	users := repository.NewUserRepo(h.AS.db)
	must := func(u *model.User) *model.User {
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
	oa := must(&model.User{Username: "oa36a", PasswordHash: HashPassword("pw"), FullName: "조직장36A", Role: model.RoleOrgAdmin, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true})
	sup := must(&model.User{Username: "sup36a", PasswordHash: HashPassword("pw"), FullName: "지원36A", Role: model.RoleSupport, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true})
	tech := must(&model.User{Username: "tech36a", PasswordHash: HashPassword("pw"), FullName: "기술36A", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true})
	oaCK := jwtCookieUser(t, oa)
	supCK := jwtCookieUser(t, sup)
	techCK := jwtCookieUser(t, tech)

	g := &model.WorkGroup{OrgID: oa.OrgID, Name: "점검A팀"}
	if err := h.AS.groups.Create(model.WorkGroupActor{UserID: oa.UserID, OrgID: oa.OrgID, Role: oa.Role}, g); err != nil {
		t.Fatal(err)
	}

	newPage := doGetWith(t, e, "/as/new", oaCK)
	if newPage.Code != http.StatusOK {
		t.Fatalf("GET /as/new %d", newPage.Code)
	}
	body := newPage.Body.String()
	if countSubstr(body, `data-focus-target="assignee"`) < 2 {
		t.Fatal("장비 기본 행·템플릿에 data-focus-target=assignee 가 없다")
	}
	if countSubstr(body, `name="eq_work_group_id"`) < 2 {
		t.Fatal("장비 행에 eq_work_group_id 가 없다")
	}
	if !strings.Contains(body, `data-require-assignee`) {
		t.Fatal("org_admin 신규 접수에 data-require-assignee 가 없다")
	}
	if !strings.Contains(body, "배정 담당자") || !strings.Contains(body, "text-red-500") {
		t.Fatal("org_admin 장비 행 담당자 필수 표시가 없다")
	}
	if !strings.Contains(body, "또는 업무 그룹") {
		t.Fatal("장비 행에 업무 그룹 선택이 없다")
	}

	form := url.Values{
		"customer_id":        {"cust_a"},
		"received_by":        {"조직장36A"},
		"receipt_datetime":   {time.Now().Format("2006-01-02T15:04")},
		"confirm_contact":    {"010-3333-4444"},
		"photo_memo":         {"오류화면 메모"},
		"eq_asset_id":        {""},
		"eq_symptom":         {"게이트 무반응 재현"},
		"eq_urgency_reason":  {"none"},
		"eq_visit_date":      {""},
		"eq_assigned_code":   {""},
		"eq_assigned_custom": {""},
		"eq_work_group_id":   {""},
	}
	miss := postForm(t, e, "/as", form, oaCK)
	if miss.Code == http.StatusSeeOther {
		t.Fatalf("리다이렉트로 입력값이 지워진다 loc=%s", miss.Header().Get("Location"))
	}
	if miss.Code != http.StatusUnprocessableEntity && miss.Code != http.StatusOK {
		t.Fatalf("담당 없이 저장 code=%d body=%s", miss.Code, miss.Body.String()[:min(400, miss.Body.Len())])
	}
	missBody := miss.Body.String()
	if strings.Contains(miss.Header().Get("Location"), "assignee_required") {
		t.Fatal("쿼리 리다이렉트면 입력값이 사라진다")
	}
	if !strings.Contains(missBody, "담당자를 선택하세요") {
		t.Fatal("오류 문구 없음")
	}
	if !strings.Contains(missBody, "게이트 무반응 재현") {
		t.Fatal("증상이 보존되지 않았다")
	}
	if !strings.Contains(missBody, "오류화면 메모") {
		t.Fatal("사진 메모가 보존되지 않았다")
	}
	if !strings.Contains(missBody, "010-3333-4444") {
		t.Fatal("연락처가 보존되지 않았다")
	}
	if !strings.Contains(missBody, `data-focus-target="assignee"`) {
		t.Fatal("포커스 대상이 없다")
	}

	two := url.Values{
		"customer_id":        {"cust_a"},
		"received_by":        {"조직장36A"},
		"receipt_datetime":   {time.Now().Format("2006-01-02T15:04")},
		"eq_asset_id":        {"", ""},
		"eq_symptom":         {"1행 채움", "2행 비움"},
		"eq_urgency_reason":  {"none", "none"},
		"eq_visit_date":      {"", ""},
		"eq_assigned_code":   {tech.UserID, ""},
		"eq_assigned_custom": {"", ""},
		"eq_work_group_id":   {"", ""},
	}
	twoRec := postForm(t, e, "/as", two, oaCK)
	if twoRec.Code == http.StatusSeeOther {
		t.Fatalf("2행 중 1행 비움이 리다이렉트 loc=%s", twoRec.Header().Get("Location"))
	}
	twoBody := twoRec.Body.String()
	if !strings.Contains(twoBody, "1행 채움") || !strings.Contains(twoBody, "2행 비움") {
		t.Fatal("두 행 증상이 보존되지 않았다")
	}
	if !strings.Contains(twoBody, `selected>`+tech.FullName) && !strings.Contains(twoBody, `value="`+tech.UserID+`" selected`) {
		t.Fatal("채운 행의 담당자가 보존되지 않았다")
	}

	okPerson := url.Values{
		"customer_id":      {"cust_a"},
		"received_by":      {"조직장36A"},
		"receipt_datetime": {time.Now().Format("2006-01-02T15:04")},
		"eq_asset_id":      {""},
		"eq_symptom":       {"개인 담당 저장"},
		"eq_assigned_code": {tech.UserID},
		"eq_work_group_id": {""},
	}
	ok := postForm(t, e, "/as", okPerson, oaCK)
	if ok.Code != http.StatusSeeOther || strings.Contains(ok.Header().Get("Location"), "err=") {
		t.Fatalf("개인 담당 저장 loc=%s code=%d", ok.Header().Get("Location"), ok.Code)
	}
	got, err := asRepo.GetByID(repository.OrgAll, asIDFromHref(ok.Header().Get("Location")))
	if err != nil || got == nil {
		t.Fatalf("get person: %v", err)
	}
	if got.AssignedUserID != tech.UserID {
		t.Fatalf("AssignedUserID=%s", got.AssignedUserID)
	}

	okGroup := url.Values{
		"customer_id":      {"cust_a"},
		"received_by":      {"조직장36A"},
		"receipt_datetime": {time.Now().Format("2006-01-02T15:04")},
		"eq_asset_id":      {""},
		"eq_symptom":       {"그룹 담당 저장"},
		"eq_assigned_code": {""},
		"eq_work_group_id": {g.GroupID},
	}
	okG := postForm(t, e, "/as", okGroup, oaCK)
	if okG.Code != http.StatusSeeOther || strings.Contains(okG.Header().Get("Location"), "err=") {
		t.Fatalf("그룹 담당 저장 loc=%s code=%d", okG.Header().Get("Location"), okG.Code)
	}
	gotG, err := asRepo.GetByID(repository.OrgAll, asIDFromHref(okG.Header().Get("Location")))
	if err != nil || gotG == nil {
		t.Fatalf("get group: %v", err)
	}
	if strings.TrimSpace(gotG.WorkGroupID) != g.GroupID {
		t.Fatalf("work_group_id=%s", gotG.WorkGroupID)
	}

	okCustom := url.Values{
		"customer_id":        {"cust_a"},
		"received_by":        {"조직장36A"},
		"receipt_datetime":   {time.Now().Format("2006-01-02T15:04")},
		"eq_asset_id":        {""},
		"eq_symptom":         {"직접입력 저장"},
		"eq_assigned_code":   {"custom"},
		"eq_assigned_custom": {"외부기사"},
		"eq_work_group_id":   {""},
	}
	okC := postForm(t, e, "/as", okCustom, oaCK)
	if okC.Code != http.StatusSeeOther || strings.Contains(okC.Header().Get("Location"), "err=") {
		t.Fatalf("직접입력 저장 loc=%s", okC.Header().Get("Location"))
	}
	gotC, err := asRepo.GetByID(repository.OrgAll, asIDFromHref(okC.Header().Get("Location")))
	if err != nil || gotC == nil {
		t.Fatalf("get custom: %v", err)
	}
	if gotC.AssignedTo != "외부기사" || gotC.AssignedUserID != "" {
		t.Fatalf("custom AssignedTo=%s uid=%s", gotC.AssignedTo, gotC.AssignedUserID)
	}

	supNew := doGetWith(t, e, "/as/new", supCK)
	if !strings.Contains(supNew.Body.String(), `value="`+sup.UserID+`" selected`) {
		t.Fatal("지원 기본 담당자가 본인이 아님")
	}
	supOK := postForm(t, e, "/as", url.Values{
		"customer_id":      {"cust_a"},
		"received_by":      {"지원36A"},
		"receipt_datetime": {time.Now().Format("2006-01-02T15:04")},
		"eq_symptom":       {"지원 본인"},
		"eq_assigned_code": {sup.UserID},
	}, supCK)
	if supOK.Code != http.StatusSeeOther || strings.Contains(supOK.Header().Get("Location"), "err=") {
		t.Fatalf("지원 본인 저장 loc=%s", supOK.Header().Get("Location"))
	}
	supMiss := postForm(t, e, "/as", url.Values{
		"customer_id":      {"cust_a"},
		"received_by":      {"지원36A"},
		"receipt_datetime": {time.Now().Format("2006-01-02T15:04")},
		"eq_symptom":       {"지원 비움"},
		"eq_assigned_code": {""},
		"eq_work_group_id": {""},
	}, supCK)
	if supMiss.Code == http.StatusSeeOther && !strings.Contains(supMiss.Header().Get("Location"), "err=") {
		t.Fatal("지원이 담당자를 비웠는데 저장됐다")
	}

	techOK := postForm(t, e, "/as", url.Values{
		"customer_id":      {"cust_a"},
		"received_by":      {"기술36A"},
		"receipt_datetime": {time.Now().Format("2006-01-02T15:04")},
		"eq_symptom":       {"기술은 담당 선택 없이도"},
		"eq_assigned_code": {""},
	}, techCK)
	if techOK.Code != http.StatusSeeOther || strings.Contains(techOK.Header().Get("Location"), "err=") {
		t.Fatalf("tech 기존 동작 깨짐 loc=%s code=%d", techOK.Header().Get("Location"), techOK.Code)
	}
}

func Test95_36A_ScreenAssigneeFocusConsistency(t *testing.T) {
	root := "web/templates"
	if _, err := os.Stat(root); err != nil {
		root = "server/web/templates"
	}
	read := func(rel string) string {
		t.Helper()
		b, err := os.ReadFile(root + "/" + rel)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	form := read("as/form.html")
	if !strings.Contains(form, `name="eq_assigned_code"`) || !strings.Contains(form, `data-focus-target="assignee"`) {
		t.Fatal("as/form 장비 행 포커스 대상")
	}
	if !strings.Contains(form, `name="eq_work_group_id"`) {
		t.Fatal("as/form eq_work_group_id")
	}
	modals := read("workboard/_modals.html")
	if countSubstr(modals, `data-focus-target="assignee"`) < 4 {
		t.Fatal("업무등록 모달 4종에 포커스 대상이 부족하다")
	}
	if !strings.Contains(modals, `data-require-assignee`) {
		t.Fatal("행정·작업 모달에 data-require-assignee 가 없다")
	}
	task := read("workboard/task_show.html")
	if !strings.Contains(task, `data-focus-target="assignee"`) {
		t.Fatal("업무 상세 담당자에 포커스 대상이 없다")
	}
	mnt := read("maintenance/_assignee_multi.html")
	if !strings.Contains(mnt, `data-focus-target="assignee"`) {
		t.Fatal("방문 추가 복수담당에 포커스 대상이 없다")
	}
	plan := read("maintenance/plan_show.html")
	if !strings.Contains(plan, `data-require-assignee`) {
		t.Fatal("방문 추가 폼에 data-require-assignee 가 없다")
	}
	weekly := read("stats/_assignee_field.html")
	if strings.Contains(weekly, `data-require-assignee`) {
		t.Fatal("95-32 사업 연결 팝업 담당자는 optional 이어야 한다")
	}
}
