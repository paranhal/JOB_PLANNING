-- 091 공용 담당자 선택기 (95-51 / 95-25)
-- 백필: user_id 가 있을 때만 ID 행, 이름만 있으면 이름 행. 이름→ID 추정 없음.

CREATE TABLE IF NOT EXISTS as_receipt_assignees (
    as_id TEXT NOT NULL,
    user_id TEXT NOT NULL DEFAULT '',
    assignee_name TEXT NOT NULL DEFAULT '',
    role TEXT NOT NULL DEFAULT 'owner' CHECK (role IN ('owner','support')),
    work_group_id TEXT,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now','localtime'))
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_as_receipt_assignees_user
    ON as_receipt_assignees(as_id, user_id) WHERE user_id <> '';
CREATE INDEX IF NOT EXISTS idx_as_receipt_assignees_user ON as_receipt_assignees(user_id);
CREATE INDEX IF NOT EXISTS idx_as_receipt_assignees_as ON as_receipt_assignees(as_id, sort_order);

CREATE TABLE IF NOT EXISTS as_work_item_assignees (
    work_id TEXT NOT NULL,
    user_id TEXT NOT NULL DEFAULT '',
    assignee_name TEXT NOT NULL DEFAULT '',
    role TEXT NOT NULL DEFAULT 'owner' CHECK (role IN ('owner','support')),
    work_group_id TEXT,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now','localtime'))
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_as_work_item_assignees_user
    ON as_work_item_assignees(work_id, user_id) WHERE user_id <> '';
CREATE INDEX IF NOT EXISTS idx_as_work_item_assignees_user ON as_work_item_assignees(user_id);
CREATE INDEX IF NOT EXISTS idx_as_work_item_assignees_work ON as_work_item_assignees(work_id, sort_order);

-- work_task_members.user_id 는 050에서 있음. 그룹만 추가.
-- ALTER 는 apply 함수에서 duplicate 무시.

INSERT INTO as_receipt_assignees (as_id, user_id, assignee_name, role, sort_order)
SELECT as_id,
       COALESCE(assigned_user_id,''),
       COALESCE(assigned_to,''),
       'owner',
       0
  FROM as_receipts
 WHERE (COALESCE(assigned_to,'') <> '' OR COALESCE(assigned_user_id,'') <> '')
   AND NOT EXISTS (
        SELECT 1 FROM as_receipt_assignees a WHERE a.as_id = as_receipts.as_id
   );

INSERT INTO as_work_item_assignees (work_id, user_id, assignee_name, role, sort_order)
SELECT work_id,
       COALESCE(assigned_user_id,''),
       COALESCE(assigned_to,''),
       'owner',
       0
  FROM as_work_items
 WHERE (COALESCE(assigned_to,'') <> '' OR COALESCE(assigned_user_id,'') <> '')
   AND NOT EXISTS (
        SELECT 1 FROM as_work_item_assignees a WHERE a.work_id = as_work_items.work_id
   );
