package repository

import (
	"database/sql"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// InitDB SQLite DB를 열고 스키마를 초기화한다
func InitDB(dbPath string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	// WAL 모드 활성화 (동시 읽기 성능 향상)
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, err
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		return nil, err
	}

	if err := initSchema(db); err != nil {
		return nil, err
	}

	return db, nil
}

func initSchema(db *sql.DB) error {
	schema := `
-- 코드 관리
CREATE TABLE IF NOT EXISTS codes (
    code_id     TEXT PRIMARY KEY,
    code_group  TEXT NOT NULL,
    code_value  TEXT NOT NULL,
    code_name   TEXT NOT NULL,
    sort_order  INTEGER DEFAULT 0,
    is_active   INTEGER DEFAULT 1
);

-- 사용자 (권한 §10)
CREATE TABLE IF NOT EXISTS users (
    user_id       TEXT PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    full_name     TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'viewer',
    is_active     INTEGER DEFAULT 1,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 고객 마스터 (§5.1)
CREATE TABLE IF NOT EXISTS customers (
    customer_id        TEXT PRIMARY KEY,
    org_name           TEXT NOT NULL,
    official_name      TEXT NOT NULL,
    org_email          TEXT,
    main_phone         TEXT,
    website            TEXT,
    business_number    TEXT,
    representative     TEXT,
    industry           TEXT,
    has_parent         INTEGER DEFAULT 0,
    parent_customer_id TEXT,
    address            TEXT,
    address_detail     TEXT,
    is_active          INTEGER DEFAULT 1,
    notes              TEXT,
    created_at         DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at         DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (parent_customer_id) REFERENCES customers(customer_id)
);

-- 고객 건물 (§5.2)
CREATE TABLE IF NOT EXISTS customer_buildings (
    building_id   TEXT PRIMARY KEY,
    customer_id   TEXT NOT NULL,
    building_name TEXT NOT NULL,
    building_type TEXT,
    address       TEXT,
    is_active     INTEGER DEFAULT 1,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (customer_id) REFERENCES customers(customer_id)
);

-- 고객 층
CREATE TABLE IF NOT EXISTS customer_floors (
    floor_id    TEXT PRIMARY KEY,
    building_id TEXT NOT NULL,
    floor_name  TEXT NOT NULL,
    sort_order  INTEGER DEFAULT 0,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (building_id) REFERENCES customer_buildings(building_id)
);

-- 고객 실
CREATE TABLE IF NOT EXISTS customer_rooms (
    room_id     TEXT PRIMARY KEY,
    floor_id    TEXT NOT NULL,
    room_name   TEXT NOT NULL,
    room_number TEXT,
    purpose     TEXT,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (floor_id) REFERENCES customer_floors(floor_id)
);

-- 고객 담당자 (§5.3)
CREATE TABLE IF NOT EXISTS contacts (
    contact_id  TEXT PRIMARY KEY,
    customer_id TEXT NOT NULL,
    full_name   TEXT NOT NULL,
    job_role    TEXT,
    title       TEXT,
    phone       TEXT,
    mobile      TEXT,
    email       TEXT,
    start_date  TEXT,
    end_date    TEXT,
    status      TEXT DEFAULT 'active',
    is_primary  INTEGER DEFAULT 0,
    notes       TEXT,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (customer_id) REFERENCES customers(customer_id)
);

-- 고객 담당자 이력 (§5.4)
CREATE TABLE IF NOT EXISTS contact_history (
    history_id    TEXT PRIMARY KEY,
    contact_id    TEXT NOT NULL,
    customer_id   TEXT NOT NULL,
    start_date    TEXT,
    end_date      TEXT,
    department    TEXT,
    job_role      TEXT,
    title         TEXT,
    phone         TEXT,
    email         TEXT,
    status        TEXT,
    change_reason TEXT,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (contact_id) REFERENCES contacts(contact_id)
);

-- 설치 기본정보 (§6.1)
CREATE TABLE IF NOT EXISTS assets (
    asset_id            TEXT PRIMARY KEY,
    customer_id         TEXT NOT NULL,
    product_name        TEXT NOT NULL,
    product_type        TEXT,
    model_name          TEXT,
    manufacturer        TEXT,
    serial_number       TEXT,
    install_date        TEXT,
    retire_date         TEXT,
    installer_type      TEXT,
    original_installer  TEXT,
    operation_status    TEXT DEFAULT 'operating',
    management_type     TEXT,
    is_managed          INTEGER DEFAULT 1,
    requester_type      TEXT,
    requester_name      TEXT,
    customer_contact_id TEXT,
    our_contact         TEXT,
    building_id         TEXT,
    floor_id            TEXT,
    room_id             TEXT,
    location_detail     TEXT,
    notes               TEXT,
    created_at          DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (customer_id) REFERENCES customers(customer_id)
);

-- 설치 SW 상세 (§6.2)
CREATE TABLE IF NOT EXISTS asset_sw_details (
    sw_detail_id  TEXT PRIMARY KEY,
    asset_id      TEXT NOT NULL,
    software_name TEXT,
    version       TEXT,
    install_type  TEXT,
    hw_info       TEXT,
    os            TEXT,
    os_version    TEXT,
    dbms          TEXT,
    db_version    TEXT,
    access_method TEXT,
    access_url    TEXT,
    install_path  TEXT,
    backup_path   TEXT,
    config_path   TEXT,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (asset_id) REFERENCES assets(asset_id)
);

-- 수행관계 (§7)
CREATE TABLE IF NOT EXISTS performance_relations (
    relation_id   TEXT PRIMARY KEY,
    customer_id   TEXT,
    asset_id      TEXT,
    relation_type TEXT NOT NULL,
    company_type  TEXT,
    company_name  TEXT,
    contact_name  TEXT,
    contact_phone TEXT,
    contact_email TEXT,
    start_date    TEXT,
    end_date      TEXT,
    is_active     INTEGER DEFAULT 1,
    notes         TEXT,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- AS 접수 (§8)
CREATE TABLE IF NOT EXISTS as_receipts (
    as_id               TEXT PRIMARY KEY,
    as_number           TEXT NOT NULL UNIQUE,
    receipt_datetime    DATETIME DEFAULT CURRENT_TIMESTAMP,
    customer_id         TEXT NOT NULL,
    asset_id            TEXT,
    receipt_channel     TEXT,
    requester           TEXT,
    symptom             TEXT,
    urgency             TEXT DEFAULT 'normal',
    priority            TEXT DEFAULT 'normal',
    requester_type      TEXT,
    requester_name      TEXT,
    assigned_to         TEXT,
    status              TEXT DEFAULT 'received',
    start_datetime      DATETIME,
    complete_datetime   DATETIME,
    process_type        TEXT,
    cause_type          TEXT,
    action_taken        TEXT,
    parts_used          TEXT,
    is_recurrence       INTEGER DEFAULT 0,
    result_code         TEXT,
    customer_confirmer  TEXT,
    confirm_datetime    DATETIME,
    followup_action     TEXT,
    created_at          DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (customer_id) REFERENCES customers(customer_id),
    FOREIGN KEY (asset_id) REFERENCES assets(asset_id)
);

-- AS 처리 이력
CREATE TABLE IF NOT EXISTS as_processes (
    process_id       TEXT PRIMARY KEY,
    as_id            TEXT NOT NULL,
    process_datetime DATETIME DEFAULT CURRENT_TIMESTAMP,
    worker           TEXT,
    work_type        TEXT,
    work_content     TEXT,
    parts_used       TEXT,
    time_spent       INTEGER,
    notes            TEXT,
    FOREIGN KEY (as_id) REFERENCES as_receipts(as_id)
);

-- 첨부파일
CREATE TABLE IF NOT EXISTS attachments (
    attachment_id TEXT PRIMARY KEY,
    ref_type      TEXT NOT NULL,
    ref_id        TEXT NOT NULL,
    file_name     TEXT NOT NULL,
    file_path     TEXT NOT NULL,
    file_size     INTEGER,
    mime_type     TEXT,
    uploaded_at   DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 기본 코드 데이터
INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order) VALUES
('IND001','industry','library','도서관',1),
('IND002','industry','school','학교',2),
('IND003','industry','public','공공기관',3),
('IND004','industry','private','민간',4),
('AS_S001','as_status','received','접수',1),
('AS_S002','as_status','in_progress','진행중',2),
('AS_S003','as_status','hold','보류',3),
('AS_S004','as_status','completed','완료',4),
('AS_S005','as_status','closed','종료',5),
('URG001','urgency','high','상',1),
('URG002','urgency','normal','중',2),
('URG003','urgency','low','하',3),
('OPS001','operation_status','operating','운영중',1),
('OPS002','operation_status','maintenance','점검중',2),
('OPS003','operation_status','fault','장애',3),
('OPS004','operation_status','retired','철수',4),
('OPS005','operation_status','disposed','폐기',5),
('PT001','product_type','sw','SW',1),
('PT002','product_type','hw','HW',2),
('PT003','product_type','server','서버',3),
('PT004','product_type','network','네트워크장비',4),
('PT005','product_type','peripheral','주변장비',5),
('INST001','installer_type','self','자사',1),
('INST002','installer_type','other','타사',2),
('INST003','installer_type','manufacturer','제조사',3),
('INST004','installer_type','partner','협력사',4),
('INST005','installer_type','unknown','미상',5);
`
	_, err := db.Exec(schema)
	return err
}
