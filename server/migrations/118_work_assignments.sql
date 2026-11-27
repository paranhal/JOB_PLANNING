-- 95-225: 공동 작업 범위(work_tasks.collab_scope) + 업무 배정(work_assignments).
-- collab_scope: NULL(없음) | 'org'(같은 조직 누구나) | 'members'(지정한 조직원만; work_task_members 재사용)
-- 배정: 하부 업무(sub=package)·사이트(site)·하부 작업(subtask=child work_task)·AS 하부업무(as_work)를 다른 사용자(타 조직 포함)에게 요청한다.
-- 상태 전이: pending → accepted / held / rejected, held → accepted / rejected. accepted·rejected 는 종결.
ALTER TABLE work_tasks ADD COLUMN collab_scope TEXT;

-- 공동 작업 「지정한 조직원만」 범위의 구성원(타 조직원 포함). work_task_members 는 조직 소속을 강제(95-215)하므로 따로 둔다.
CREATE TABLE IF NOT EXISTS work_collab_members (
  task_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  added_by TEXT NOT NULL DEFAULT '',
  added_at TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (task_id, user_id)
);

CREATE TABLE IF NOT EXISTS work_assignments (
  id               TEXT PRIMARY KEY,
  task_id          TEXT NOT NULL,
  target_type      TEXT NOT NULL CHECK (target_type IN ('sub','site','subtask','as_work')),
  target_id        TEXT NOT NULL,
  target_label     TEXT NOT NULL DEFAULT '',
  project_id       TEXT NOT NULL DEFAULT '',
  project_name     TEXT NOT NULL DEFAULT '',   -- 표시용 스냅샷
  activity_kind    TEXT NOT NULL DEFAULT '',   -- 표시용 스냅샷(업무 구분)
  from_user_id     TEXT NOT NULL,
  to_user_id       TEXT NOT NULL,
  status           TEXT NOT NULL CHECK (status IN ('pending','accepted','held','rejected')),
  reject_reason    TEXT NOT NULL DEFAULT '',
  created_at       TEXT NOT NULL,
  decided_at       TEXT NOT NULL DEFAULT '',
  last_notified_at TEXT NOT NULL DEFAULT '',
  CHECK (status <> 'rejected' OR TRIM(reject_reason) <> '')
);
CREATE INDEX IF NOT EXISTS idx_wa_to_status ON work_assignments(to_user_id, status);
CREATE INDEX IF NOT EXISTS idx_wa_task ON work_assignments(task_id);
-- 같은 대상에 진행 중인 배정은 하나만
CREATE UNIQUE INDEX IF NOT EXISTS ux_wa_active_target ON work_assignments(task_id, target_type, target_id) WHERE status IN ('pending','held','accepted');
