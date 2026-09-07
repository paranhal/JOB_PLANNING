-- 020 AS 접수·조치 전문검색 (§12.11)
-- modernc.org/sqlite + CGO_ENABLED=0 에서 FTS5+trigram 이 된다.
-- 기본 unicode61 은 한국어 조사에 안 맞다. tokenize='trigram' 필수.
-- FTS5 가 없으면 applyASSearch 가 테이블을 만들지 않고 LIKE 로 검색한다.
-- 검색에는 §4.5 기준일을 넣지 않는다.

CREATE VIRTUAL TABLE IF NOT EXISTS as_search USING fts5(
  as_id UNINDEXED, symptom, action, cause_detail, conclusion,
  tokenize='trigram'
);
