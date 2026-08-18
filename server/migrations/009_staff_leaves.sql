-- 기획서 §23.13.3 staff_leaves
-- 실제 앱은 InitDB 시 repository.applyStaffLeaves 가 동일 CREATE 를 실행한다.

CREATE TABLE IF NOT EXISTS staff_leaves (
  leave_id    TEXT PRIMARY KEY,
  user_id     TEXT NOT NULL,
  leave_date  TEXT NOT NULL,
  leave_kind  TEXT NOT NULL DEFAULT 'annual',
  note        TEXT,
  created_by  TEXT,
  created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (user_id, leave_date)
);

CREATE INDEX IF NOT EXISTS idx_staff_leaves_date ON staff_leaves(leave_date);
CREATE INDEX IF NOT EXISTS idx_staff_leaves_user ON staff_leaves(user_id);
