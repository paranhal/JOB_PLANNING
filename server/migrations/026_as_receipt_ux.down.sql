-- 026 롤백. 접수 행·고객 행은 남기고 추가 컬럼·코드만 되돌린다.

DELETE FROM codes WHERE code_id IN (
  'RC007','RC008',
  'URR_C1','URR_C2',
  'URR_R1','URR_R2','URR_R3',
  'URR_K1','URR_K2','URR_K3','URR_K4',
  'URR_W1','URR_W2'
);
UPDATE codes SET code_name='협력사요청' WHERE code_id='RC004' AND code_group='receipt_channel';
UPDATE codes SET code_name='제조사요청' WHERE code_id='RC006' AND code_group='receipt_channel';

ALTER TABLE as_receipts DROP COLUMN urgency_reason_note;
ALTER TABLE as_receipts DROP COLUMN urgency_reason;
ALTER TABLE customers DROP COLUMN party_kind;
