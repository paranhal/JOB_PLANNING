-- 019 지표 기준일·실행률 대상 (§4.5)
-- 값은 app_settings 가 출처다. 집계 코드에 날짜를 박지 않는다.
-- progress_scope 에서 행정·지원을 뺀 것은 임시.
-- 되돌림: 행정·지원 예정일 입력률이 4주 연속 90% 이상이면 progress_scope 에 admin 을 넣는다.
-- SQL 롤백: 019_metrics_settings.down.sql

INSERT OR IGNORE INTO app_settings(setting_key, setting_value, updated_at)
VALUES
  ('metrics_base_date', '2026-08-03', datetime('now','localtime')),
  ('progress_scope', 'as,maintenance', datetime('now','localtime'));
