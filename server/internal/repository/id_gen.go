package repository

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	idFormatV2MetaKey             = "__meta:id_format_v2"
	assetCategoryBackfillMetaKey  = "__meta:asset_category_backfill"
	assetIDASCIIMetaKey           = "__meta:asset_id_ascii"
	importedCustomerReviewMetaKey = "__meta:imported_customer_review"
	assetRFIDProjectLinkMetaKey      = "__meta:asset_rfid_project_wpseed03_v1"
	assetMaterialsChungnamLinkMetaKey = "__meta:asset_materials_chungnam_wpseed01_v1"
	assetSejongLibraryICTLinkMetaKey  = "__meta:asset_sejong_library_ict_wpseed02_v2"
	projectDisplayNamesV2MetaKey      = "__meta:project_display_names_v2"
	assetProductTypeUpperMetaKey      = "__meta:asset_product_type_upper_v1"
)

// 시드 사업 ID
const (
	ProjectIDAnroboticsRFID = "WPSEED03" // 충남세종 앤로보틱스 RFID
	ProjectIDChungnamSW2026 = "WPSEED01" // 2026 충남교육청 SW 유지관리
	ProjectIDSejongICT2026  = "WPSEED02" // 세종시 도서관 ICT 2026
)

// metaDone 1회성 마이그레이션이 이미 끝났는지 확인한다.
func metaDone(db *sql.DB, key string) bool {
	var done int
	err := db.QueryRow(`SELECT last_no FROM id_sequences WHERE seq_key=?`, key).Scan(&done)
	return err == nil && done >= 1
}

func markMetaDone(db *sql.DB, key string) {
	db.Exec(`INSERT OR REPLACE INTO id_sequences(seq_key,last_no) VALUES(?,1)`, key)
}

// NextSeq 시퀀스 키에 대한 다음 일련번호(1부터)를 원자적으로 반환한다.
func NextSeq(db *sql.DB, seqKey string) (int, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var last int
	err = tx.QueryRow(`SELECT last_no FROM id_sequences WHERE seq_key=?`, seqKey).Scan(&last)
	if err == sql.ErrNoRows {
		if _, err := tx.Exec(`INSERT INTO id_sequences (seq_key, last_no) VALUES (?, 1)`, seqKey); err != nil {
			return 0, err
		}
		last = 1
	} else if err != nil {
		return 0, err
	} else {
		last++
		if _, err := tx.Exec(`UPDATE id_sequences SET last_no=? WHERE seq_key=?`, last, seqKey); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return last, nil
}

func fmtSeq3(n int) string { return fmt.Sprintf("%03d", n) }
func fmtSeq2(n int) string { return fmt.Sprintf("%02d", n) }

// ExtractAreaCode 대표전화에서 지역번호 추출 (02 또는 0xx). 없으면 000.
func ExtractAreaCode(phone string) string {
	digits := regexp.MustCompile(`\D`).ReplaceAllString(phone, "")
	if digits == "" {
		return "000"
	}
	if strings.HasPrefix(digits, "02") {
		return "02"
	}
	if len(digits) >= 3 && digits[0] == '0' {
		return digits[:3]
	}
	return "000"
}

// NormalizeModelCode 모델/제품명/제조사를 ID용 코드로 정규화한다.
//
// 자산번호는 URL 경로와 이미지 파일 경로에 그대로 쓰이므로 ASCII 영숫자와
// 하이픈만 남긴다. 타사 장비는 모델명이 없고 제품명이 한글뿐인 경우가 많아
// 모델명 → 제품명 → 제조사 순으로 ASCII를 찾고, 모두 없으면 ETC를 쓴다.
func NormalizeModelCode(model, product, manufacturer string) string {
	for _, s := range []string{model, product, manufacturer} {
		if code := asciiIDCode(s); code != "" {
			return code
		}
	}
	return "ETC"
}

func asciiIDCode(s string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case r < utf8.RuneSelf && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			b.WriteRune(unicode.ToUpper(r))
			prevDash = false
		// 한글 등 ASCII가 아닌 문자는 구분자로 취급해 앞뒤 코드가 붙지 않게 한다.
		case r == '-' || r == '_' || r == ' ' || r == '/' || r >= utf8.RuneSelf:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 40 {
		out = strings.Trim(out[:40], "-")
	}
	return out
}

// ProductTypeCode 제품구분 → 구분번호 (키오스크=1, SW=2, 그 외 HW=3)
func ProductTypeCode(productType string) string {
	t := strings.ToLower(strings.TrimSpace(productType))
	switch {
	case t == "kiosk" || strings.Contains(t, "키오스크") || strings.Contains(t, "kiosk"):
		return "1"
	case t == "sw" || strings.Contains(t, "software") || t == "소프트웨어":
		return "2"
	default:
		return "3"
	}
}

// NextCustomerID 고객번호: C{지역코드}-{YY}-{NNN}
func NextCustomerID(db *sql.DB, mainPhone string) (string, error) {
	area := ExtractAreaCode(mainPhone)
	yy := time.Now().Format("06")
	key := fmt.Sprintf("customer:%s:%s", area, yy)
	n, err := NextSeq(db, key)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("C%s-%s-%s", area, yy, fmtSeq3(n)), nil
}

// NextAssetID 설치자산: A{모델코드}{구분}-{NNN}
func NextAssetID(db *sql.DB, modelName, productName, productType, manufacturer string) (string, error) {
	model := NormalizeModelCode(modelName, productName, manufacturer)
	code := ProductTypeCode(productType)
	key := fmt.Sprintf("asset:%s:%s", model, code)
	n, err := NextSeq(db, key)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("A%s%s-%s", model, code, fmtSeq3(n)), nil
}

// NextASNumber 접수번호: R{YYMM}-{NNN} (as_id와 as_number 동일)
func NextASNumber(db *sql.DB, at time.Time) (string, error) {
	ym := at.Format("0601")
	key := fmt.Sprintf("as:%s", ym)
	n, err := NextSeq(db, key)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("R%s-%s", ym, fmtSeq3(n)), nil
}

// NextProcessNumber 처리번호: {접수번호}-P{NN}
func NextProcessNumber(db *sql.DB, asNumber string, _ time.Time) (string, error) {
	key := fmt.Sprintf("process:%s", asNumber)
	n, err := NextSeq(db, key)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-P%s", asNumber, fmtSeq2(n)), nil
}

// NextWorkNumber 하부업무번호: {접수번호}-W{NN}
func NextWorkNumber(db *sql.DB, asNumber string) (string, error) {
	key := fmt.Sprintf("work:%s", asNumber)
	n, err := NextSeq(db, key)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-W%s", asNumber, fmtSeq2(n)), nil
}
