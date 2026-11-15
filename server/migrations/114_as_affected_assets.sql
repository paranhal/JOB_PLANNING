-- 95-201: AS 조치 화면 「영향 장비」 + KLAS 원인 분류에 KDevice 추가
-- 영향 장비는 업무·건수를 만들지 않는다. 등록 자산이면 asset_id, 직접 입력이면 asset_id NULL + label.
CREATE TABLE IF NOT EXISTS as_receipt_affected_assets (
  id                 INTEGER PRIMARY KEY AUTOINCREMENT,
  as_id              TEXT NOT NULL,
  asset_id           TEXT,
  label              TEXT NOT NULL DEFAULT '',
  note               TEXT NOT NULL DEFAULT '',
  created_at         TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_by_user_id TEXT
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_as_affected_asset ON as_receipt_affected_assets(as_id, asset_id) WHERE asset_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS ix_as_affected_asset_asset ON as_receipt_affected_assets(asset_id);
CREATE INDEX IF NOT EXISTS ix_as_affected_asset_as ON as_receipt_affected_assets(as_id);

-- 원인 분류(기존 표 = 도서관사업팀). 멱등. 기존 분류·코드·정렬은 바꾸지 않는다.
-- 조직별 표(as_cause_category_orgs)에는 Go(applyASAffectedAssetsV114)가 KLAS 를 쓰는 조직에만 같이 넣는다.
INSERT OR IGNORE INTO as_cause_categories (code, level, parent_code, label, sort_order, is_active, cause_type_map, is_fault) VALUES
  ('klas.kdevice',             2, 'klas',         'KDevice(클라이언트 프로그램)', 35, 1, '', 1),
  ('klas.kdevice.install',     3, 'klas.kdevice', '설치·재설치',                  10, 1, '', 0),
  ('klas.kdevice.login_config',3, 'klas.kdevice', '설정·자동 로그인',             20, 1, '', 1),
  ('klas.kdevice.device_link', 3, 'klas.kdevice', '장비 연동 오류',               30, 1, '', 1),
  ('klas.kdevice.etc',         3, 'klas.kdevice', '기타',                         40, 1, '', 1);
