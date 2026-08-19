-- 기획서 §7.7.2 work_task_members
-- 실제 앱은 InitDB 시 repository.applyWorkTaskMembers 가 동일 CREATE·이관·트리거를 실행한다.
--
-- work_tasks.assignee 는 없애지 않는다. 「주담당」으로 유지한다.
-- 참여자 1명뿐인 업무도 owner 행 1건을 만든다.
--
-- 빈 assignee('') 는 값을 채우지 않고 미배정 상태로 둔다.
-- 통계는 work_tasks.assignee 빈 값을 (미배정) 버킷으로 묶으므로
-- 컬럼을 '미배정'으로 바꾸면 집계가 달라진다. 이 단계의 합격 기준은 통계 불변이다.

CREATE TABLE IF NOT EXISTS work_task_members (
  task_id      TEXT NOT NULL,
  assignee     TEXT NOT NULL DEFAULT '',
  member_role  TEXT NOT NULL CHECK (member_role IN ('owner', 'support')),
  duration_min INTEGER,
  sort_order   INTEGER NOT NULL DEFAULT 0,
  created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (task_id, assignee)
);

CREATE INDEX IF NOT EXISTS idx_work_task_members_assignee ON work_task_members(assignee);
CREATE INDEX IF NOT EXISTS idx_work_task_members_role ON work_task_members(task_id, member_role);

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
 );

-- 트리거는 applyWorkTaskMembers 가 DROP 후 재생성한다. 아래는 문서·수동 적용용.

DROP TRIGGER IF EXISTS trg_wtm_reject_second_owner;
CREATE TRIGGER trg_wtm_reject_second_owner
BEFORE INSERT ON work_task_members
FOR EACH ROW
WHEN NEW.member_role = 'owner'
 AND (SELECT COUNT(*) FROM work_task_members WHERE task_id = NEW.task_id AND member_role = 'owner') >= 1
BEGIN
  SELECT RAISE(ABORT, '주담당은 정확히 1명이어야 합니다');
END;

DROP TRIGGER IF EXISTS trg_wtm_require_owner_on_support;
CREATE TRIGGER trg_wtm_require_owner_on_support
BEFORE INSERT ON work_task_members
FOR EACH ROW
WHEN NEW.member_role = 'support'
 AND (SELECT COUNT(*) FROM work_task_members WHERE task_id = NEW.task_id AND member_role = 'owner') = 0
BEGIN
  SELECT RAISE(ABORT, '주담당은 정확히 1명이어야 합니다');
END;

DROP TRIGGER IF EXISTS trg_wtm_reject_owner_role_upd;
CREATE TRIGGER trg_wtm_reject_owner_role_upd
BEFORE UPDATE ON work_task_members
FOR EACH ROW
WHEN NEW.member_role = 'owner'
 AND OLD.member_role != 'owner'
 AND (SELECT COUNT(*) FROM work_task_members WHERE task_id = NEW.task_id AND member_role = 'owner') >= 1
BEGIN
  SELECT RAISE(ABORT, '주담당은 정확히 1명이어야 합니다');
END;

DROP TRIGGER IF EXISTS trg_wtm_reject_demote_last_owner;
CREATE TRIGGER trg_wtm_reject_demote_last_owner
BEFORE UPDATE ON work_task_members
FOR EACH ROW
WHEN OLD.member_role = 'owner' AND NEW.member_role != 'owner'
 AND (SELECT COUNT(*) FROM work_task_members
       WHERE task_id = OLD.task_id AND member_role = 'owner' AND assignee != OLD.assignee) = 0
BEGIN
  SELECT RAISE(ABORT, '주담당은 정확히 1명이어야 합니다');
END;

DROP TRIGGER IF EXISTS trg_wtm_reject_delete_last_owner;
CREATE TRIGGER trg_wtm_reject_delete_last_owner
BEFORE DELETE ON work_task_members
FOR EACH ROW
WHEN OLD.member_role = 'owner'
 AND EXISTS (SELECT 1 FROM work_tasks WHERE task_id = OLD.task_id)
BEGIN
  SELECT RAISE(ABORT, '주담당은 정확히 1명이어야 합니다');
END;

DROP TRIGGER IF EXISTS trg_wt_insert_owner_member;
CREATE TRIGGER trg_wt_insert_owner_member
AFTER INSERT ON work_tasks
FOR EACH ROW
BEGIN
  INSERT INTO work_task_members (task_id, assignee, member_role, duration_min, sort_order, created_at)
  VALUES (NEW.task_id, COALESCE(NEW.assignee, ''), 'owner', NEW.duration_min, 0, CURRENT_TIMESTAMP);
END;

DROP TRIGGER IF EXISTS trg_wt_update_owner_assignee;
CREATE TRIGGER trg_wt_update_owner_assignee
AFTER UPDATE OF assignee ON work_tasks
FOR EACH ROW
WHEN COALESCE(NEW.assignee, '') != COALESCE(OLD.assignee, '')
BEGIN
  UPDATE work_task_members
     SET assignee = COALESCE(NEW.assignee, '')
   WHERE task_id = NEW.task_id AND member_role = 'owner';
END;

DROP TRIGGER IF EXISTS trg_wt_delete_members;
CREATE TRIGGER trg_wt_delete_members
AFTER DELETE ON work_tasks
FOR EACH ROW
BEGIN
  DELETE FROM work_task_members WHERE task_id = OLD.task_id;
END;
