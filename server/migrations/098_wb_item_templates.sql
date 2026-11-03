-- 95-83 작업 항목 마스터·세트. 운영 행 UPDATE 없음. 예시 시드 없음.

CREATE TABLE IF NOT EXISTS wb_item_templates (
  template_id TEXT PRIMARY KEY,
  org_id TEXT NOT NULL DEFAULT '',
  kind TEXT NOT NULL,
  name TEXT NOT NULL,
  default_qty INTEGER NOT NULL DEFAULT 1,
  unit TEXT NOT NULL DEFAULT '건',
  is_active INTEGER NOT NULL DEFAULT 1,
  sort_order INTEGER NOT NULL DEFAULT 0,
  note TEXT NOT NULL DEFAULT '',
  created_by TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (datetime('now','localtime')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now','localtime'))
);
CREATE INDEX IF NOT EXISTS idx_wb_item_tpl_org ON wb_item_templates(org_id, kind, is_active);

CREATE TABLE IF NOT EXISTS wb_item_template_links (
  template_id TEXT NOT NULL,
  child_template_id TEXT NOT NULL,
  default_checked INTEGER NOT NULL DEFAULT 1,
  sort_order INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (template_id, child_template_id)
);
CREATE INDEX IF NOT EXISTS idx_wb_item_tpl_link_child ON wb_item_template_links(child_template_id);

ALTER TABLE work_task_items ADD COLUMN template_id TEXT NOT NULL DEFAULT '';
ALTER TABLE work_task_items ADD COLUMN set_main_item_id TEXT NOT NULL DEFAULT '';
ALTER TABLE work_task_items ADD COLUMN unit TEXT NOT NULL DEFAULT '';

INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order, is_active)
VALUES
  ('WIU01','wb_item_unit','건','건',1,1),
  ('WIU02','wb_item_unit','대','대',2,1),
  ('WIU03','wb_item_unit','m','m',3,1),
  ('WIU04','wb_item_unit','식','식',4,1);
