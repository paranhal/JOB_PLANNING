-- 조직 구분: 사업(business) / 지원(support). 기존 조직은 business.

ALTER TABLE orgs ADD COLUMN kind TEXT DEFAULT 'business';
UPDATE orgs SET kind='business' WHERE TRIM(COALESCE(kind,''))='';
