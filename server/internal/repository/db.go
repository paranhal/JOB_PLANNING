package repository

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"strings"

	"customer-support/internal/model"

	_ "modernc.org/sqlite"
)

// InitDB SQLite DB를 열고 스키마를 초기화한다
func InitDB(dbPath string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, err
	}

	// PRAGMA 는 커넥션별 설정이다. DSN 으로 넘겨야 풀의 모든 커넥션에 걸린다. §44.5
	sep := "?"
	if strings.Contains(dbPath, "?") {
		sep = "&"
	}
	dsn := dbPath + sep +
		"_pragma=busy_timeout(5000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=foreign_keys(1)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_pragma=cache_size(-32000)" + // 32MB. 운영 DB 17MB 를 통째로 메모리에
		"&_pragma=wal_autocheckpoint(512)"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}

	// 체크포인트가 WAL 을 되감으려면 활성 독자가 없는 순간이 있어야 한다.
	// 커넥션이 무제한이면 그 순간이 오지 않는다. §44.5 ③
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(8)
	db.SetConnMaxLifetime(0)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := initSchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	logForeignKeyCheck(db)
	return db, nil
}

func logForeignKeyCheck(db *sql.DB) {
	if db == nil {
		return
	}
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		log.Printf("foreign_key_check: %v", err)
		return
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		n++
	}
	log.Printf("foreign_key_check: %d건", n)
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

-- 사용자 (§10) — role: admin/tech/sales/office/observer
CREATE TABLE IF NOT EXISTS users (
    user_id       TEXT PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    full_name     TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'observer',
    permissions   TEXT DEFAULT '',
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
    postal_code        TEXT,
    addr_sido          TEXT,
    addr_sigungu       TEXT,
    addr_dong          TEXT,
    is_active          INTEGER DEFAULT 1,
    notes              TEXT,
    party_kind         TEXT DEFAULT 'customer',
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
    affiliation TEXT DEFAULT 'institution',
    job_role    TEXT,
    title       TEXT,
    job_grade   TEXT,
    phone       TEXT,
    mobile      TEXT,
    email       TEXT,
    start_date  TEXT,
    end_date    TEXT,
    status      TEXT DEFAULT 'active',
    contact_role TEXT DEFAULT 'regular',
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
    mobile        TEXT,
    email         TEXT,
    status        TEXT,
    affiliation   TEXT,
    contact_role  TEXT,
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
    product_category    TEXT,
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
    maint_contract_type TEXT,
    maint_cycle         TEXT,
    maint_start_date    TEXT,
    maint_end_date      TEXT,
    maint_billing_party TEXT,
    maint_billing_cycle TEXT,
    requester_type      TEXT,
    requester_name      TEXT,
    customer_contact_id TEXT,
    our_contact         TEXT,
    building_id         TEXT,
    floor_id            TEXT,
    room_id             TEXT,
    install_location    TEXT,
    location_detail     TEXT,
    notes               TEXT,
    project_id          TEXT,
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
    assigned_user_id    TEXT,
    received_by         TEXT,
    visit_scheduled_date TEXT,
    schedule_confirmed  INTEGER DEFAULT 0,
    status              TEXT DEFAULT 'received',
    start_datetime      DATETIME,
    complete_datetime   DATETIME,
    visit_date          TEXT,
    process_type        TEXT,
    process_type_reason TEXT,
    work_place          TEXT,
    transfer_detail     TEXT,
    confirm_target      TEXT,
    confirm_contact     TEXT,
    cause_type          TEXT,
    cause_detail        TEXT,
    conclusion          TEXT,
    action_taken        TEXT,
    parts_used          TEXT,
    is_recurrence       INTEGER DEFAULT 0,
    is_reopen           INTEGER DEFAULT 0,
    result_code         TEXT,
    revisit_reason      TEXT,
    hold_reason         TEXT,
    hold_next_action    TEXT,
    cancel_datetime     DATETIME,
    customer_confirmer  TEXT,
    confirm_datetime    DATETIME,
    followup_action     TEXT,
    replace_review      INTEGER DEFAULT 0,
    project_id          TEXT,
    receipt_group_id    TEXT,
    urgency_reason      TEXT,
    urgency_reason_note TEXT,
    cause_cat1          TEXT,
    cause_cat2          TEXT,
    cause_cat3          TEXT,
    followup_note       TEXT,
    created_at          DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (customer_id) REFERENCES customers(customer_id),
    FOREIGN KEY (asset_id) REFERENCES assets(asset_id)
);

-- 원인분류 계층. 지금은 1차만. 2·3차는 엑셀 수급 후. §34.3.3
CREATE TABLE IF NOT EXISTS as_cause_categories (
    code            TEXT PRIMARY KEY,
    level           INTEGER NOT NULL,
    parent_code     TEXT,
    label           TEXT NOT NULL,
    sort_order      INTEGER DEFAULT 0,
    is_active       INTEGER DEFAULT 1,
    cause_type_map  TEXT NOT NULL DEFAULT '',
    is_fault        INTEGER NOT NULL DEFAULT 1
);

-- AS 처리 이력
CREATE TABLE IF NOT EXISTS as_processes (
    process_id       TEXT PRIMARY KEY,
    as_id            TEXT NOT NULL,
    process_number   TEXT,
    process_datetime DATETIME DEFAULT CURRENT_TIMESTAMP,
    worker           TEXT,
    work_type        TEXT,
    cause_type       TEXT,
    work_content     TEXT,
    parts_used       TEXT,
    time_spent       INTEGER,
    notes            TEXT,
    result_code      TEXT,
    transfer_detail  TEXT,
    next_action_date TEXT,
    wait_reason      TEXT,
    prep_notes       TEXT,
    FOREIGN KEY (as_id) REFERENCES as_receipts(as_id)
);

-- AS 하부업무: 확인·재방문 ({접수번호}-Wnn)
CREATE TABLE IF NOT EXISTS as_work_items (
    work_id             TEXT PRIMARY KEY,
    work_number         TEXT NOT NULL UNIQUE,
    as_id               TEXT NOT NULL,
    work_kind           TEXT NOT NULL,
    scheduled_date      TEXT,
    schedule_confirmed  INTEGER DEFAULT 0,
    confirm_target      TEXT,
    confirm_contact     TEXT,
    assigned_to         TEXT,
    assigned_user_id    TEXT,
    status              TEXT DEFAULT 'open',
    notes               TEXT,
    created_at          DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (as_id) REFERENCES as_receipts(as_id)
);

-- 업무 고유번호 시퀀스 (§10.2)
CREATE TABLE IF NOT EXISTS id_sequences (
    seq_key TEXT PRIMARY KEY,
    last_no INTEGER NOT NULL DEFAULT 0
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
    keywords      TEXT,
    slot_no       INTEGER DEFAULT 0,
    uploaded_at   DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 정기 점검 (기획서 §17)
CREATE TABLE IF NOT EXISTS maintenance_site_config (
    customer_id    TEXT PRIMARY KEY,
    short_name     TEXT NOT NULL,
    region         TEXT,
    has_klas       INTEGER NOT NULL DEFAULT 0,
    has_rfid       INTEGER NOT NULL DEFAULT 0,
    inspection_cycle TEXT NOT NULL DEFAULT 'monthly',
    entry_category TEXT NOT NULL DEFAULT 'normal',
    fixed_rule     TEXT,
    updated_at     DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (customer_id) REFERENCES customers(customer_id)
);

CREATE TABLE IF NOT EXISTS maintenance_plans (
    plan_id    TEXT PRIMARY KEY,
    plan_year  INTEGER NOT NULL,
    title      TEXT,
    status     TEXT NOT NULL DEFAULT 'draft',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_maintenance_plans_year ON maintenance_plans(plan_year);

CREATE TABLE IF NOT EXISTS maintenance_visits (
    visit_id        TEXT PRIMARY KEY,
    plan_id         TEXT NOT NULL,
    visit_date      TEXT NOT NULL,
    customer_id     TEXT NOT NULL,
    sort_order      INTEGER NOT NULL DEFAULT 0,
    auto_generated  INTEGER NOT NULL DEFAULT 0,
    entry_category  TEXT NOT NULL DEFAULT 'normal',
    notes           TEXT,
    assignee        TEXT,
    product_type    TEXT,
    completed       INTEGER NOT NULL DEFAULT 0,
    completed_date  TEXT,
    project_id      TEXT,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (plan_id) REFERENCES maintenance_plans(plan_id),
    FOREIGN KEY (customer_id) REFERENCES customers(customer_id)
);

-- 중복 방지 인덱스는 product_type 컬럼이 추가된 뒤(아래 마이그레이션) 만든다.

-- 업무처리현황 · 기타 업무
CREATE TABLE IF NOT EXISTS work_other (
    other_id   TEXT PRIMARY KEY,
    work_date  TEXT NOT NULL,
    title      TEXT NOT NULL,
    phase      TEXT NOT NULL DEFAULT 'receipt',
    org_name   TEXT,
    notes      TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_work_other_date ON work_other(work_date, phase);

-- 워크보드 (프로젝트·업무 · 칸반/목록)
CREATE TABLE IF NOT EXISTS work_projects (
    project_id        TEXT PRIMARY KEY,
    name              TEXT NOT NULL,
    short_name        TEXT,
    plan_year         INTEGER DEFAULT 0,
    is_paid           INTEGER NOT NULL DEFAULT 1,
    sort_order        INTEGER NOT NULL DEFAULT 0,
    ordering_party_id TEXT,
    ordering_party    TEXT,
    customer_id       TEXT,
    contract_type   TEXT,
    billing_type    TEXT,
    start_date      TEXT,
    end_date        TEXT,
    notes           TEXT,
    contact_id      TEXT,
    color           TEXT NOT NULL DEFAULT '#3B82F6',
    status          TEXT NOT NULL DEFAULT 'active',
    sales_project_id TEXT,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS project_scope_rules (
    rule_id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    parent_customer_id TEXT,
    product_keys TEXT,
    work_kinds TEXT,
    notes TEXT,
    FOREIGN KEY (project_id) REFERENCES work_projects(project_id)
);
CREATE TABLE IF NOT EXISTS weekly_report_rows (
    row_key      TEXT PRIMARY KEY,
    sheet_row    INTEGER NOT NULL,
    division     TEXT NOT NULL DEFAULT '',
    team         TEXT NOT NULL DEFAULT '',
    no_label     TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT '',
    project_id   TEXT,
    row_kind     TEXT NOT NULL DEFAULT 'project',
    highlight    INTEGER NOT NULL DEFAULT 0,
    is_active    INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_weekly_report_rows_project ON weekly_report_rows(project_id);
CREATE INDEX IF NOT EXISTS idx_weekly_report_rows_sheet ON weekly_report_rows(sheet_row);
CREATE TABLE IF NOT EXISTS work_tasks (
    task_id         TEXT PRIMARY KEY,
    work_type       TEXT NOT NULL DEFAULT 'admin',
    project_id      TEXT,
    title           TEXT NOT NULL,
    description     TEXT,
    due_date        TEXT,
    work_date       TEXT,
    start_time      TEXT,
    end_time        TEXT,
    duration_min    INTEGER NOT NULL DEFAULT 30,
    status          TEXT NOT NULL DEFAULT 'waiting',
    priority        TEXT NOT NULL DEFAULT 'normal',
    assignee        TEXT,
    assignee_source TEXT NOT NULL DEFAULT '',
    tags            TEXT,
    progress        INTEGER NOT NULL DEFAULT 0,
    source_type     TEXT,
    source_id       TEXT,
    source_role     TEXT NOT NULL DEFAULT '',
    parent_task_id  TEXT,
    customer_id     TEXT,
    customer_name   TEXT,
    receipt_date    TEXT,
    complete_date   TEXT,
    recurrence_role    TEXT,
    occurrence_seq     INTEGER,
    occurrence_status  TEXT,
    not_done_reason    TEXT,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (project_id) REFERENCES work_projects(project_id)
);
CREATE INDEX IF NOT EXISTS idx_work_tasks_status ON work_tasks(status, due_date);
-- work_date·source_type 인덱스는 기존 DB에 컬럼을 먼저 추가해야 하므로 마이그레이션에서 만든다.

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
-- 담당자 직급 (§5.3)
('JG001','job_grade','librarian','사서직',1),
('JG002','job_grade','it','전산직',2),
('JG003','job_grade','other','기타',3),
('JG004','job_grade','custom','직접입력',4),
-- 제품구분
('PT001','product_type','SW','SW',1),
('PT002','product_type','HW','HW',2),
('PT003','product_type','SERVER','서버',3),
('PT004','product_type','NETWORK','네트워크장비',4),
('PT005','product_type','PERIPHERAL','주변장비',5),
('PT006','product_type','KIOSK','키오스크',6),
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
('MT006','management_type','third_party','타사장비',6),
-- 유지보수 계약구분 (§6.1)
('MCT001','maint_contract_type','paid','유상',1),
('MCT002','maint_contract_type','free','무상',2),
('MCT003','maint_contract_type','call','CALL',3),
('MCT004','maint_contract_type','none','미계약',4),
-- 유지보수 점검주기 (청구주기와 동일 체계)
('MCY001','maint_cycle','monthly','월',1),
('MCY002','maint_cycle','quarterly','분기',2),
('MCY003','maint_cycle','semi','반기',3),
('MCY004','maint_cycle','odd_bimonthly','홀수격월',4),
('MCY005','maint_cycle','even_bimonthly','짝수격월',5),
('MCY006','maint_cycle','custom','직접입력',6),
-- 유지보수 청구주기
('MBC001','maint_billing_cycle','monthly','월',1),
('MBC002','maint_billing_cycle','quarterly','분기',2),
('MBC003','maint_billing_cycle','semi','반기',3),
('MBC004','maint_billing_cycle','odd_bimonthly','홀수격월',4),
('MBC005','maint_billing_cycle','even_bimonthly','짝수격월',5),
('MBC006','maint_billing_cycle','custom','직접입력',6),
-- 요청주체유형
('RQ001','requester_type','customer','고객직접',1),
('RQ002','requester_type','manufacturer','제조사',2),
('RQ003','requester_type','partner','협력사',3),
('RQ004','requester_type','prime','원청',4),
('RQ005','requester_type','internal','내부',5),
('RQ006','requester_type','onecall','원콜',6),
-- AS상태
('AS_S001','as_status','received','접수',1),
('AS_S002','as_status','in_progress','진행중',2),
('AS_S003','as_status','hold','보류',3),
('AS_S006','as_status','partial_complete','부분완료',4),
('AS_S004','as_status','completed','완료',5),
('AS_S005','as_status','closed','종료',6),
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
('PRT006','process_type','undetermined','미정',6),
-- 접수채널
('RC001','receipt_channel','phone','전화',1),
('RC002','receipt_channel','email','이메일',2),
('RC003','receipt_channel','visit','방문',3),
('RC004','receipt_channel','partner','협력사요청',4),
('RC005','receipt_channel','onecall','원콜',5),
('RC006','receipt_channel','maker','제조사요청',6),
-- 처리결과코드
('RSC001','result_code','done','완료',1),
('RSC006','result_code','partial','부분완료',2),
('RSC002','result_code','temporary','임시조치',3),
('RSC003','result_code','transfer','타사이관',4),
('RSC005','result_code','revisit_needed','재방문필요',5),
('RSC004','result_code','escalation','제조사에스컬레이션',6),
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
		`ALTER TABLE as_receipts ADD COLUMN received_by TEXT`,
		`ALTER TABLE as_receipts ADD COLUMN assigned_user_id TEXT`,
		`ALTER TABLE as_receipts ADD COLUMN visit_scheduled_date TEXT`,
		`ALTER TABLE as_receipts ADD COLUMN schedule_confirmed INTEGER DEFAULT 0`,
		`ALTER TABLE maintenance_site_config ADD COLUMN inspection_cycle TEXT DEFAULT 'monthly'`,
		`ALTER TABLE as_receipts ADD COLUMN revisit_reason TEXT`,
		`ALTER TABLE as_receipts ADD COLUMN hold_reason TEXT`,
		`ALTER TABLE as_receipts ADD COLUMN hold_next_action TEXT`,
		`ALTER TABLE as_receipts ADD COLUMN cancel_datetime TEXT`,
		`ALTER TABLE assets ADD COLUMN product_category TEXT`,
		`ALTER TABLE as_processes ADD COLUMN process_number TEXT`,
		`ALTER TABLE as_processes ADD COLUMN result_code TEXT`,
		`ALTER TABLE as_processes ADD COLUMN transfer_detail TEXT`,
		`ALTER TABLE as_processes ADD COLUMN next_action_date TEXT`,
		`ALTER TABLE as_processes ADD COLUMN wait_reason TEXT`,
		`ALTER TABLE as_processes ADD COLUMN prep_notes TEXT`,
		`ALTER TABLE contact_history ADD COLUMN created_by TEXT`,
		`ALTER TABLE assets ADD COLUMN loc_building_name TEXT`,
		`ALTER TABLE assets ADD COLUMN loc_floor_name TEXT`,
		`ALTER TABLE assets ADD COLUMN loc_room_name TEXT`,
		`ALTER TABLE assets ADD COLUMN install_location TEXT`,
		`ALTER TABLE assets ADD COLUMN maint_contract_type TEXT`,
		`ALTER TABLE assets ADD COLUMN maint_cycle TEXT`,
		`ALTER TABLE assets ADD COLUMN maint_start_date TEXT`,
		`ALTER TABLE assets ADD COLUMN maint_end_date TEXT`,
		`ALTER TABLE assets ADD COLUMN maint_billing_party TEXT`,
		`ALTER TABLE assets ADD COLUMN maint_billing_cycle TEXT`,
		`ALTER TABLE contacts ADD COLUMN job_grade TEXT`,
		`ALTER TABLE contacts ADD COLUMN contact_role TEXT`,
		`ALTER TABLE contacts ADD COLUMN affiliation TEXT`,
		`ALTER TABLE contact_history ADD COLUMN contact_role TEXT`,
		`ALTER TABLE contact_history ADD COLUMN affiliation TEXT`,
		`ALTER TABLE contact_history ADD COLUMN mobile TEXT`,
		`ALTER TABLE customers ADD COLUMN postal_code TEXT`,
		`ALTER TABLE customers ADD COLUMN addr_sido TEXT`,
		`ALTER TABLE customers ADD COLUMN addr_sigungu TEXT`,
		`ALTER TABLE customers ADD COLUMN addr_dong TEXT`,
		`ALTER TABLE customers ADD COLUMN needs_review INTEGER DEFAULT 0`,
		`ALTER TABLE customers ADD COLUMN review_reason TEXT`,
		// 완료된 건과 같은 증상으로 다시 접수한 건을 원 접수와 잇는다.
		`ALTER TABLE as_receipts ADD COLUMN parent_as_id TEXT`,
		`ALTER TABLE as_receipts ADD COLUMN reopen_reason TEXT`,
		`CREATE INDEX IF NOT EXISTS idx_as_receipts_parent ON as_receipts(parent_as_id)`,
		`ALTER TABLE maintenance_visits ADD COLUMN assignee TEXT`,
		`ALTER TABLE maintenance_visits ADD COLUMN product_type TEXT`,
		`ALTER TABLE maintenance_visits ADD COLUMN completed INTEGER DEFAULT 0`,
		`ALTER TABLE maintenance_visits ADD COLUMN completed_date TEXT`,
		`ALTER TABLE maintenance_visits ADD COLUMN project_id TEXT`,
		`CREATE INDEX IF NOT EXISTS idx_maintenance_visits_project ON maintenance_visits(project_id)`,
		// 같은 날 같은 기관이라도 KLAS·앤로보틱스처럼 점검 대상이 다르면 별도 방문으로 둔다.
		`DROP INDEX IF EXISTS idx_maintenance_visit_dedup`,
		`ALTER TABLE attachments ADD COLUMN keywords TEXT`,
		`ALTER TABLE attachments ADD COLUMN slot_no INTEGER DEFAULT 0`,
		`ALTER TABLE as_receipts ADD COLUMN transfer_detail TEXT`,
		`ALTER TABLE as_receipts ADD COLUMN confirm_target TEXT`,
		`ALTER TABLE as_receipts ADD COLUMN confirm_contact TEXT`,
		`ALTER TABLE as_receipts ADD COLUMN work_place TEXT`,
		`ALTER TABLE as_receipts ADD COLUMN import_key TEXT`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_as_receipts_import_key ON as_receipts(import_key)
			WHERE import_key IS NOT NULL AND import_key != ''`,
		`ALTER TABLE work_projects ADD COLUMN ordering_party TEXT`,
		`ALTER TABLE work_projects ADD COLUMN customer_id TEXT`,
		`ALTER TABLE work_projects ADD COLUMN contract_type TEXT`,
		`ALTER TABLE work_projects ADD COLUMN billing_type TEXT`,
		`ALTER TABLE work_projects ADD COLUMN notes TEXT`,
		`ALTER TABLE work_projects ADD COLUMN contact_id TEXT`,
		`ALTER TABLE work_projects ADD COLUMN ordering_party_id TEXT`,
		`ALTER TABLE work_projects ADD COLUMN plan_year INTEGER DEFAULT 0`,
		`ALTER TABLE work_projects ADD COLUMN short_name TEXT`,
		`ALTER TABLE work_projects ADD COLUMN is_paid INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE work_projects ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE assets ADD COLUMN project_id TEXT`,
		`CREATE INDEX IF NOT EXISTS idx_assets_project ON assets(project_id)`,
		`CREATE TABLE IF NOT EXISTS project_scope_rules (
			rule_id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			parent_customer_id TEXT,
			product_keys TEXT,
			work_kinds TEXT,
			notes TEXT,
			FOREIGN KEY (project_id) REFERENCES work_projects(project_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_project_scope_rules_project ON project_scope_rules(project_id)`,
		`ALTER TABLE work_tasks ADD COLUMN work_type TEXT NOT NULL DEFAULT 'admin'`,
		`ALTER TABLE work_tasks ADD COLUMN work_date TEXT`,
		`ALTER TABLE work_tasks ADD COLUMN start_time TEXT`,
		`ALTER TABLE work_tasks ADD COLUMN end_time TEXT`,
		`ALTER TABLE work_tasks ADD COLUMN source_type TEXT`,
		`ALTER TABLE work_tasks ADD COLUMN source_id TEXT`,
		`ALTER TABLE work_tasks ADD COLUMN duration_min INTEGER NOT NULL DEFAULT 30`,
		`ALTER TABLE work_tasks ADD COLUMN parent_task_id TEXT`,
		`CREATE INDEX IF NOT EXISTS idx_work_tasks_work_date ON work_tasks(work_date)`,
		`CREATE INDEX IF NOT EXISTS idx_work_tasks_parent ON work_tasks(parent_task_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_work_tasks_source_role ON work_tasks(source_type, source_id, source_role)
			WHERE source_type IS NOT NULL AND source_type != ''`,
		`ALTER TABLE work_tasks ADD COLUMN customer_id TEXT`,
		`ALTER TABLE work_tasks ADD COLUMN customer_name TEXT`,
		`UPDATE work_tasks SET customer_id = (
			SELECT p.customer_id FROM work_projects p
			WHERE p.project_id = work_tasks.project_id AND TRIM(COALESCE(p.customer_id,'')) != ''
		) WHERE TRIM(COALESCE(work_tasks.customer_id,'')) = ''
		  AND TRIM(COALESCE(work_tasks.project_id,'')) != ''
		  AND work_tasks.work_type IN ('admin','support')`,
		// 완료·종료 AS 수정 잠금 해제용 설정·세션·감사 로그
		`CREATE TABLE IF NOT EXISTS app_settings (
			setting_key   TEXT PRIMARY KEY,
			setting_value TEXT NOT NULL,
			updated_at    TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS as_edit_unlocks (
			unlock_id   TEXT PRIMARY KEY,
			as_id       TEXT NOT NULL,
			user_id     TEXT NOT NULL,
			unlocked_at TEXT NOT NULL,
			expires_at  TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_as_edit_unlocks_lookup ON as_edit_unlocks(as_id, user_id)`,
		`CREATE TABLE IF NOT EXISTS as_edit_unlock_log (
			log_id      TEXT PRIMARY KEY,
			as_id       TEXT NOT NULL,
			user_id     TEXT NOT NULL,
			username    TEXT,
			success     INTEGER NOT NULL DEFAULT 0,
			ip_address  TEXT,
			created_at  TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_as_edit_unlock_log_as ON as_edit_unlock_log(as_id, created_at)`,
		// 일일 업무회의 변경 이력
		`CREATE TABLE IF NOT EXISTS data_change_logs (
			log_id TEXT PRIMARY KEY,
			occurred_at TEXT NOT NULL,
			user_id TEXT,
			username TEXT,
			user_name TEXT,
			action TEXT NOT NULL,
			table_name TEXT NOT NULL,
			pk_column TEXT,
			entity_id TEXT,
			entity_label TEXT,
			summary TEXT,
			before_json TEXT,
			after_json TEXT,
			rolled_back INTEGER DEFAULT 0,
			rolled_back_at TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_data_change_logs_at ON data_change_logs(occurred_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_data_change_logs_entity ON data_change_logs(table_name, entity_id)`,
		`CREATE TABLE IF NOT EXISTS data_backups (
			backup_id TEXT PRIMARY KEY,
			folder_name TEXT NOT NULL UNIQUE,
			kind TEXT NOT NULL,
			created_at TEXT NOT NULL,
			created_by_id TEXT,
			created_by_name TEXT,
			note TEXT
		)`,
		// folder_name UNIQUE 가 인덱스를 만든다. 중복 idx_data_backups_folder 는 049 에서 뗀다. §44.7
		`CREATE TABLE IF NOT EXISTS data_log_archives (
			archive_id TEXT PRIMARY KEY,
			folder_name TEXT NOT NULL,
			from_at TEXT,
			to_at TEXT,
			log_count INTEGER DEFAULT 0,
			created_at TEXT NOT NULL
		)`,
		`ALTER TABLE work_tasks ADD COLUMN hold_reason TEXT`,
		`ALTER TABLE work_tasks ADD COLUMN review_date TEXT`,
		`ALTER TABLE work_tasks ADD COLUMN cancel_reason TEXT`,
		`ALTER TABLE work_tasks ADD COLUMN wait_party_kind TEXT`,
		`ALTER TABLE work_tasks ADD COLUMN wait_party TEXT`,
		`ALTER TABLE work_tasks ADD COLUMN wait_request TEXT`,
		`ALTER TABLE work_tasks ADD COLUMN reply_due_date TEXT`,
		`ALTER TABLE work_tasks ADD COLUMN next_check_date TEXT`,
		`ALTER TABLE work_tasks ADD COLUMN complete_note TEXT`,
		`ALTER TABLE work_tasks ADD COLUMN receipt_date TEXT`,
		`ALTER TABLE work_tasks ADD COLUMN complete_date TEXT`,
		`CREATE TABLE IF NOT EXISTS work_actions (
			action_id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			title TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'todo',
			required INTEGER NOT NULL DEFAULT 1,
			scheduled_date TEXT,
			due_date TEXT,
			assignee TEXT,
			wait_party_kind TEXT,
			wait_party TEXT,
			wait_request TEXT,
			reply_due_date TEXT,
			next_check_date TEXT,
			confirmed INTEGER NOT NULL DEFAULT 0,
			sort_order INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (task_id) REFERENCES work_tasks(task_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_work_actions_task ON work_actions(task_id, sort_order, action_id)`,
		`CREATE TABLE IF NOT EXISTS work_activities (
			activity_id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			action_id TEXT,
			activity_type TEXT NOT NULL DEFAULT 'other',
			content TEXT NOT NULL,
			actor TEXT,
			spent_minutes INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (task_id) REFERENCES work_tasks(task_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_work_activities_task ON work_activities(task_id, created_at)`,
		`CREATE TABLE IF NOT EXISTS daily_meeting_stats (
			stat_date       TEXT NOT NULL,
			scope           TEXT NOT NULL DEFAULT 'team',
			scope_key       TEXT NOT NULL DEFAULT '',
			planned         INTEGER NOT NULL DEFAULT 0,
			receipt         INTEGER NOT NULL DEFAULT 0,
			process         INTEGER NOT NULL DEFAULT 0,
			modified        INTEGER NOT NULL DEFAULT 0,
			execution_rate  REAL NOT NULL DEFAULT 0,
			as_planned      INTEGER NOT NULL DEFAULT 0,
			as_receipt      INTEGER NOT NULL DEFAULT 0,
			as_process      INTEGER NOT NULL DEFAULT 0,
			mnt_planned     INTEGER NOT NULL DEFAULT 0,
			mnt_receipt     INTEGER NOT NULL DEFAULT 0,
			mnt_process     INTEGER NOT NULL DEFAULT 0,
			admin_planned   INTEGER NOT NULL DEFAULT 0,
			admin_receipt   INTEGER NOT NULL DEFAULT 0,
			admin_process   INTEGER NOT NULL DEFAULT 0,
			computed_at     TEXT NOT NULL,
			PRIMARY KEY (stat_date, scope, scope_key)
		)`,
	}
	for _, q := range alters {
		db.Exec(q) // 이미 있으면 오류 무시
	}

	db.Exec(`INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order) VALUES
		('PT006','product_type','KIOSK','키오스크',6),
		('RQ006','requester_type','onecall','원콜',6),
		('RC005','receipt_channel','onecall','원콜',5),
		('RC006','receipt_channel','maker','제조사요청',6),
		('RSC005','result_code','revisit_needed','재방문필요',4),
		('RSC006','result_code','partial','부분완료',2),
		('AS_S006','as_status','partial_complete','부분완료',4),
		('PCT001','project_contract_type','private','수의계약',1),
		('PCT002','project_contract_type','open_bid','일반경쟁입찰',2),
		('PCT003','project_contract_type','limited_bid','제한경쟁입찰',3),
		('PCT004','project_contract_type','designated_bid','지명경쟁입찰',4),
		('PCT005','project_contract_type','negotiated','협상에의한계약',5),
		('PCT006','project_contract_type','unit_price','단가계약',6),
		('PCT007','project_contract_type','custom','직접입력',7),
		('PBT001','project_billing_type','lump_sum','일시불',1),
		('PBT002','project_billing_type','installment','분할청구(선금·중도금·잔금)',2),
		('PBT003','project_billing_type','monthly','월정액',3),
		('PBT004','project_billing_type','quarterly','분기',4),
		('PBT005','project_billing_type','semi','반기',5),
		('PBT006','project_billing_type','yearly','연간',6),
		('PBT007','project_billing_type','custom','직접입력',7)`)
	db.Exec(`UPDATE codes SET is_active=0 WHERE code_id IN ('RSC002','RSC004')`) // 임시조치·제조사에스컬레이션 비활성
	db.Exec(`UPDATE codes SET sort_order=1, code_name='완료', is_active=1 WHERE code_id='RSC001'`)
	db.Exec(`UPDATE codes SET sort_order=2, code_name='부분완료', is_active=1 WHERE code_id='RSC006'`)
	db.Exec(`UPDATE codes SET sort_order=3, code_name='타사이관', is_active=1 WHERE code_id='RSC003'`)
	db.Exec(`UPDATE codes SET sort_order=4, code_name='재방문필요', is_active=1 WHERE code_id='RSC005'`)
	db.Exec(`UPDATE codes SET sort_order=4, code_name='부분완료', is_active=1 WHERE code_id='AS_S006'`)
	db.Exec(`UPDATE codes SET sort_order=5 WHERE code_id='AS_S004'`) // 완료
	db.Exec(`UPDATE codes SET sort_order=6 WHERE code_id='AS_S005'`) // 종료
	db.Exec(`CREATE TABLE IF NOT EXISTS as_work_items (
		work_id             TEXT PRIMARY KEY,
		work_number         TEXT NOT NULL UNIQUE,
		as_id               TEXT NOT NULL,
		work_kind           TEXT NOT NULL,
		scheduled_date      TEXT,
		schedule_confirmed  INTEGER DEFAULT 0,
		confirm_target      TEXT,
		confirm_contact     TEXT,
		assigned_to         TEXT,
		assigned_user_id    TEXT,
		status              TEXT DEFAULT 'open',
		notes               TEXT,
		created_at          DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at          DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	db.Exec(`ALTER TABLE as_work_items ADD COLUMN assigned_to TEXT`)
	db.Exec(`ALTER TABLE as_work_items ADD COLUMN assigned_user_id TEXT`)
	// 기존 하부업무: 담당자 비어 있으면 원 접수 담당자를 초기값으로만 복사(이후 독립)
	db.Exec(`
		UPDATE as_work_items
		SET assigned_to=(SELECT ar.assigned_to FROM as_receipts ar WHERE ar.as_id=as_work_items.as_id),
		    assigned_user_id=(SELECT ar.assigned_user_id FROM as_receipts ar WHERE ar.as_id=as_work_items.as_id)
		WHERE (assigned_to IS NULL OR TRIM(assigned_to)='')
		  AND EXISTS (SELECT 1 FROM as_receipts ar WHERE ar.as_id=as_work_items.as_id
		              AND ar.assigned_to IS NOT NULL AND TRIM(ar.assigned_to)!='')`)

	// 기존 배정명을 user_id로 보강
	db.Exec(`
		UPDATE as_receipts SET assigned_user_id=(
			SELECT u.user_id FROM users u
			WHERE TRIM(u.full_name)=TRIM(as_receipts.assigned_to)
			   OR TRIM(u.username)=TRIM(as_receipts.assigned_to)
			LIMIT 1
		)
		WHERE (assigned_user_id IS NULL OR assigned_user_id='')
		  AND assigned_to IS NOT NULL AND TRIM(assigned_to)!=''`)

	// 기존 is_primary → contact_role 보강
	db.Exec(`UPDATE contacts SET contact_role='primary' WHERE is_primary=1 AND (contact_role IS NULL OR contact_role='')`)
	db.Exec(`UPDATE contacts SET contact_role='regular' WHERE (contact_role IS NULL OR contact_role='')`)
	db.Exec(`UPDATE contacts SET affiliation='institution' WHERE affiliation IS NULL OR affiliation=''`)

	// 점검주기 코드를 청구주기와 동일 체계로 동기화 (기존 Call/연/없음 시드 교체)
	db.Exec(`DELETE FROM codes WHERE code_group='maint_cycle'`)
	db.Exec(`INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order) VALUES
		('MCY001','maint_cycle','monthly','월',1),
		('MCY002','maint_cycle','quarterly','분기',2),
		('MCY003','maint_cycle','semi','반기',3),
		('MCY004','maint_cycle','odd_bimonthly','홀수격월',4),
		('MCY005','maint_cycle','even_bimonthly','짝수격월',5),
		('MCY006','maint_cycle','custom','직접입력',6)`)

	// 관리유형: 타사장비 (당사 비관리 · 연동 참고)
	db.Exec(`INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order) VALUES
		('MT006','management_type','third_party','타사장비',6)`)

	// 담당자 직급 코드 시드 (기존 DB에도 반영)
	db.Exec(`INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order) VALUES
		('JG001','job_grade','librarian','사서직',1),
		('JG002','job_grade','it','전산직',2),
		('JG003','job_grade','other','기타',3),
		('JG004','job_grade','custom','직접입력',4)`)

	// 담당구분 · 변경사유 · 소속 코드
	db.Exec(`INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order) VALUES
		('CROLE001','contact_role','primary','주담당',1),
		('CROLE002','contact_role','secondary','부담당',2),
		('CROLE003','contact_role','regular','일반 담당자',3),
		('CHRS001','contact_change_reason','transfer','전보',1),
		('CHRS002','contact_change_reason','resign','퇴직',2),
		('CHRS003','contact_change_reason','role_adjust','업무조정',3),
		('CAFF001','contact_affiliation','institution','소속기관',1),
		('CAFF002','contact_affiliation','integrator','통합사업자',2),
		('CAFF003','contact_affiliation','partner','협력업체',3),
		('CAFF004','contact_affiliation','other','기타',4)`)

	// 통계 분류 코드
	db.Exec(`INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order) VALUES
		('SPC001','stats_product_category','homepage','홈페이지',1),
		('SPC002','stats_product_category','elibrary','전자도서관',2),
		('SPC003','stats_product_category','mobile','모바일',3),
		('SPC004','stats_product_category','rfid','RFID자동화',4),
		('SPC005','stats_product_category','materials','자료관리',5),
		('SPC006','stats_product_category','other','기타',6),
		('SWF001','work_form','maintenance','정기점검',1),
		('SWF002','work_form','as','AS',2),
		('SWF003','work_form','install','설치',3),
		('SWF004','work_form','other','기타',4),
		('SRF001','receipt_form','onecall','원콜',1),
		('SRF002','receipt_form','phone','전화',2),
		('SRF003','receipt_form','manufacturer','제조사요청',3),
		('SRF004','receipt_form','onsite','현장',4),
		('SRF005','receipt_form','mail','메일',5)`)

	// 기존 설치자산 제품분류 기본값(RFID자동화)은 1회성 백필이다.
	// 매번 실행하면 타사 장비를 다른 분류로 비워둔 경우까지 RFID로 되돌린다.
	if !metaDone(db, assetCategoryBackfillMetaKey) {
		db.Exec(`UPDATE assets SET product_category='rfid' WHERE product_category IS NULL OR TRIM(product_category)=''`)
		markMetaDone(db, assetCategoryBackfillMetaKey)
	}

	// AS 엑셀 적재 중 기관명이 매칭되지 않아 자동 생성된 고객은 사람이 확인해야 한다.
	// 표식을 지운 뒤 다시 켜지면 안 되므로 1회만 세운다.
	if !metaDone(db, importedCustomerReviewMetaKey) {
		db.Exec(`UPDATE customers SET needs_review=1, review_reason=?
			WHERE COALESCE(needs_review,0)=0 AND notes LIKE '%AS 완료내역 엑셀 적재%'`,
			ReviewReasonImportedCustomer)
		markMetaDone(db, importedCustomerReviewMetaKey)
	}

	// 기관명·공식명칭에 '도서관'이 있으면 업종을 도서관으로 통일
	db.Exec(`UPDATE customers SET industry='도서관', updated_at=CURRENT_TIMESTAMP
		WHERE (org_name LIKE '%도서관%' OR official_name LIKE '%도서관%')
		  AND COALESCE(industry,'') != '도서관'`)

	// AS 워크플로 상태 보정: 일정확정→진행중, 배정만→담당자배정, 그 외→접수
	// 기존 진행중 건은 일정 확정으로 이관(1회성 의미, 이후 필드 기준 재파생)
	db.Exec(`UPDATE as_receipts SET schedule_confirmed=1
		WHERE status='in_progress' AND COALESCE(schedule_confirmed,0)=0
		  AND TRIM(COALESCE(visit_scheduled_date,'')) != ''`)
	db.Exec(`UPDATE as_receipts SET status='in_progress', updated_at=CURRENT_TIMESTAMP
		WHERE status IN ('received','assigned','in_progress','')
		  AND COALESCE(schedule_confirmed,0)=1`)
	db.Exec(`UPDATE as_receipts SET status='assigned', updated_at=CURRENT_TIMESTAMP
		WHERE status IN ('received','assigned','in_progress','')
		  AND COALESCE(schedule_confirmed,0)=0
		  AND (
		    (assigned_to IS NOT NULL AND TRIM(assigned_to)!='')
		    OR (assigned_user_id IS NOT NULL AND TRIM(assigned_user_id)!='')
		  )`)
	db.Exec(`UPDATE as_receipts SET status='received', updated_at=CURRENT_TIMESTAMP
		WHERE status IN ('received','assigned','in_progress','')
		  AND COALESCE(schedule_confirmed,0)=0
		  AND (assigned_to IS NULL OR TRIM(assigned_to)='')
		  AND (assigned_user_id IS NULL OR TRIM(assigned_user_id)='')`)

	db.Exec(`UPDATE maintenance_site_config SET inspection_cycle='monthly'
		WHERE inspection_cycle IS NULL OR TRIM(inspection_cycle)=''`)

	migrateCustomerStructuredAddresses(db)
	seedDefaultProjects(db)
	renameSeedProjectDisplayNames(db)
	dedupeWorkProjectsByName(db)
	linkRFIDAssetsToAnroboticsProject(db)
	linkMaterialsChungnamToSWProject(db)
	linkSejongLibraryToICTProject(db)
	uppercaseAssetProductTypes(db)
	backfillASPlannedDailyTasks(db)
	applyAppendixB(db)
	applyS11WorkTaskDates(db)
	applyASReceiptsProjectID(db)
	applyWeeklyReportRows(db)
	applyASReceiptsCauseReport(db)
	applyHolidaySource(db)
	applyStaffLeaves(db)
	applyWorkTaskMembers(db)
	applyV214ProjectKindRollback(db)
	applySalesProjects(db)
	applySalesActivities(db)
	applySalesParties(db)
	applySalesPromote(db)
	applySalesStagesV227(db)
	applySalesActivityTypesV227(db)
	applySalesPartyAutoV227(db)
	applyWorkRecurrence(db)
	applyInboxToWaiting(db)
	applyMetricsSettings(db)
	applyASSearch(db)
	applyASKeywords(db)
	seedKeywordLinksIfNeeded(db)
	applyASImport(db)
	applyASReceiptGroup(db)
	applyAS34ReceiptUX(db)
	applyAS34ActionUX(db)
	applyAS47VisitDate(db)
	applyWorkListIndexes(db)
	applyQueryPlanIndexes(db)
	applyAS34TransferFollowup(db)
	applyRegionDistanceOrder(db)
	applyASCauseCategoriesV2(db)
	applyASCaseVotes(db)
	applyASKbEntries(db)
	applyASKbGaps(db)
	applyWorkTaskAssigneeSource(db)
	applyWorkAssignNotices(db)
	applyWorkTaskBlocked(db)
	applyAssigneeUserIDs(db)
	applyASProcessTruth(db)
	applyNF1(db)
	applyAppVersions(db)
	applySalesTaskType(db)
	applySalesStageLabels(db)
	applySalesMemos(db)
	applyWorkProjectContract(db)

	// 미정+사유 등록일(§8.1 재검토). 부록 B.1 컬럼을 바꾸지 않고 기존 테이블에만 추가한다.
	if _, err := db.Exec(`ALTER TABLE as_receipts ADD COLUMN schedule_no_date_at TEXT`); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		log.Printf("schedule_no_date_at: %v", err)
	}
	db.Exec(`UPDATE as_receipts SET schedule_no_date_at = date(updated_at)
		WHERE TRIM(COALESCE(schedule_no_date_reason,'')) != ''
		  AND TRIM(COALESCE(schedule_no_date_at,'')) = ''`)

	if err := migrateBusinessIDsV2(db); err != nil {
		log.Printf("warning: business id migrate v2: %v", err)
	}

	if err := migrateAssetIDsToASCII(db); err != nil {
		log.Printf("warning: asset id ascii migrate: %v", err)
	}

	BackfillAssetImageSlots(db)
	migrateUserRolesAndPermissions(db)
	LoadLookupCache(db)

	return nil
}

// migrateUserRolesAndPermissions 등급 체계·권한 CSV 컬럼 보강
func migrateUserRolesAndPermissions(db *sql.DB) {
	db.Exec(`ALTER TABLE users ADD COLUMN permissions TEXT DEFAULT ''`)
	// 레거시 역할 명칭 정리
	db.Exec(`UPDATE users SET role='office' WHERE role IN ('receipt','user','접수','접수담당')`)
	db.Exec(`UPDATE users SET role='observer' WHERE role IN ('viewer','열람','열람사용자')`)
	// 권한이 비어 있으면 등급 기본값 채움
	rows, err := db.Query(`SELECT user_id, role, COALESCE(permissions,'') FROM users`)
	if err != nil {
		return
	}
	defer rows.Close()
	type row struct{ id, role, perms string }
	var list []row
	for rows.Next() {
		var r row
		if rows.Scan(&r.id, &r.role, &r.perms) == nil {
			list = append(list, r)
		}
	}
	for _, r := range list {
		role := model.NormalizeRole(r.role)
		perms := strings.TrimSpace(r.perms)
		// 옵저버: 통계만/빈 값 → 전체 조회 기본 권한
		if role == model.RoleObserver && (perms == "" || perms == "stats") {
			db.Exec(`UPDATE users SET role=?, permissions=? WHERE user_id=?`,
				role, model.FormatPermissions(model.ObserverViewPermissions()), r.id)
			continue
		}
		if perms != "" {
			continue
		}
		db.Exec(`UPDATE users SET role=?, permissions=? WHERE user_id=?`,
			role, model.FormatPermissions(model.DefaultPermissions(role)), r.id)
	}
}

// migrateCustomerStructuredAddresses 구 address → 우편번호/시도/군구/동/상세 분해 이관
func migrateCustomerStructuredAddresses(db *sql.DB) {
	rows, err := db.Query(`
		SELECT customer_id, COALESCE(address,''), COALESCE(address_detail,'')
		FROM customers
		WHERE (addr_sido IS NULL OR TRIM(addr_sido)='')
		  AND (
		    (address IS NOT NULL AND TRIM(address)!='')
		    OR (address_detail IS NOT NULL AND TRIM(address_detail)!='')
		  )`)
	if err != nil {
		return
	}
	defer rows.Close()
	type row struct{ id, addr, det string }
	var list []row
	for rows.Next() {
		var r row
		if rows.Scan(&r.id, &r.addr, &r.det) != nil {
			continue
		}
		list = append(list, r)
	}
	for _, r := range list {
		pc, sido, sigungu, dong, det := model.ParseLegacyCustomerAddress(r.addr, r.det)
		c := &model.Customer{
			PostalCode: pc, AddrSido: sido, AddrSigungu: sigungu, AddrDong: dong, AddressDetail: det,
		}
		c.SyncCombinedAddress()
		if c.Address == "" {
			c.Address = strings.TrimSpace(r.addr)
		}
		_, _ = db.Exec(`UPDATE customers SET
			postal_code=?, addr_sido=?, addr_sigungu=?, addr_dong=?,
			address=?, address_detail=?, updated_at=CURRENT_TIMESTAMP
			WHERE customer_id=?`,
			pc, sido, sigungu, dong, c.Address, det, r.id)
	}
}
