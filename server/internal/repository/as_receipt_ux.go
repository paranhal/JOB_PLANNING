package repository

import (
	"database/sql"
	"log"
	"strings"
)

// applyAS34ReceiptUX 마이그레이션 026. §34.2.2~§34.2.6
func applyAS34ReceiptUX(db *sql.DB) {
	if _, err := db.Exec(`ALTER TABLE customers ADD COLUMN party_kind TEXT DEFAULT 'customer'`); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		log.Printf("026 customers.party_kind: %v", err)
	}
	if _, err := db.Exec(`ALTER TABLE as_receipts ADD COLUMN urgency_reason TEXT`); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		log.Printf("026 as_receipts.urgency_reason: %v", err)
	}
	if _, err := db.Exec(`ALTER TABLE as_receipts ADD COLUMN urgency_reason_note TEXT`); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		log.Printf("026 as_receipts.urgency_reason_note: %v", err)
	}

	db.Exec(`UPDATE customers SET party_kind='own'
		WHERE customer_id='C000-26-008' OR org_name LIKE '%비젼아이티%'`)
	db.Exec(`UPDATE customers SET party_kind='partner'
		WHERE customer_id='C000-26-009' OR org_name LIKE '%채움씨앤아이%'`)
	db.Exec(`UPDATE customers SET party_kind='customer' WHERE TRIM(COALESCE(party_kind,''))=''`)

	db.Exec(`UPDATE codes SET code_name='협력사' WHERE code_id='RC004' AND code_group='receipt_channel'`)
	db.Exec(`UPDATE codes SET code_name='제조사' WHERE code_id='RC006' AND code_group='receipt_channel'`)
	db.Exec(`INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order, is_active) VALUES
		('RC007','receipt_channel','prime','원청',7,1),
		('RC008','receipt_channel','internal','내부',8,1),
		('URR_C1','urgency_reason_common','none','해당 없음',1,1),
		('URR_C2','urgency_reason_common','other','기타(직접 입력)',99,1),
		('URR_R1','urgency_reason_rfid','rfid_ops_stop','장비 운영 불가',10,1),
		('URR_R2','urgency_reason_rfid','rfid_loan_stop','대출/반납 전면 중단',11,1),
		('URR_R3','urgency_reason_rfid','rfid_safety','안전 위험',12,1),
		('URR_K1','urgency_reason_klas','klas_server','서버 다운',10,1),
		('URR_K2','urgency_reason_klas','klas_admin','자료관리 접속 불가',11,1),
		('URR_K3','urgency_reason_klas','klas_loan','대출/반납 처리 불가',12,1),
		('URR_K4','urgency_reason_klas','klas_db','DB 오류',13,1),
		('URR_W1','urgency_reason_web','web_site','사이트 접속 불가',10,1),
		('URR_W2','urgency_reason_web','web_login','로그인 전면 불가',11,1)`)
}
