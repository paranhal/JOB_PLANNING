package repository

import (
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/audit"
	"customer-support/internal/model"
)

func TestLookupUsernameConflictKinds(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "uname-cf.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	audit.Init(db)
	users := NewUserRepo(db)
	acc := NewAccountRepo(db)

	live := &model.User{Username: "live72", PasswordHash: "x", FullName: "사용중", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary}
	gone := &model.User{Username: "gone72", PasswordHash: "x", FullName: "삭제됨", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary}
	ren := &model.User{Username: "old72", PasswordHash: "x", FullName: "변경", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary}
	for _, u := range []*model.User{live, gone, ren} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	if err := users.SoftDelete(gone.UserID, "actor", "퇴사"); err != nil {
		t.Fatal(err)
	}
	if err := users.RenameUsername(ren.UserID, "new72"); err != nil {
		t.Fatal(err)
	}
	if err := acc.Reserve("hold72", "U-HOLD", "hold"); err != nil {
		t.Fatal(err)
	}

	assertKind := func(name, except, want string) {
		t.Helper()
		cf, err := users.LookupUsernameConflict(name, except)
		if err != nil {
			t.Fatalf("%s err=%v", name, err)
		}
		if cf.Kind != want {
			t.Fatalf("%s kind=%q want %q", name, cf.Kind, want)
		}
	}
	assertKind("live72", "", model.UsernameConflictLive)
	assertKind("GONE72", "", model.UsernameConflictDeleted)
	assertKind("old72", "", model.UsernameConflictRenamed)
	assertKind("hold72", "", model.UsernameConflictReservedOrphan)
	assertKind("live72", live.UserID, "")
	assertKind("fresh72", "", "")
	assertKind("old72", ren.UserID, "")

	taken, err := users.UsernameTaken("gone72", "")
	if err != nil || !taken {
		t.Fatalf("taken=%v err=%v", taken, err)
	}
}

func Test95_73_LookupReleaseRenameWsg(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "uname-73.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	audit.Init(db)
	users := NewUserRepo(db)
	acc := NewAccountRepo(db)

	owner := &model.User{Username: "fa_admin", PasswordHash: "x", FullName: "경영지원팀관리자", Role: model.RoleOffice, IsActive: true, OrgID: model.OrgIDLibrary, Permissions: model.PermUsersEdit}
	tech := &model.User{Username: "wsg1", PasswordHash: "x", FullName: "우섬길", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, Permissions: model.PermASProcess}
	for _, u := range []*model.User{owner, tech} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	if err := acc.Reserve("wsg", owner.UserID, model.ReserveRenamedFrom); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE username_reservations SET created_at=? WHERE username_norm=?`, "2026-10-27 12:45:10", "wsg"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_tasks (task_id, work_type, title, assignee, assignee_user_id, status) VALUES ('WT73','admin','현장','우섬길',?,'waiting')`, tech.UserID); err != nil {
		t.Fatal(err)
	}

	cf, err := users.LookupUsernameConflict("wsg", tech.UserID)
	if err != nil || cf.Kind != model.UsernameConflictRenamed {
		t.Fatalf("kind %+v err=%v", cf, err)
	}
	if cf.OwnerUsername != "fa_admin" || cf.OwnerName != "경영지원팀관리자" || cf.ReservedAt[:10] != "2026-10-27" {
		t.Fatalf("owner %+v", cf)
	}
	msg := cf.VisionMessage()
	if !strings.Contains(msg, "wsg 는 2026-10-27") || !strings.Contains(msg, "fa_admin") {
		t.Fatalf("msg=%s", msg)
	}
	if cf.PublicMessage() != model.UsernameInquiryMsg {
		t.Fatal("public")
	}

	own, err := users.LookupUsernameConflict("wsg", owner.UserID)
	if err != nil || !own.Empty() {
		t.Fatalf("Q-1 본인 되돌리기 %+v err=%v", own, err)
	}

	orphan, err := users.LookupUsernameConflict("ghost73", "")
	if err != nil || !orphan.Empty() {
		t.Fatalf("없는 아이디 %+v", orphan)
	}
	if err := acc.Reserve("ghost73", "U-NONE", "hold"); err != nil {
		t.Fatal(err)
	}
	og, _ := users.LookupUsernameConflict("ghost73", "")
	if og.Kind != model.UsernameConflictReservedOrphan {
		t.Fatalf("orphan %q", og.Kind)
	}

	if err := users.ReleaseUsernameReservation("fa_admin", owner.UserID, "잘못", ""); err == nil {
		t.Fatal("current 해제")
	}
	if err := users.ReleaseUsernameReservation(goneUsername(t, users), "actor", "잘못", ""); err == nil {
		t.Fatal("삭제 아이디 해제")
	}

	if err := users.ReleaseUsernameReservation("wsg", "actor", "운영 재현", "NOPE"); err == nil {
		t.Fatal("없는 계정 적용이 성공")
	}
	still, _ := users.LookupUsernameConflict("wsg", tech.UserID)
	if still.Kind != model.UsernameConflictRenamed {
		t.Fatal("롤백 실패")
	}

	if err := users.ReleaseUsernameReservation("wsg", "actor", "우섬길 실사용", tech.UserID); err != nil {
		t.Fatal(err)
	}
	back, _ := users.GetByID(tech.UserID)
	if back == nil || back.Username != "wsg" || back.UserID != tech.UserID || back.Permissions != model.PermASProcess {
		t.Fatalf("변경 후 %+v", back)
	}
	if users.AssignedWorkCount(tech.UserID) < 1 {
		t.Fatal("업무 user_id 가 바뀌었다")
	}
	wsgCF, _ := users.LookupUsernameConflict("wsg", tech.UserID)
	if !wsgCF.Empty() {
		t.Fatalf("wsg current %+v", wsgCF)
	}
	oldCF, _ := users.LookupUsernameConflict("wsg1", "")
	if oldCF.Kind != model.UsernameConflictRenamed || oldCF.UserID != tech.UserID {
		t.Fatalf("wsg1 예약 %+v", oldCF)
	}
	fa, _ := acc.GetReservation("fa_admin")
	if fa == nil || fa.Reason != model.ReserveCurrent || fa.UserID != owner.UserID {
		t.Fatalf("fa_admin 예약 %+v", fa)
	}
	var nLife, nAudit int
	_ = db.QueryRow(`SELECT COUNT(*) FROM user_lifecycle_events WHERE kind=? AND detail LIKE '%wsg%'`, model.LifeReservationReleased).Scan(&nLife)
	if nLife < 1 {
		t.Fatal("lifecycle 없음")
	}
	_ = db.QueryRow(`SELECT COUNT(*) FROM data_change_logs WHERE table_name='username_reservations' AND entity_id='wsg'`).Scan(&nAudit)
	if nAudit < 1 {
		t.Fatal("audit 없음")
	}
}

func goneUsername(t *testing.T, users *UserRepo) string {
	t.Helper()
	u := &model.User{Username: "gone73rel", PasswordHash: "x", FullName: "삭제73", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary}
	if err := users.Create(u); err != nil {
		t.Fatal(err)
	}
	if err := users.SoftDelete(u.UserID, "actor", "퇴사"); err != nil {
		t.Fatal(err)
	}
	return "gone73rel"
}
