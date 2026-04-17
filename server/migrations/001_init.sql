-- 고객지원시스템 초기 스키마
-- 기획서 §12 논리 데이터 모델 기준
-- 이 파일은 참조용입니다. 실제 스키마는 internal/repository/db.go에서 자동 실행됩니다.

-- PostgreSQL 전환 시 사용할 스키마 (SQLite와 차이 주석 표시)

CREATE TABLE IF NOT EXISTS codes (
    code_id     VARCHAR(50)  PRIMARY KEY,
    code_group  VARCHAR(50)  NOT NULL,
    code_value  VARCHAR(100) NOT NULL,
    code_name   VARCHAR(100) NOT NULL,
    sort_order  INTEGER      DEFAULT 0,
    is_active   BOOLEAN      DEFAULT TRUE
);

CREATE TABLE IF NOT EXISTS users (
    user_id       VARCHAR(50)  PRIMARY KEY,
    username      VARCHAR(100) NOT NULL UNIQUE,
    password_hash TEXT         NOT NULL,
    full_name     VARCHAR(100) NOT NULL,
    role          VARCHAR(20)  NOT NULL DEFAULT 'viewer',
    is_active     BOOLEAN      DEFAULT TRUE,
    created_at    TIMESTAMP    DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS customers (
    customer_id        VARCHAR(50)  PRIMARY KEY,
    org_name           VARCHAR(200) NOT NULL,
    official_name      VARCHAR(200) NOT NULL,
    org_email          VARCHAR(200),
    main_phone         VARCHAR(50),
    website            VARCHAR(300),
    business_number    VARCHAR(20),
    representative     VARCHAR(100),
    industry           VARCHAR(50),
    has_parent         BOOLEAN      DEFAULT FALSE,
    parent_customer_id VARCHAR(50)  REFERENCES customers(customer_id),
    address            TEXT,
    address_detail     TEXT,
    is_active          BOOLEAN      DEFAULT TRUE,
    notes              TEXT,
    created_at         TIMESTAMP    DEFAULT CURRENT_TIMESTAMP,
    updated_at         TIMESTAMP    DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS customer_buildings (
    building_id   VARCHAR(50)  PRIMARY KEY,
    customer_id   VARCHAR(50)  NOT NULL REFERENCES customers(customer_id),
    building_name VARCHAR(200) NOT NULL,
    building_type VARCHAR(50),
    address       TEXT,
    is_active     BOOLEAN      DEFAULT TRUE,
    created_at    TIMESTAMP    DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS customer_floors (
    floor_id    VARCHAR(50)  PRIMARY KEY,
    building_id VARCHAR(50)  NOT NULL REFERENCES customer_buildings(building_id),
    floor_name  VARCHAR(50)  NOT NULL,
    sort_order  INTEGER      DEFAULT 0,
    created_at  TIMESTAMP    DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS customer_rooms (
    room_id     VARCHAR(50)  PRIMARY KEY,
    floor_id    VARCHAR(50)  NOT NULL REFERENCES customer_floors(floor_id),
    room_name   VARCHAR(100) NOT NULL,
    room_number VARCHAR(50),
    purpose     VARCHAR(100),
    created_at  TIMESTAMP    DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS contacts (
    contact_id  VARCHAR(50)  PRIMARY KEY,
    customer_id VARCHAR(50)  NOT NULL REFERENCES customers(customer_id),
    full_name   VARCHAR(100) NOT NULL,
    job_role    VARCHAR(100),
    title       VARCHAR(100),
    phone       VARCHAR(50),
    mobile      VARCHAR(50),
    email       VARCHAR(200),
    start_date  DATE,
    end_date    DATE,
    status      VARCHAR(20)  DEFAULT 'active',
    is_primary  BOOLEAN      DEFAULT FALSE,
    notes       TEXT,
    created_at  TIMESTAMP    DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS contact_history (
    history_id    VARCHAR(50) PRIMARY KEY,
    contact_id    VARCHAR(50) NOT NULL REFERENCES contacts(contact_id),
    customer_id   VARCHAR(50) NOT NULL,
    start_date    DATE,
    end_date      DATE,
    department    VARCHAR(100),
    job_role      VARCHAR(100),
    title         VARCHAR(100),
    phone         VARCHAR(50),
    email         VARCHAR(200),
    status        VARCHAR(20),
    change_reason TEXT,
    created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS assets (
    asset_id            VARCHAR(50)  PRIMARY KEY,
    customer_id         VARCHAR(50)  NOT NULL REFERENCES customers(customer_id),
    product_name        VARCHAR(200) NOT NULL,
    product_type        VARCHAR(50),
    model_name          VARCHAR(200),
    manufacturer        VARCHAR(100),
    serial_number       VARCHAR(100),
    install_date        DATE,
    retire_date         DATE,
    installer_type      VARCHAR(50),
    original_installer  VARCHAR(100),
    operation_status    VARCHAR(50)  DEFAULT 'operating',
    management_type     VARCHAR(50),
    is_managed          BOOLEAN      DEFAULT TRUE,
    requester_type      VARCHAR(50),
    requester_name      VARCHAR(100),
    customer_contact_id VARCHAR(50),
    our_contact         VARCHAR(100),
    building_id         VARCHAR(50),
    floor_id            VARCHAR(50),
    room_id             VARCHAR(50),
    loc_building_name   VARCHAR(200),
    loc_floor_name      VARCHAR(100),
    loc_room_name       VARCHAR(200),
    location_detail     TEXT,
    notes               TEXT,
    created_at          TIMESTAMP    DEFAULT CURRENT_TIMESTAMP,
    updated_at          TIMESTAMP    DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS asset_sw_details (
    sw_detail_id  VARCHAR(50)  PRIMARY KEY,
    asset_id      VARCHAR(50)  NOT NULL REFERENCES assets(asset_id),
    software_name VARCHAR(200),
    version       VARCHAR(50),
    install_type  VARCHAR(50),
    hw_info       TEXT,
    os            VARCHAR(100),
    os_version    VARCHAR(50),
    dbms          VARCHAR(100),
    db_version    VARCHAR(50),
    access_method VARCHAR(50),
    access_url    TEXT,
    install_path  TEXT,
    backup_path   TEXT,
    config_path   TEXT,
    created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS performance_relations (
    relation_id   VARCHAR(50)  PRIMARY KEY,
    customer_id   VARCHAR(50),
    asset_id      VARCHAR(50),
    relation_type VARCHAR(100) NOT NULL,
    company_type  VARCHAR(50),
    company_name  VARCHAR(200),
    contact_name  VARCHAR(100),
    contact_phone VARCHAR(50),
    contact_email VARCHAR(200),
    start_date    DATE,
    end_date      DATE,
    is_active     BOOLEAN  DEFAULT TRUE,
    notes         TEXT,
    created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS as_receipts (
    as_id               VARCHAR(50)  PRIMARY KEY,
    as_number           VARCHAR(50)  NOT NULL UNIQUE,
    receipt_datetime    TIMESTAMP    DEFAULT CURRENT_TIMESTAMP,
    customer_id         VARCHAR(50)  NOT NULL REFERENCES customers(customer_id),
    asset_id            VARCHAR(50)  REFERENCES assets(asset_id),
    receipt_channel     VARCHAR(50),
    requester           VARCHAR(100),
    symptom             TEXT,
    urgency             VARCHAR(20)  DEFAULT 'normal',
    priority            VARCHAR(20)  DEFAULT 'normal',
    requester_type      VARCHAR(50),
    requester_name      VARCHAR(100),
    assigned_to         VARCHAR(100),
    status              VARCHAR(20)  DEFAULT 'received',
    start_datetime      TIMESTAMP,
    complete_datetime   TIMESTAMP,
    process_type        VARCHAR(50),
    cause_type          VARCHAR(50),
    action_taken        TEXT,
    parts_used          TEXT,
    is_recurrence       BOOLEAN      DEFAULT FALSE,
    result_code         VARCHAR(50),
    customer_confirmer  VARCHAR(100),
    confirm_datetime    TIMESTAMP,
    followup_action     TEXT,
    created_at          TIMESTAMP    DEFAULT CURRENT_TIMESTAMP,
    updated_at          TIMESTAMP    DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS as_processes (
    process_id       VARCHAR(50) PRIMARY KEY,
    as_id            VARCHAR(50) NOT NULL REFERENCES as_receipts(as_id),
    process_datetime TIMESTAMP   DEFAULT CURRENT_TIMESTAMP,
    worker           VARCHAR(100),
    work_type        VARCHAR(50),
    work_content     TEXT,
    parts_used       TEXT,
    time_spent       INTEGER,
    notes            TEXT
);

CREATE TABLE IF NOT EXISTS attachments (
    attachment_id VARCHAR(50)  PRIMARY KEY,
    ref_type      VARCHAR(20)  NOT NULL,
    ref_id        VARCHAR(50)  NOT NULL,
    file_name     VARCHAR(300) NOT NULL,
    file_path     TEXT         NOT NULL,
    file_size     BIGINT,
    mime_type     VARCHAR(100),
    uploaded_at   TIMESTAMP    DEFAULT CURRENT_TIMESTAMP
);

-- 인덱스 (PostgreSQL 전환 시 추가 권장)
-- CREATE INDEX idx_customers_org_name ON customers(org_name);
-- CREATE INDEX idx_assets_customer_id ON assets(customer_id);
-- CREATE INDEX idx_as_receipts_customer_id ON as_receipts(customer_id);
-- CREATE INDEX idx_as_receipts_status ON as_receipts(status);
-- CREATE INDEX idx_as_receipts_receipt_datetime ON as_receipts(receipt_datetime);
