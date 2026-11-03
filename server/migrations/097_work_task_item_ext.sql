-- 95-82 작업 항목 구분·장비·담당·자산·AS 연결. 운영 행 UPDATE 없음.

ALTER TABLE work_task_items ADD COLUMN kind TEXT NOT NULL DEFAULT 'etc';
ALTER TABLE work_task_items ADD COLUMN product_name TEXT NOT NULL DEFAULT '';
ALTER TABLE work_task_items ADD COLUMN model_name TEXT NOT NULL DEFAULT '';
ALTER TABLE work_task_items ADD COLUMN qty INTEGER NOT NULL DEFAULT 1;
ALTER TABLE work_task_items ADD COLUMN as_id TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_wti_as ON work_task_items(as_id);

CREATE TABLE IF NOT EXISTS work_task_item_assignees (
  item_id TEXT NOT NULL,
  assignee_name TEXT NOT NULL DEFAULT '',
  assignee_user_id TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (item_id, assignee_name)
);

CREATE TABLE IF NOT EXISTS work_task_item_assets (
  item_id TEXT NOT NULL,
  asset_id TEXT NOT NULL,
  role TEXT NOT NULL DEFAULT 'ref',
  created_at TEXT NOT NULL DEFAULT (datetime('now','localtime')),
  PRIMARY KEY (item_id, asset_id, role)
);
CREATE INDEX IF NOT EXISTS idx_wtia_asset ON work_task_item_assets(asset_id);

INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order, is_active)
VALUES
  ('WIK01','wb_item_kind','install','장비 장착',1,1),
  ('WIK02','wb_item_kind','work','공사',2,1),
  ('WIK03','wb_item_kind','as','AS',3,1),
  ('WIK04','wb_item_kind','etc','기타',4,1);
