-- 014 영업 활동 로그 (§32.8~§32.9, 2단계)
-- work_projects 는 건드리지 않는다. 실제 적용은 InitDB → applySalesActivities.
-- 활동 유형은 codes(sales_activity_type) 에 둔다.

CREATE TABLE IF NOT EXISTS sales_activities (
  activity_id       TEXT PRIMARY KEY,
  sales_id          TEXT NOT NULL,
  activity_date     TEXT NOT NULL,
  start_time        TEXT NOT NULL DEFAULT '',
  duration_min      INTEGER NOT NULL DEFAULT 30,
  activity_type     TEXT NOT NULL DEFAULT '',
  title             TEXT NOT NULL DEFAULT '',
  content           TEXT NOT NULL DEFAULT '',
  place             TEXT NOT NULL DEFAULT '',
  our_members       TEXT NOT NULL DEFAULT '',
  counterparts      TEXT NOT NULL DEFAULT '',
  next_action       TEXT NOT NULL DEFAULT '',
  next_action_date  TEXT NOT NULL DEFAULT '',
  stage_at_time     TEXT NOT NULL DEFAULT '',
  created_by        TEXT NOT NULL DEFAULT '',
  created_at        DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_sales_activities_sales ON sales_activities(sales_id, activity_date);
CREATE INDEX IF NOT EXISTS idx_sales_activities_date ON sales_activities(activity_date);

INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order, is_active) VALUES
  ('SAT01','sales_activity_type','research','정보수집',1,1),
  ('SAT02','sales_activity_type','call','전화',2,1),
  ('SAT03','sales_activity_type','visit','방문미팅',3,1),
  ('SAT04','sales_activity_type','online','온라인미팅',4,1),
  ('SAT05','sales_activity_type','mail','메일',5,1),
  ('SAT06','sales_activity_type','material','자료송부',6,1),
  ('SAT07','sales_activity_type','quote','견적제출',7,1),
  ('SAT08','sales_activity_type','proposal','제안서제출',8,1),
  ('SAT09','sales_activity_type','bid','입찰',9,1),
  ('SAT10','sales_activity_type','other','기타',10,1);
