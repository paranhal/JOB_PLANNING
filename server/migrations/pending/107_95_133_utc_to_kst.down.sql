-- 승인 후 적용된 경우에만. 마커를 지우고 +9를 되돌리면 이중 적용과 구분되지 않으므로
-- 운영에서는 down 대신 백업 복원을 쓴다. 테스트용 역변환:

-- UPDATE work_activities SET created_at = datetime(created_at, '-9 hours')
--  WHERE datetime(created_at) >= datetime(:cutoff);
-- DELETE FROM app_settings WHERE setting_key='95_133_utc_shift_done';
