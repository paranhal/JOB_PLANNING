package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// 95-200: 하위 업무 「업무 모두 종료」 — 이 회차 완료 + 이후 일정 제외(+status cancelled) + 상위 업무 완료.

type occRow struct {
	ID, Occ, Date string
}

// mkRecurring 상위 1건 + 회차 n건(2026-11-01 부터 하루씩). occ[i] 로 회차별 occurrence_status 를 지정한다(” 는 scheduled).
func mkRecurring(t *testing.T, v env191, parent, policy string, occ []string) []string {
	t.Helper()
	db := v.wb.DB()
	ins := func(id, role, par, occSt, status, date string) {
		var d interface{} = date
		if date == "" {
			d = nil
		}
		if _, err := db.Exec(`INSERT INTO work_tasks(task_id,work_type,title,status,org_id,work_date,due_date,start_time,end_time,assignee,assignee_user_id,recurrence_role,parent_task_id,occurrence_status,progress)
			VALUES (?,'admin',?,?,?,?,?,?,?,?,?,?,?,?,0)`, id, "반복-"+id, status, model.OrgIDLibrary, d, d, "09:00", "09:30",
			v.owner.FullName, v.owner.UserID, role, nullStr198(par), nullStr198(occSt)); err != nil {
			t.Fatal(err)
		}
	}
	ins(parent, "parent", "", "", "todo", "2026-11-01")
	if _, err := db.Exec(`INSERT INTO work_recurrence(task_id,rule_type,complete_policy) VALUES (?,?,?)`, parent, "daily", policy); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for i, st := range occ {
		id := fmt.Sprintf("%s-%02d", parent, i+1)
		date := fmt.Sprintf("2026-11-%02d", i+1)
		status, occSt := "todo", st
		switch st {
		case "complete":
			status = "complete"
		case "in_progress":
			status = "in_progress"
		case "":
			occSt = "scheduled"
		case "nodate":
			occSt, date = "scheduled", ""
		}
		ins(id, "occurrence", parent, occSt, status, date)
		if st == "complete" {
			_, _ = db.Exec(`UPDATE work_tasks SET complete_date='2026-11-0' || ?, complete_note='원래 메모', progress=100 WHERE task_id=?`, i+1, id)
		}
		if st == "skipped" {
			_, _ = db.Exec(`UPDATE work_tasks SET not_done_reason='기존 제외 사유' WHERE task_id=?`, id)
		}
		ids = append(ids, id)
	}
	return ids
}

func nullStr198(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

type taskState struct{ Status, Occ, Reason, CompleteDate, Note string }

func stateOf(t *testing.T, db *sql.DB, id string) taskState {
	t.Helper()
	var s taskState
	if err := db.QueryRow(`SELECT COALESCE(status,''), COALESCE(occurrence_status,''), COALESCE(not_done_reason,''), COALESCE(complete_date,''), COALESCE(complete_note,'') FROM work_tasks WHERE task_id=?`, id).
		Scan(&s.Status, &s.Occ, &s.Reason, &s.CompleteDate, &s.Note); err != nil {
		t.Fatal(err)
	}
	return s
}

func closeAll(v env191, parent, occ string, form url.Values, u *model.User) *httpRecorder198 {
	r := v.do("POST", "/workboard/tasks/"+parent+"/occurrences/"+occ+"/close-all", form, u)
	return &httpRecorder198{Code: r.Code, Loc: r.Header().Get("Location"), Body: r.Body.String()}
}

type httpRecorder198 struct {
	Code int
	Loc  string
	Body string
}

func okForm(reason string) url.Values {
	return url.Values{"close_reason": {reason}, "earlier_sent": {"1"}, "include_earlier": {"1"}}
}

func fifteen(done ...int) []string {
	occ := make([]string, 15)
	for _, i := range done {
		occ[i-1] = "complete"
	}
	return occ
}

// 재현: 중간 회차를 끝내도 남은 일정이 미완료로 남고 상위가 완료되지 않는다(기존 동작).
func TestHTTP95_200_Repro_OpenOccurrencesRemain(t *testing.T) {
	v := setup191(t)
	ids := mkRecurring(t, v, "P200R", "manual", fifteen())
	r := v.do("POST", "/workboard/tasks/P200R/occurrences/"+ids[2], url.Values{"occurrence_status": {"complete"}}, v.admin)
	if r.Code != http.StatusSeeOther {
		t.Fatalf("완료 %d", r.Code)
	}
	if n, _ := v.wb.CountOpenOccurrences("P200R"); n != 14 {
		t.Fatalf("남은 열린 회차 %d", n)
	}
	if stateOf(t, v.wb.DB(), "P200R").Status == "complete" {
		t.Fatal("재현 실패: 상위가 이미 완료됐다")
	}
}

func TestHTTP95_200_CloseAll_Manual(t *testing.T) {
	v := setup191(t)
	db := v.wb.DB()
	// 1·2번째는 이미 완료, 3번째에서 종료, 4~15번째(12건)는 이후
	ids := mkRecurring(t, v, "P200A", "manual", fifteen(1, 2))
	before1, before2 := stateOf(t, db, ids[0]), stateOf(t, db, ids[1])
	r := closeAll(v, "P200A", ids[2], okForm("고객 요청으로 종료"), v.admin)
	if r.Code != http.StatusSeeOther || !strings.Contains(r.Loc, "ok=rec_closed") || !strings.Contains(r.Loc, "n=12") || strings.Contains(r.Loc, "partial") {
		t.Fatalf("종료 %d %s", r.Code, r.Loc)
	}
	if s := stateOf(t, db, ids[2]); s.Status != "complete" || s.Occ != "complete" || s.CompleteDate == "" {
		t.Fatalf("기준 회차 %+v", s)
	}
	for _, id := range ids[3:] {
		s := stateOf(t, db, id)
		if s.Occ != "skipped" || s.Status != "cancelled" || !strings.Contains(s.Reason, "업무 조기 종료: 고객 요청으로 종료") {
			t.Fatalf("%s 제외 상태 %+v", id, s)
		}
	}
	if after := stateOf(t, db, ids[0]); after != before1 {
		t.Fatalf("이미 완료한 1번째가 바뀌었다 %+v → %+v", before1, after)
	}
	if after := stateOf(t, db, ids[1]); after != before2 {
		t.Fatalf("이미 완료한 2번째가 바뀌었다")
	}
	p := stateOf(t, db, "P200A")
	if p.Status != "complete" || !strings.Contains(p.Note, "업무 조기 종료: 고객 요청으로 종료") || p.CompleteDate == "" {
		t.Fatalf("상위 %+v", p)
	}
	var prog int
	_ = db.QueryRow(`SELECT progress FROM work_tasks WHERE task_id='P200A'`).Scan(&prog)
	if prog != 100 {
		t.Fatalf("상위 진행률 %d", prog)
	}
	// 결과 화면 플래시
	body := v.do("GET", "/workboard/tasks/P200A?ok=rec_closed&n=12", nil, v.admin).Body.String()
	if !strings.Contains(body, "12건 제외, 상위 업무를 완료했습니다") {
		t.Fatal("결과 플래시가 없다")
	}
}

func TestHTTP95_200_PoliciesAuto_RequireResult(t *testing.T) {
	v := setup191(t)
	db := v.wb.DB()
	// auto
	ids := mkRecurring(t, v, "P200B", "auto", fifteen())
	if r := closeAll(v, "P200B", ids[4], okForm("종료"), v.admin); !strings.Contains(r.Loc, "ok=rec_closed") {
		t.Fatalf("auto %s", r.Loc)
	}
	if stateOf(t, db, "P200B").Status != "complete" {
		t.Fatal("auto: 상위가 완료되지 않았다")
	}
	// require_result: 최종 결과가 비면 거절하고 아무것도 바뀌지 않는다
	ids = mkRecurring(t, v, "P200C", "require_result", fifteen())
	var pv struct {
		NeedFinalResult bool `json:"needFinalResult"`
	}
	r := v.do("GET", "/workboard/tasks/P200C/occurrences/"+ids[1]+"/close-all/preview", nil, v.admin)
	_ = json.Unmarshal(r.Body.Bytes(), &pv)
	if !pv.NeedFinalResult {
		t.Fatalf("require_result 인데 최종 결과 필요 표시가 없다 %s", r.Body.String())
	}
	if cr := closeAll(v, "P200C", ids[1], okForm("종료"), v.admin); !strings.Contains(cr.Loc, "err=rec_result") {
		t.Fatalf("최종 결과 없이 %s", cr.Loc)
	}
	if n, _ := v.wb.CountOpenOccurrences("P200C"); n != 15 || stateOf(t, db, "P200C").Status == "complete" {
		t.Fatalf("거절됐는데 상태가 바뀌었다 open=%d", n)
	}
	f := okForm("종료")
	f.Set("final_result", "신구대조표 최종본 제출")
	if cr := closeAll(v, "P200C", ids[1], f, v.admin); !strings.Contains(cr.Loc, "ok=rec_closed") {
		t.Fatalf("최종 결과 입력 %s", cr.Loc)
	}
	rule, _ := v.wb.GetRecurrence("P200C")
	if rule == nil || rule.FinalResult != "신구대조표 최종본 제출" || stateOf(t, db, "P200C").Status != "complete" {
		t.Fatalf("최종 결과 저장/완료 %+v", rule)
	}
	// manual 에서는 최종 결과 입력칸이 필요 없다
	mkRecurring(t, v, "P200D", "manual", fifteen())
	r = v.do("GET", "/workboard/tasks/P200D/occurrences/P200D-02/close-all/preview", nil, v.admin)
	_ = json.Unmarshal(r.Body.Bytes(), &pv)
	if pv.NeedFinalResult {
		t.Fatal("manual 인데 최종 결과가 필요하다고 나온다")
	}
}

func TestHTTP95_200_EarlierOpen(t *testing.T) {
	v := setup191(t)
	db := v.wb.DB()
	// 1·2번째는 열려 있고 3번째에서 종료
	ids := mkRecurring(t, v, "P200E", "manual", fifteen())
	// 기본(체크 on): 함께 제외 → 상위 완료
	r := closeAll(v, "P200E", ids[2], okForm("종료"), v.admin)
	if !strings.Contains(r.Loc, "ok=rec_closed&n=14") {
		t.Fatalf("함께 제외 %s", r.Loc)
	}
	for _, id := range ids[:2] {
		if s := stateOf(t, db, id); s.Occ != "skipped" {
			t.Fatalf("이전 회차 %s 제외 안 됨 %+v", id, s)
		}
	}
	// 체크 해제: 이전 미완료는 열린 채, 이후만 제외, 상위는 진행 중 + 안내
	ids = mkRecurring(t, v, "P200F", "manual", fifteen())
	f := url.Values{"close_reason": {"종료"}, "earlier_sent": {"1"}} // include_earlier 없음 = 해제
	r = closeAll(v, "P200F", ids[2], f, v.admin)
	if !strings.Contains(r.Loc, "ok=rec_closed_partial") || !strings.Contains(r.Loc, "n=12") || !strings.Contains(r.Loc, "keep=2") {
		t.Fatalf("해제 %s", r.Loc)
	}
	for _, id := range ids[:2] {
		if s := stateOf(t, db, id); s.Occ == "skipped" || s.Status == "cancelled" {
			t.Fatalf("해제했는데 이전 회차가 제외됐다 %s %+v", id, s)
		}
	}
	if stateOf(t, db, "P200F").Status == "complete" {
		t.Fatal("이전 미완료가 남았는데 상위가 완료됐다")
	}
	body := v.do("GET", "/workboard/tasks/P200F?ok=rec_closed_partial&n=12&keep=2", nil, v.admin).Body.String()
	if !strings.Contains(body, "이전 미완료 2건이 남아 있어 상위 업무는 진행 중으로 둡니다") {
		t.Fatal("안내 문구가 없다")
	}
	// 미리보기는 이전 미완료 2건·이후 12건
	var pv repository.CloseOccPreview
	pr := v.do("GET", "/workboard/tasks/P200F/occurrences/"+ids[2]+"/close-all/preview", nil, v.admin)
	if err := json.Unmarshal(pr.Body.Bytes(), &pv); err != nil || pv.Earlier != 2 {
		t.Fatalf("미리보기(종료 뒤) %+v %v", pv, err)
	}
}

func TestHTTP95_200_StatusMix(t *testing.T) {
	v := setup191(t)
	db := v.wb.DB()
	// 1=기준(진행중), 2=연기, 3=보류, 4=진행중, 5=미완료(overdue), 6=날짜 없음, 7=이미 제외, 8=이미 완료
	occ := []string{"in_progress", "deferred", "hold", "in_progress", "overdue", "nodate", "skipped", "complete"}
	ids := mkRecurring(t, v, "P200G", "manual", occ)
	before7, before8 := stateOf(t, db, ids[6]), stateOf(t, db, ids[7])
	var pv repository.CloseOccPreview
	pr := v.do("GET", "/workboard/tasks/P200G/occurrences/"+ids[0]+"/close-all/preview", nil, v.admin)
	if err := json.Unmarshal(pr.Body.Bytes(), &pv); err != nil {
		t.Fatal(err)
	}
	if pv.Later != 5 || pv.LaterByStatus["deferred"] != 1 || pv.LaterByStatus["hold"] != 1 || pv.LaterByStatus["in_progress"] != 1 || pv.LaterByStatus["overdue"] != 1 || pv.LaterByStatus["scheduled"] != 1 {
		t.Fatalf("미리보기 %+v", pv)
	}
	if r := closeAll(v, "P200G", ids[0], okForm("종료"), v.admin); !strings.Contains(r.Loc, "n=5") {
		t.Fatalf("종료 %s", r.Loc)
	}
	for _, id := range ids[1:6] {
		if s := stateOf(t, db, id); s.Occ != "skipped" || s.Status != "cancelled" {
			t.Fatalf("%s %+v", id, s)
		}
	}
	if stateOf(t, db, ids[6]) != before7 || stateOf(t, db, ids[7]) != before8 {
		t.Fatal("이미 제외·완료된 회차가 바뀌었다")
	}
}

func TestHTTP95_200_PermissionsAndStates(t *testing.T) {
	v := setup191(t)
	db := v.wb.DB()
	ids := mkRecurring(t, v, "P200H", "manual", fifteen())
	other := mkRecurring(t, v, "P200I", "manual", fifteen())
	// 사유가 비면 거절(서버 검증)하고 아무것도 바뀌지 않는다
	for _, reason := range []string{"", "   "} {
		r := closeAll(v, "P200H", ids[2], url.Values{"close_reason": {reason}, "earlier_sent": {"1"}, "include_earlier": {"1"}}, v.admin)
		if r.Code != http.StatusSeeOther || !strings.Contains(r.Loc, "err=rec_close_reason") {
			t.Fatalf("사유 없음 %d %s", r.Code, r.Loc)
		}
	}
	if n, _ := v.wb.CountOpenOccurrences("P200H"); n != 15 {
		t.Fatalf("사유 없는데 바뀌었다 %d", n)
	}
	// 다른 상위의 회차 id 를 섞으면 404
	if r := closeAll(v, "P200H", other[2], okForm("종료"), v.admin); r.Code != http.StatusNotFound {
		t.Fatalf("다른 상위의 회차 %d", r.Code)
	}
	// 반복 업무가 아닌 id / 회차가 아닌 id
	if r := closeAll(v, "P200H", "P200H", okForm("종료"), v.admin); r.Code != http.StatusNotFound {
		t.Fatalf("상위 id 를 회차로 %d", r.Code)
	}
	// 타 조직 사용자는 거절
	if r := closeAll(v, "P200H", ids[2], okForm("종료"), v.other); r.Code == http.StatusSeeOther || r.Code == http.StatusOK {
		t.Fatalf("타 조직 사용자가 종료했다 %d", r.Code)
	}
	// 읽기 전용 사용자 거절
	ro := mkReadOnly200(t, v)
	if r := closeAll(v, "P200H", ids[2], okForm("종료"), ro); r.Code == http.StatusSeeOther || r.Code == http.StatusOK {
		t.Fatalf("읽기 전용이 종료했다 %d", r.Code)
	}
	if n, _ := v.wb.CountOpenOccurrences("P200H"); n != 15 {
		t.Fatalf("거절됐는데 바뀌었다 %d", n)
	}
	// 정상 → 두 번째 전송은 한 번만 반영(이미 종료)
	if r := closeAll(v, "P200H", ids[2], okForm("종료"), v.admin); !strings.Contains(r.Loc, "ok=rec_closed") {
		t.Fatalf("정상 %s", r.Loc)
	}
	r2 := closeAll(v, "P200H", ids[2], okForm("종료"), v.admin)
	if !strings.Contains(r2.Loc, "err=rec_closed_already") {
		t.Fatalf("재전송 %s", r2.Loc)
	}
	// 완료·취소·보관된 상위는 거절
	for name, q := range map[string]string{
		"cancelled": `UPDATE work_tasks SET status='cancelled' WHERE task_id=?`,
		"archived":  `UPDATE work_recurrence SET archived=1 WHERE task_id=?`,
	} {
		id := "P200-" + name
		ids2 := mkRecurring(t, v, id, "manual", fifteen())
		if _, err := db.Exec(q, id); err != nil {
			t.Fatal(err)
		}
		if r := closeAll(v, id, ids2[2], okForm("종료"), v.admin); !strings.Contains(r.Loc, "err=rec_closed_already") {
			t.Fatalf("%s 상위 %d %s", name, r.Code, r.Loc)
		}
		if pr := v.do("GET", "/workboard/tasks/"+id+"/occurrences/"+ids2[2]+"/close-all/preview", nil, v.admin); pr.Code != http.StatusConflict {
			t.Fatalf("%s 미리보기 %d", name, pr.Code)
		}
	}
}

func TestHTTP95_200_RollbackOnFailure(t *testing.T) {
	v := setup191(t)
	db := v.wb.DB()
	ids := mkRecurring(t, v, "P200J", "manual", fifteen())
	// 마지막 회차를 제외하는 순간 실패하게 만든다 → 앞서 처리한 것까지 전부 되돌아가야 한다
	if _, err := db.Exec(`CREATE TRIGGER trg200_fail BEFORE UPDATE OF occurrence_status ON work_tasks
		WHEN NEW.task_id='` + ids[14] + `' AND NEW.occurrence_status='skipped' BEGIN SELECT RAISE(ABORT,'boom'); END`); err != nil {
		t.Fatal(err)
	}
	r := closeAll(v, "P200J", ids[2], okForm("종료"), v.admin)
	if !strings.Contains(r.Loc, "err=rec_rule") {
		t.Fatalf("실패 응답 %d %s", r.Code, r.Loc)
	}
	if n, _ := v.wb.CountOpenOccurrences("P200J"); n != 15 {
		t.Fatalf("일부만 바뀐 상태가 남았다 open=%d", n)
	}
	if s := stateOf(t, db, ids[2]); s.Status == "complete" {
		t.Fatalf("기준 회차가 완료로 남았다 %+v", s)
	}
	if stateOf(t, db, "P200J").Status == "complete" {
		t.Fatal("상위가 완료로 남았다")
	}
}

func TestHTTP95_200_ButtonVisibility(t *testing.T) {
	v := setup191(t)
	db := v.wb.DB()
	// 열린 회차가 여러 건: 하위 상세 머리 버튼 + 상위 이력 표(열린 회차 행에만)
	ids := mkRecurring(t, v, "P200K", "manual", fifteen(1, 2))
	occPage := v.do("GET", "/workboard/tasks/"+ids[2], nil, v.admin).Body.String()
	if !strings.Contains(occPage, `data-occ-close-all`) || !strings.Contains(occPage, `id="occ-close-modal"`) {
		t.Fatal("하위 업무 상세에 버튼·모달이 없다")
	}
	if !strings.Contains(occPage, "occ_close_all.js?v="+AssetQuery()) {
		t.Fatal("스크립트가 버전 쿼리와 함께 로드되지 않는다(95-198 교훈)")
	}
	parentPage := v.do("GET", "/workboard/tasks/P200K", nil, v.admin).Body.String()
	if n := strings.Count(parentPage, `data-occ-close-all data-parent="P200K"`); n != 13 {
		t.Fatalf("이력 표 버튼 %d개(열린 회차 13건에만 있어야 한다)", n)
	}
	for _, done := range ids[:2] {
		if strings.Contains(parentPage, `data-occ="`+done+`"`) {
			t.Fatalf("완료된 회차 %s 행에 버튼이 있다", done)
		}
	}
	// 열린 회차가 X 하나뿐이면 숨김(기존 「완료」로 충분)
	one := mkRecurring(t, v, "P200L", "manual", []string{"complete", "complete", ""})
	if strings.Contains(v.do("GET", "/workboard/tasks/"+one[2], nil, v.admin).Body.String(), `data-occ-close-all`) {
		t.Fatal("열린 회차가 하나뿐인데 버튼이 보인다(하위)")
	}
	if strings.Contains(v.do("GET", "/workboard/tasks/P200L", nil, v.admin).Body.String(), `data-occ-close-all`) {
		t.Fatal("열린 회차가 하나뿐인데 버튼이 보인다(표)")
	}
	// 상위가 완료면 숨김
	done := mkRecurring(t, v, "P200M", "manual", fifteen())
	if _, err := db.Exec(`UPDATE work_tasks SET status='complete' WHERE task_id='P200M'`); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(v.do("GET", "/workboard/tasks/"+done[2], nil, v.admin).Body.String(), `data-occ-close-all`) {
		t.Fatal("상위가 완료인데 버튼이 보인다")
	}
	// 일반 업무에는 없음
	if strings.Contains(v.do("GET", "/workboard/tasks/"+v.progress, nil, v.admin).Body.String(), `data-occ-close-all`) {
		t.Fatal("일반 업무에 버튼이 있다")
	}
	// 쓰기 권한 없는 사용자에게는 없음
	ro := mkReadOnly200(t, v)
	if strings.Contains(v.do("GET", "/workboard/tasks/"+ids[2], nil, ro).Body.String(), `data-occ-close-all`) {
		t.Fatal("읽기 전용 사용자에게 버튼이 보인다")
	}
}

// 종료된 회차가 지연·진행중 목록에 남지 않고, 지연 표시 갱신이 제외 회차를 되살리지 않는다.
func TestHTTP95_200_ExcludedNotRevived(t *testing.T) {
	v := setup191(t)
	db := v.wb.DB()
	ids := mkRecurring(t, v, "P200N", "manual", fifteen())
	if r := closeAll(v, "P200N", ids[2], okForm("종료"), v.admin); !strings.Contains(r.Loc, "ok=rec_closed") {
		t.Fatal(r.Loc)
	}
	if _, err := v.wb.MarkPastOccurrencesOverdue("2026-12-31"); err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if s := stateOf(t, db, id); s.Occ == "overdue" {
			t.Fatalf("%s 가 지연으로 되살아났다", id)
		}
	}
	board := repository.NewWorkBoardRepo(db)
	for _, b := range []string{model.WorkBucketDelayed, model.WorkBucketInProgress} {
		items, err := board.ListBucketOn(b, "2026-12-31", "", nil, 500)
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range items {
			if strings.HasPrefix(it.RefID, "P200N") {
				t.Fatalf("%s 목록에 종료된 업무 %s 가 남아 있다", b, it.RefID)
			}
		}
	}
}

// 읽기 전용(C-Level) 계정을 실제 DB 사용자로 만든다(토큰 주장만으로는 서버가 DB 사용자를 따른다).
func mkReadOnly200(t *testing.T, v env191) *model.User {
	t.Helper()
	users := repository.NewUserRepo(v.wb.DB())
	u := &model.User{Username: "clv200", PasswordHash: HashPassword("x"), FullName: "읽기전용", Role: model.RoleCLevel,
		IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true, IsReadOnly: true}
	if err := users.Create(u); err != nil {
		got, e2 := users.GetByUsername("clv200")
		if e2 != nil || got == nil {
			t.Fatal(err)
		}
		return got
	}
	return u
}

// 주간보고: 종료된 업무의 제외 회차가 계획·이월(미연결 목록)에 남지 않는다.
func TestHTTP95_200_WeeklyReportExcludesClosedOccurrences(t *testing.T) {
	v := setup191(t)
	db := v.wb.DB()
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order) VALUES ('O01','O01','도서관사업부','도서관',1,1)`); err != nil {
		t.Fatal(err)
	}
	ids := mkRecurring(t, v, "P200W", "manual", fifteen())
	stats := repository.NewStatsRepo(db)
	anchor, _ := time.ParseInLocation("2006-01-02", "2026-11-09", time.Local)
	count := func() int {
		draft, err := stats.BuildCompanyWeeklyDraftOrg(anchor, model.OrgIDLibrary)
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, u := range draft.Unassigned {
			if strings.HasPrefix(u.ID, "P200W") {
				n++
			}
		}
		return n
	}
	if count() == 0 {
		t.Fatal("재현 실패: 종료 전에는 주간보고에 회차가 잡혀야 한다")
	}
	if r := closeAll(v, "P200W", ids[2], okForm("종료"), v.admin); !strings.Contains(r.Loc, "ok=rec_closed") {
		t.Fatal(r.Loc)
	}
	if n := count(); n != 0 {
		t.Fatalf("종료 뒤에도 주간보고 미완료 목록에 회차 %d건이 남는다", n)
	}
}

// require_result 정책이어도 이전 미완료를 남기면 상위를 완료하지 않으므로 최종 결과 없이 종료할 수 있다.
func TestHTTP95_200_RequireResultPartialNeedsNoResult(t *testing.T) {
	v := setup191(t)
	ids := mkRecurring(t, v, "P200P", "require_result", fifteen())
	f := url.Values{"close_reason": {"종료"}, "earlier_sent": {"1"}} // 함께 제외 해제
	r := closeAll(v, "P200P", ids[2], f, v.admin)
	if !strings.Contains(r.Loc, "ok=rec_closed_partial") {
		t.Fatalf("부분 종료 %s", r.Loc)
	}
	if stateOf(t, v.wb.DB(), "P200P").Status == "complete" {
		t.Fatal("상위가 완료됐다")
	}
}
