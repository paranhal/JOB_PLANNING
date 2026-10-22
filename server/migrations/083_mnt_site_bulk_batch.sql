-- 95-15A 일괄 등록 배치 ID. 시드 INSERT 없음. 기존 사이트·방문 행은 수정하지 않는다.

ALTER TABLE maintenance_site_config ADD COLUMN bulk_batch_id TEXT NULL;
CREATE INDEX IF NOT EXISTS ix_mnt_site_bulk_batch ON maintenance_site_config(bulk_batch_id);
