-- 033 활동 「상대」한 칸 → sales_parties 자동 생성 (§32.7.1).
-- 실제 적용은 InitDB → applySalesPartyAutoV227.

ALTER TABLE sales_parties ADD COLUMN is_auto INTEGER NOT NULL DEFAULT 0;
