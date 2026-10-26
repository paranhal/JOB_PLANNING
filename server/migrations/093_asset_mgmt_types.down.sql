-- 093 down: 코드만 되돌린다. 자산 값(관리유형·점검주기)은 이 SQL 로 복원하지 않는다.
-- 적용 전 app.db 백업을 복사해 되돌린다. cmd/migrate093 -revert 는 변경 CSV 가 있을 때만 값을 복구한다.

UPDATE codes SET is_active=1, sort_order=1 WHERE code_group='management_type' AND code_value='direct';
UPDATE codes SET is_active=1, sort_order=2 WHERE code_group='management_type' AND code_value='fault';
UPDATE codes SET is_active=1, sort_order=3 WHERE code_group='management_type' AND code_value='periodic';
UPDATE codes SET is_active=1, sort_order=4 WHERE code_group='management_type' AND code_value='on_demand';
UPDATE codes SET is_active=1, sort_order=5 WHERE code_group='management_type' AND code_value='reference';
UPDATE codes SET is_active=1, sort_order=6 WHERE code_group='management_type' AND code_value='third_party';

UPDATE codes SET is_active=0 WHERE code_group='maint_cycle' AND code_value='call';
UPDATE codes SET sort_order=6 WHERE code_group='maint_cycle' AND code_value='custom';
