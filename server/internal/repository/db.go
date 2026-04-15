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
-- 코드 관리 (§11)
CREATE TABLE IF NOT EXISTS codes (
    code_id     TEXT PRIMARY KEY,
    code_group  TEXT NOT NULL,
    code_value  TEXT NOT NULL,
    code_name   TEXT NOT NULL,
    sort_order  INTEGER DEFAULT 0,
    is_active   INTEGER DEFAULT 1
);

-- 사용자 (§10)
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
    business_number    TEXT UNIQUE,
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
    created_by    TEXT,
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
    loc_building_name   TEXT,
    loc_floor_name      TEXT,
    loc_room_name       TEXT,
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

-- 접속정보참조 (§6.3)
CREATE TABLE IF NOT EXISTS access_info_references (
    access_info_id  TEXT PRIMARY KEY,
    asset_id        TEXT NOT NULL,
    file_path       TEXT,
    storage_method  TEXT,
    last_verified   TEXT,
    managed_by      TEXT,
    notes           TEXT,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
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
    is_reopen           INTEGER DEFAULT 0,
    result_code         TEXT,
    customer_confirmer  TEXT,
    confirm_datetime    DATETIME,
    followup_action     TEXT,
    replace_review      INTEGER DEFAULT 0,
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

-- 첨부파일 (§12)
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

-- ── 기본 코드 시드 데이터 (§11 전체 코드그룹) ──
INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order) VALUES
-- 업종
('IND001','industry','library','도서관',1),
('IND002','industry','school','학교',2),
('IND003','industry','public','공공기관',3),
('IND004','industry','private','민간',4),
-- 담당업무
('JR001','job_role','it','전산',1),
('JR002','job_role','network','네트워크',2),
('JR003','job_role','library_system','도서관리시스템',3),
('JR004','job_role','admin_system','행정시스템',4),
('JR005','job_role','general','일반행정',5),
-- 제품구분
('PT001','product_type','sw','SW',1),
('PT002','product_type','hw','HW',2),
('PT003','product_type','server','서버',3),
('PT004','product_type','network','네트워크장비',4),
('PT005','product_type','peripheral','주변장비',5),
-- 설치주체
('INST001','installer_type','self','자사',1),
('INST002','installer_type','other','타사',2),
('INST003','installer_type','manufacturer','제조사',3),
('INST004','installer_type','partner','협력사',4),
('INST005','installer_type','unknown','미상',5),
-- 관리유형
('MT001','management_type','direct','직접유지보수',1),
('MT002','management_type','fault','장애대응',2),
('MT003','management_type','periodic','정기점검',3),
('MT004','management_type','on_demand','요청시지원',4),
('MT005','management_type','reference','참고관리',5),
-- 요청주체유형
('RQ001','requester_type','customer','고객직접',1),
('RQ002','requester_type','manufacturer','제조사',2),
('RQ003','requester_type','partner','협력사',3),
('RQ004','requester_type','prime','원청',4),
('RQ005','requester_type','internal','내부',5),
-- AS상태
('AS_S001','as_status','received','접수',1),
('AS_S002','as_status','in_progress','진행중',2),
('AS_S003','as_status','hold','보류',3),
('AS_S004','as_status','completed','완료',4),
('AS_S005','as_status','closed','종료',5),
-- 긴급도
('URG001','urgency','high','상',1),
('URG002','urgency','normal','중',2),
('URG003','urgency','low','하',3),
-- 운영상태
('OPS001','operation_status','operating','운영중',1),
('OPS002','operation_status','maintenance','점검중',2),
('OPS003','operation_status','fault','장애',3),
('OPS004','operation_status','retired','철수',4),
('OPS005','operation_status','disposed','폐기',5),
-- 원인분류
('CT001','cause_type','hw','HW고장',1),
('CT002','cause_type','sw','SW오류',2),
('CT003','cause_type','network','네트워크',3),
('CT004','cause_type','env','환경문제',4),
('CT005','cause_type','user','사용자오류',5),
-- 처리유형
('PRT001','process_type','remote','원격지원',1),
('PRT002','process_type','visit','현장방문',2),
('PRT003','process_type','replace','부품교체',3),
('PRT004','process_type','config','설정변경',4),
('PRT005','process_type','inquiry','문의응대',5),
-- 접수채널
('RC001','receipt_channel','phone','전화',1),
('RC002','receipt_channel','email','이메일',2),
('RC003','receipt_channel','visit','방문',3),
('RC004','receipt_channel','partner','협력사요청',4),
-- 처리결과코드
('RSC001','result_code','done','완료',1),
('RSC002','result_code','temporary','임시조치',2),
('RSC003','result_code','transfer','타사이관',3),
('RSC004','result_code','escalation','제조사에스컬레이션',4),
-- 수행관계 구분
('REL001','relation_type','mfg_request','제조사요청수행',1),
('REL002','relation_type','partner_request','협력사요청수행',2),
('REL003','relation_type','direct_maint','직접유지보수',3),
('REL004','relation_type','fault_coop','장애대응협조',4),
-- 상대회사구분
('COMP001','company_type','manufacturer','제조사',1),
('COMP002','company_type','partner','협력사',2),
('COMP003','company_type','prime','원청사',3),
('COMP004','company_type','customer','고객기관',4);
`
	_, err := db.Exec(schema)
	if err != nil {
		return err
	}

	// 기존 DB에 새 컬럼 추가 (이미 있으면 무시)
	alters := []string{
		`ALTER TABLE as_receipts ADD COLUMN is_reopen INTEGER DEFAULT 0`,
		`ALTER TABLE as_receipts ADD COLUMN replace_review INTEGER DEFAULT 0`,
		`ALTER TABLE contact_history ADD COLUMN created_by TEXT`,
		`ALTER TABLE assets ADD COLUMN loc_building_name TEXT`,
		`ALTER TABLE assets ADD COLUMN loc_floor_name TEXT`,
		`ALTER TABLE assets ADD COLUMN loc_room_name TEXT`,
	}
	for _, q := range alters {
		db.Exec(q) // 이미 있으면 오류 무시
	}

	return nil
}
