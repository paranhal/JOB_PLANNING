-- 037 롤백. 노임단가표와 견적 부가 컬럼을 뗀다. 036 견적 테이블은 남긴다.

DROP TABLE IF EXISTS labor_rates;

DELETE FROM app_settings WHERE setting_key IN ('quote_overhead_rate','quote_tech_fee_rate');
