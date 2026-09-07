-- 기획서 §34.3.1 ~ §34.3.4
-- 처리유형 축소 · 원인분류 1차 · 조치 결과에서 재방문·대기 제외.
-- 실제 앱은 InitDB 시 repository.applyAS34ActionUX 가 동일 내용을 실행한다.

ALTER TABLE as_receipts ADD COLUMN cause_cat1 TEXT;
ALTER TABLE as_receipts ADD COLUMN cause_cat2 TEXT;
ALTER TABLE as_receipts ADD COLUMN cause_cat3 TEXT;

CREATE TABLE IF NOT EXISTS as_cause_categories (
    code        TEXT PRIMARY KEY,
    level       INTEGER NOT NULL,
    parent_code TEXT,
    label       TEXT NOT NULL,
    sort_order  INTEGER DEFAULT 0,
    is_active   INTEGER DEFAULT 1
);

INSERT OR IGNORE INTO as_cause_categories (code, level, parent_code, label, sort_order, is_active) VALUES
('rfid',   1, NULL, 'RFID',    1, 1),
('klas',   1, NULL, 'KLAS',    2, 1),
('web',    1, NULL, '홈페이지', 3, 1),
('server', 1, NULL, '서버',    4, 1);

UPDATE as_receipts SET process_type='visit'
 WHERE process_type IN ('replace','config');
UPDATE as_processes SET work_type='visit'
 WHERE work_type IN ('replace','config');

UPDATE codes SET is_active=0
 WHERE code_group='process_type' AND code_value IN ('replace','config');
UPDATE codes SET is_active=0
 WHERE code_group='result_code' AND code_value IN ('revisit_needed','temporary','escalation');
