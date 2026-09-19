-- 051 as_keyword_links 키워드 조회 인덱스. §41.16.6
-- PK 가 as_id 로 시작하므로 keyword_id 로 찾는 인덱스가 없다.
-- 실제 앱은 InitDB 시 applyASKeywords 가 동일 인덱스를 만든다.

CREATE INDEX IF NOT EXISTS idx_as_keyword_links_kw ON as_keyword_links(keyword_id, as_id);

INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (51, datetime('now'));
