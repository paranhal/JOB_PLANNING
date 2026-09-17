package repository

import (
	"database/sql"
	"log"
)

// work_task_members 트리거 이름. 적용·테스트에서 DROP 순서로 쓴다.
var workTaskMemberTriggers = []string{
	"trg_wt_insert_owner_member",
	"trg_wt_update_owner_assignee",
	"trg_wt_delete_members",
	"trg_wtm_reject_second_owner",
	"trg_wtm_require_owner_on_support",
	"trg_wtm_reject_owner_role_upd",
	"trg_wtm_reject_demote_last_owner",
	"trg_wtm_reject_delete_last_owner",
}

const workTaskMembersSchema = `
CREATE TABLE IF NOT EXISTS work_task_members (
  task_id      TEXT NOT NULL,
  assignee     TEXT NOT NULL DEFAULT '',
  user_id      TEXT NOT NULL DEFAULT '',
  member_role  TEXT NOT NULL CHECK (member_role IN ('owner', 'support')),
  duration_min INTEGER,
  sort_order   INTEGER NOT NULL DEFAULT 0,
  created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (task_id, assignee)
)`

const workTaskMembersOwnerMsg = "주담당은 정확히 1명이어야 합니다"

// applyWorkTaskMembers 마이그레이션 010. §7.7.2
// work_tasks.assignee 는 주담당으로 유지한다. 참여자는 별도 테이블에 더한다.
func applyWorkTaskMembers(db *sql.DB) {
	if db == nil {
		return
	}
	dropWorkTaskMemberTriggers(db)
	if _, err := db.Exec(workTaskMembersSchema); err != nil {
		log.Printf("010 work_task_members: %v", err)
		return
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_work_task_members_assignee ON work_task_members(assignee)`); err != nil {
		log.Printf("010 work_task_members assignee index: %v", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_work_task_members_role ON work_task_members(task_id, member_role)`); err != nil {
		log.Printf("010 work_task_members role index: %v", err)
	}
	n := execRows(db, `
INSERT OR IGNORE INTO work_task_members (task_id, assignee, member_role, duration_min, sort_order, created_at)
SELECT t.task_id,
       COALESCE(t.assignee, ''),
       'owner',
       t.duration_min,
       0,
       COALESCE(t.created_at, CURRENT_TIMESTAMP)
  FROM work_tasks t
 WHERE NOT EXISTS (
       SELECT 1 FROM work_task_members m WHERE m.task_id = t.task_id
 )`)
	if n > 0 {
		log.Printf("010 work_task_members backfill: %d", n)
	}
	installWorkTaskMemberTriggers(db)
}

func dropWorkTaskMemberTriggers(db *sql.DB) {
	for _, name := range workTaskMemberTriggers {
		if _, err := db.Exec(`DROP TRIGGER IF EXISTS ` + name); err != nil {
			log.Printf("010 drop %s: %v", name, err)
		}
	}
}

func installWorkTaskMemberTriggers(db *sql.DB) {
	uidSQL := ownerMemberUserIDSQL("NEW")
	insertSQL := `CREATE TRIGGER trg_wt_insert_owner_member
AFTER INSERT ON work_tasks
FOR EACH ROW
BEGIN
  INSERT INTO work_task_members (task_id, assignee, member_role, duration_min, sort_order, created_at)
  VALUES (NEW.task_id, COALESCE(NEW.assignee, ''), 'owner', NEW.duration_min, 0, CURRENT_TIMESTAMP);
END`
	updateSQL := `CREATE TRIGGER trg_wt_update_owner_assignee
AFTER UPDATE OF assignee ON work_tasks
FOR EACH ROW
WHEN COALESCE(NEW.assignee, '') != COALESCE(OLD.assignee, '')
BEGIN
  UPDATE work_task_members
     SET assignee = COALESCE(NEW.assignee, '')
   WHERE task_id = NEW.task_id AND member_role = 'owner';
END`
	if tableHasColumn(db, "work_task_members", "user_id") && tableHasColumn(db, "work_tasks", "assignee_user_id") {
		insertSQL = `CREATE TRIGGER trg_wt_insert_owner_member
AFTER INSERT ON work_tasks
FOR EACH ROW
BEGIN
  INSERT INTO work_task_members (task_id, assignee, user_id, member_role, duration_min, sort_order, created_at)
  VALUES (NEW.task_id, COALESCE(NEW.assignee, ''), ` + uidSQL + `, 'owner', NEW.duration_min, 0, CURRENT_TIMESTAMP);
END`
		updateSQL = `CREATE TRIGGER trg_wt_update_owner_assignee
AFTER UPDATE OF assignee, assignee_user_id ON work_tasks
FOR EACH ROW
WHEN COALESCE(NEW.assignee, '') != COALESCE(OLD.assignee, '')
  OR COALESCE(NEW.assignee_user_id, '') != COALESCE(OLD.assignee_user_id, '')
BEGIN
  UPDATE work_task_members
     SET assignee = COALESCE(NEW.assignee, ''),
         user_id = ` + uidSQL + `
   WHERE task_id = NEW.task_id AND member_role = 'owner';
END`
	}
	for _, q := range []string{
		`CREATE TRIGGER trg_wtm_reject_second_owner
BEFORE INSERT ON work_task_members
FOR EACH ROW
WHEN NEW.member_role = 'owner'
 AND (SELECT COUNT(*) FROM work_task_members WHERE task_id = NEW.task_id AND member_role = 'owner') >= 1
BEGIN
  SELECT RAISE(ABORT, '` + workTaskMembersOwnerMsg + `');
END`,
		`CREATE TRIGGER trg_wtm_require_owner_on_support
BEFORE INSERT ON work_task_members
FOR EACH ROW
WHEN NEW.member_role = 'support'
 AND (SELECT COUNT(*) FROM work_task_members WHERE task_id = NEW.task_id AND member_role = 'owner') = 0
BEGIN
  SELECT RAISE(ABORT, '` + workTaskMembersOwnerMsg + `');
END`,
		`CREATE TRIGGER trg_wtm_reject_owner_role_upd
BEFORE UPDATE ON work_task_members
FOR EACH ROW
WHEN NEW.member_role = 'owner'
 AND OLD.member_role != 'owner'
 AND (SELECT COUNT(*) FROM work_task_members WHERE task_id = NEW.task_id AND member_role = 'owner') >= 1
BEGIN
  SELECT RAISE(ABORT, '` + workTaskMembersOwnerMsg + `');
END`,
		`CREATE TRIGGER trg_wtm_reject_demote_last_owner
BEFORE UPDATE ON work_task_members
FOR EACH ROW
WHEN OLD.member_role = 'owner' AND NEW.member_role != 'owner'
 AND (SELECT COUNT(*) FROM work_task_members
       WHERE task_id = OLD.task_id AND member_role = 'owner' AND assignee != OLD.assignee) = 0
BEGIN
  SELECT RAISE(ABORT, '` + workTaskMembersOwnerMsg + `');
END`,
		`CREATE TRIGGER trg_wtm_reject_delete_last_owner
BEFORE DELETE ON work_task_members
FOR EACH ROW
WHEN OLD.member_role = 'owner'
 AND EXISTS (SELECT 1 FROM work_tasks WHERE task_id = OLD.task_id)
BEGIN
  SELECT RAISE(ABORT, '` + workTaskMembersOwnerMsg + `');
END`,
		insertSQL,
		updateSQL,
		`CREATE TRIGGER trg_wt_delete_members
AFTER DELETE ON work_tasks
FOR EACH ROW
BEGIN
  DELETE FROM work_task_members WHERE task_id = OLD.task_id;
END`,
	} {
		if _, err := db.Exec(q); err != nil {
			log.Printf("010 work_task_members trigger: %v", err)
		}
	}
}
