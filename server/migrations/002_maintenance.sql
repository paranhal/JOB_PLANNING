-- 정기 점검 (기획서 §17) — 신규 DB 또는 참고용.
-- 실제 앱은 repository/db.go initSchema 에 동일 DDL 포함.

CREATE TABLE IF NOT EXISTS maintenance_site_config (
    customer_id    TEXT PRIMARY KEY,
    short_name     TEXT NOT NULL,
    region         TEXT,
    has_klas       INTEGER NOT NULL DEFAULT 0,
    has_rfid       INTEGER NOT NULL DEFAULT 0,
    entry_category TEXT NOT NULL DEFAULT 'normal',
    fixed_rule     TEXT,
    updated_at     DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (customer_id) REFERENCES customers(customer_id)
);

CREATE TABLE IF NOT EXISTS maintenance_plans (
    plan_id    TEXT PRIMARY KEY,
    plan_year  INTEGER NOT NULL,
    title      TEXT,
    status     TEXT NOT NULL DEFAULT 'draft',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_maintenance_plans_year ON maintenance_plans(plan_year);

CREATE TABLE IF NOT EXISTS maintenance_visits (
    visit_id        TEXT PRIMARY KEY,
    plan_id         TEXT NOT NULL,
    visit_date      TEXT NOT NULL,
    customer_id     TEXT NOT NULL,
    sort_order      INTEGER NOT NULL DEFAULT 0,
    auto_generated  INTEGER NOT NULL DEFAULT 0,
    entry_category  TEXT NOT NULL DEFAULT 'normal',
    notes           TEXT,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (plan_id) REFERENCES maintenance_plans(plan_id),
    FOREIGN KEY (customer_id) REFERENCES customers(customer_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_maintenance_visit_dedup
ON maintenance_visits(plan_id, visit_date, customer_id);
