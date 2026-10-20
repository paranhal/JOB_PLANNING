-- 95-5 조직별 점검 제품 마스터. 시드 INSERT 없음. has_klas/has_rfid 는 유지.

CREATE TABLE IF NOT EXISTS inspection_products (
  product_id  TEXT PRIMARY KEY,
  org_id      TEXT NOT NULL,
  code        TEXT NOT NULL,
  name        TEXT NOT NULL,
  sort_order  INTEGER NOT NULL DEFAULT 0,
  is_active   INTEGER NOT NULL DEFAULT 1,
  created_at  TEXT NOT NULL DEFAULT '',
  updated_at  TEXT NOT NULL DEFAULT '',
  UNIQUE(org_id, code)
);
CREATE INDEX IF NOT EXISTS ix_insp_prod_org ON inspection_products(org_id, is_active, sort_order);

CREATE TABLE IF NOT EXISTS site_inspection_products (
  customer_id TEXT NOT NULL,
  product_id  TEXT NOT NULL,
  PRIMARY KEY(customer_id, product_id)
);
CREATE INDEX IF NOT EXISTS ix_site_insp_prod ON site_inspection_products(product_id);
