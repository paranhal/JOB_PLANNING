-- 95-14 회차·복수 담당·지역 기본. 시드 INSERT 없음. 기존 방문 행은 수정하지 않는다.

ALTER TABLE maintenance_visits ADD COLUMN round_no INTEGER NOT NULL DEFAULT 1;
ALTER TABLE maintenance_visits ADD COLUMN work_group_id TEXT NULL;

ALTER TABLE maintenance_site_config ADD COLUMN rounds_per_period INTEGER NOT NULL DEFAULT 1;

DROP INDEX IF EXISTS idx_maintenance_visit_dedup;
CREATE UNIQUE INDEX IF NOT EXISTS idx_maintenance_visit_dedup
ON maintenance_visits(plan_id, visit_date, customer_id, ifnull(product_type,''), ifnull(round_no,1));

CREATE TABLE IF NOT EXISTS maintenance_visit_assignees (
    visit_id      TEXT NOT NULL,
    user_id       TEXT NOT NULL DEFAULT '',
    assignee_name TEXT NOT NULL DEFAULT '',
    role          TEXT NOT NULL CHECK(role IN ('owner','support')),
    work_group_id TEXT NULL,
    sort_order    INTEGER NOT NULL DEFAULT 0,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (visit_id) REFERENCES maintenance_visits(visit_id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_mnt_va_user ON maintenance_visit_assignees(visit_id, user_id) WHERE TRIM(user_id)!='';
CREATE INDEX IF NOT EXISTS ix_mnt_visit_assignees_visit ON maintenance_visit_assignees(visit_id);

CREATE TABLE IF NOT EXISTS maintenance_region_assignees (
    org_id        TEXT NOT NULL,
    region        TEXT NOT NULL,
    user_id       TEXT NULL,
    work_group_id TEXT NULL,
    role          TEXT NOT NULL CHECK(role IN ('owner','support')),
    sort_order    INTEGER NOT NULL DEFAULT 0,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    CHECK ( (user_id IS NOT NULL AND TRIM(user_id)!='') OR (work_group_id IS NOT NULL AND TRIM(work_group_id)!='') )
);
CREATE INDEX IF NOT EXISTS ix_mnt_region_assignees_org ON maintenance_region_assignees(org_id, region);

CREATE TABLE IF NOT EXISTS maintenance_site_assignees (
    customer_id   TEXT NOT NULL,
    user_id       TEXT NULL,
    work_group_id TEXT NULL,
    role          TEXT NOT NULL CHECK(role IN ('owner','support')),
    sort_order    INTEGER NOT NULL DEFAULT 0,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    CHECK ( (user_id IS NOT NULL AND TRIM(user_id)!='') OR (work_group_id IS NOT NULL AND TRIM(work_group_id)!='') )
);
CREATE INDEX IF NOT EXISTS ix_mnt_site_assignees_cust ON maintenance_site_assignees(customer_id);
