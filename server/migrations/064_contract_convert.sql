-- 84-D D-88. 계약으로 전환 시 계약일·구분·부가세. 기존 행은 비움(코드로 채우지 않음).

ALTER TABLE work_projects ADD COLUMN contract_date TEXT;
ALTER TABLE work_projects ADD COLUMN contract_category TEXT NOT NULL DEFAULT '';
ALTER TABLE work_projects ADD COLUMN amount_vat_included INTEGER;

CREATE INDEX IF NOT EXISTS idx_wp_contract_category ON work_projects(contract_category);
CREATE INDEX IF NOT EXISTS idx_wp_contract_date ON work_projects(contract_date);

INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order, is_active) VALUES
  ('CCT01','contract_category','bid','입찰에 의한 계약',10,1),
  ('CCT02','contract_category','pps','조달 물품 구매',20,1),
  ('CCT03','contract_category','private','수의계약',30,1),
  ('CCT04','contract_category','etc','기타',40,1);
