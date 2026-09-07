-- 015 롤백. 관계자·변경 이력 표와 코드만 제거한다. 옛 사람을 살리는 구조이므로 행 삭제는 롤백 때만.

DROP TABLE IF EXISTS sales_changes;
DROP TABLE IF EXISTS sales_parties;

DELETE FROM codes WHERE code_group IN ('sales_party_type','sales_party_role');
DELETE FROM id_sequences WHERE seq_key IN (
  'sales_party',
  'sales_change',
  '__meta:sales_parties_v1',
  '__meta:sales_party_codes_v1'
);
