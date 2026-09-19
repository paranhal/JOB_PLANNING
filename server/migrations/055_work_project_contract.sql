-- 055 계약은 work_projects. 금액·계약번호만 보강. §46.10
ALTER TABLE work_projects ADD COLUMN contract_amount INTEGER NOT NULL DEFAULT 0;
ALTER TABLE work_projects ADD COLUMN contract_no TEXT NOT NULL DEFAULT '';
