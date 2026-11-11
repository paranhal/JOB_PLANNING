-- 95-175 C / 95-122. 하부 업무·계획 마일스톤. ID는 TEXT.
-- 기존 하부 업무에는 넣지 않는다. 화면의 「기본 마일스톤 추가」 또는 이후 생성 시 25/50/75/100.
CREATE TABLE IF NOT EXISTS package_milestones (
  milestone_id TEXT PRIMARY KEY,
  package_id TEXT NOT NULL DEFAULT '',
  plan_id TEXT NOT NULL DEFAULT '',
  threshold_pct INTEGER NOT NULL,
  label TEXT NOT NULL DEFAULT '',
  reached_at TEXT NOT NULL DEFAULT '',
  dropped_at TEXT NOT NULL DEFAULT '',
  confirmed_at TEXT NOT NULL DEFAULT '',
  confirmed_by TEXT NOT NULL DEFAULT '',
  note TEXT NOT NULL DEFAULT '',
  billing_id TEXT NOT NULL DEFAULT '',
  UNIQUE(package_id, plan_id, threshold_pct)
);
CREATE INDEX IF NOT EXISTS idx_pkg_milestones_pkg ON package_milestones(package_id, plan_id);
