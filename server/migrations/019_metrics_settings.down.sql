-- 019 롤백. 기준일·실행률 대상을 지운다. 집계는 설정이 없으면 하한을 걸지 않고 실행률은 전 유형.

DELETE FROM app_settings
 WHERE setting_key IN ('metrics_base_date', 'progress_scope');
