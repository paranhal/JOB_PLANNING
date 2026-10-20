DROP INDEX IF EXISTS ix_mnt_coop;
DROP INDEX IF EXISTS ix_as_coop;
DROP INDEX IF EXISTS ix_task_coop;
DROP TRIGGER IF EXISTS trg_coop_ev_no_delete;
DROP TRIGGER IF EXISTS trg_coop_ev_no_update;
DROP TABLE IF EXISTS cooperation_events;
DROP TABLE IF EXISTS cooperation_assignees;
DROP TABLE IF EXISTS cooperation_files;
DROP INDEX IF EXISTS ix_coop_user;
DROP INDEX IF EXISTS ix_coop_target;
DROP INDEX IF EXISTS ix_coop_req;
DROP TABLE IF EXISTS work_cooperations;
-- SQLite 환경마다 DROP COLUMN 이 다르다. 운영 롤백은 백업 복원을 쓴다.
