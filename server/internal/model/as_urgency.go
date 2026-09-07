package model

import "strings"

const (
	UrgencyReasonNone  = "none"
	UrgencyReasonOther = "other"
	PartyKindCustomer  = "customer"
	PartyKindPartner   = "partner"
	PartyKindOwn       = "own"
	PartyKindVendor    = "vendor"
)

// UrgencyFromReason 해당 없음이 아니면 긴급(high). §34.2.2
func UrgencyFromReason(code string) string {
	code = strings.TrimSpace(code)
	if code == "" || code == UrgencyReasonNone {
		return "normal"
	}
	return "high"
}

// SyncScheduleConfirmed 예정일이 있으면 확정, 미정(+사유)이면 0. §34.2.3
func SyncScheduleConfirmed(visitDate string) bool {
	return strings.TrimSpace(visitDate) != ""
}

// UrgencyReasonFamilyFromGroup codes.code_group → rfid|klas|web|common
func UrgencyReasonFamilyFromGroup(group string) string {
	return strings.TrimPrefix(strings.TrimSpace(group), "urgency_reason_")
}

// AssetUrgencyFamily 자산의 제품군. 긴급 사유 드롭다운 필터. §34.2.2
func AssetUrgencyFamily(category, productName, productType string) string {
	cat := strings.ToLower(strings.TrimSpace(category))
	blob := strings.ToLower(productName + " " + productType + " " + category)
	switch cat {
	case "rfid":
		return "rfid"
	case "homepage", "elibrary", "mobile":
		return "web"
	case "materials":
		return "klas"
	}
	switch {
	case strings.Contains(blob, "rfid") || strings.Contains(blob, "앤로보") ||
		strings.Contains(blob, "자가대출") || strings.Contains(blob, "반납기"):
		return "rfid"
	case strings.Contains(blob, "klas") || strings.Contains(blob, "k-las") ||
		strings.Contains(blob, "k-las"):
		return "klas"
	case strings.Contains(blob, "홈페이지") || strings.Contains(blob, "전자도서") ||
		strings.Contains(blob, "elibrary"):
		return "web"
	}
	return "common"
}

// UrgencyReasonLabel 목록·상세 표시용. other 이면 note 를 쓴다.
func UrgencyReasonLabel(code, note string, names map[string]string) string {
	code = strings.TrimSpace(code)
	note = strings.TrimSpace(note)
	if code == "" || code == UrgencyReasonNone {
		return ""
	}
	if code == UrgencyReasonOther {
		if note != "" {
			return note
		}
		return "기타"
	}
	if names != nil {
		if n := strings.TrimSpace(names[code]); n != "" {
			return n
		}
	}
	if n := strings.TrimSpace(urgencyReasonDefaultNames[code]); n != "" {
		return n
	}
	if note != "" {
		return note
	}
	return code
}

// NormalizePartyKind 빈값·모르는 값은 customer. §34.2.5
func NormalizePartyKind(k string) string {
	switch strings.TrimSpace(k) {
	case PartyKindPartner, PartyKindOwn, PartyKindVendor, PartyKindCustomer:
		return strings.TrimSpace(k)
	default:
		return PartyKindCustomer
	}
}

// PartyKindLabel 고객현황 구분 표시.
func PartyKindLabel(k string) string {
	switch NormalizePartyKind(k) {
	case PartyKindPartner:
		return "협력사"
	case PartyKindOwn:
		return "자사"
	case PartyKindVendor:
		return "제조사·공급사"
	default:
		return "고객"
	}
}

var urgencyReasonDefaultNames = map[string]string{
	"rfid_ops_stop":  "장비 운영 불가",
	"rfid_loan_stop": "대출/반납 전면 중단",
	"rfid_safety":    "안전 위험",
	"klas_server":    "서버 다운",
	"klas_admin":     "자료관리 접속 불가",
	"klas_loan":      "대출/반납 처리 불가",
	"klas_db":        "DB 오류",
	"web_site":       "사이트 접속 불가",
	"web_login":      "로그인 전면 불가",
	"other":          "기타",
}
