-- 013 영업 사업 (§32.1~§32.5, 1단계)
-- work_projects 는 건드리지 않는다. 실제 적용은 InitDB → applySalesProjects.
--
-- 단계·확도는 codes 에 둔다 (§32.3.1).
--   sales_stage      : code_value=단계코드, code_name=라벨, sort_order=순서
--   sales_stage_prob : code_value=단계코드, code_name=확도숫자(예: 15)
-- 제안 진행 15% 는 기획 임시값. 코드관리에서 code_name 을 바꾸면 반영된다.

CREATE TABLE IF NOT EXISTS sales_projects (
  sales_id                   TEXT PRIMARY KEY,
  name                       TEXT NOT NULL,
  is_tentative_name          INTEGER NOT NULL DEFAULT 0,
  stage                      TEXT NOT NULL DEFAULT 'lead',
  probability                INTEGER NOT NULL DEFAULT 10,
  probability_override       INTEGER,
  customer_id                TEXT,
  prospect_name              TEXT NOT NULL DEFAULT '',
  prospect_region            TEXT NOT NULL DEFAULT '',
  customer_confirmed         INTEGER NOT NULL DEFAULT 0,
  expected_ym                TEXT NOT NULL DEFAULT '',
  expected_precision         TEXT NOT NULL DEFAULT 'month',
  expected_ym_confirmed      INTEGER NOT NULL DEFAULT 0,
  expected_amount            INTEGER NOT NULL DEFAULT 0,
  expected_amount_confirmed  INTEGER NOT NULL DEFAULT 0,
  sales_owner                TEXT NOT NULL DEFAULT '',
  sales_owner_id             TEXT,
  competitor                 TEXT NOT NULL DEFAULT '',
  lead_source                TEXT NOT NULL DEFAULT '',
  lost_reason                TEXT NOT NULL DEFAULT '',
  status                     TEXT NOT NULL DEFAULT 'active',
  notes                      TEXT NOT NULL DEFAULT '',
  created_at                 DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at                 DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_sales_projects_stage ON sales_projects(stage);
CREATE INDEX IF NOT EXISTS idx_sales_projects_status ON sales_projects(status);
CREATE INDEX IF NOT EXISTS idx_sales_projects_expected_ym ON sales_projects(expected_ym);
CREATE INDEX IF NOT EXISTS idx_sales_projects_owner ON sales_projects(sales_owner_id);

CREATE TABLE IF NOT EXISTS sales_stage_history (
  history_id     TEXT PRIMARY KEY,
  sales_id       TEXT NOT NULL,
  from_stage     TEXT NOT NULL DEFAULT '',
  to_stage       TEXT NOT NULL,
  reason         TEXT NOT NULL DEFAULT '',
  changed_by     TEXT NOT NULL DEFAULT '',
  changed_by_id  TEXT NOT NULL DEFAULT '',
  changed_at     DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_sales_stage_history_sales ON sales_stage_history(sales_id, changed_at);

INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order, is_active) VALUES
  ('SST01','sales_stage','lead','정보 입수',1,1),
  ('SST02','sales_stage','contact','담당자 접촉',2,1),
  ('SST03','sales_stage','proposal','제안 진행',3,1),
  ('SST04','sales_stage','quote','견적 요청',4,1),
  ('SST05','sales_stage','rfp','RFP 제안',5,1),
  ('SST06','sales_stage','submit','제안서 제출',6,1),
  ('SST07','sales_stage','won','수주확정',7,1),
  ('SST08','sales_stage','contracted','계약완료',8,1),
  ('SST09','sales_stage','lost','수주실패',9,1),
  ('SSP01','sales_stage_prob','lead','10',1,1),
  ('SSP02','sales_stage_prob','contact','10',2,1),
  ('SSP03','sales_stage_prob','proposal','15',3,1),
  ('SSP04','sales_stage_prob','quote','20',4,1),
  ('SSP05','sales_stage_prob','rfp','30',5,1),
  ('SSP06','sales_stage_prob','submit','50',6,1),
  ('SSP07','sales_stage_prob','won','90',7,1),
  ('SSP08','sales_stage_prob','contracted','100',8,1),
  ('SSP09','sales_stage_prob','lost','0',9,1),
  ('SLS01','sales_lead_source','existing','기존 고객',1,1),
  ('SLS02','sales_lead_source','bid','입찰공고',2,1),
  ('SLS03','sales_lead_source','referral','소개',3,1),
  ('SLS04','sales_lead_source','exhibition','전시회',4,1),
  ('SLS05','sales_lead_source','other','기타',5,1);
