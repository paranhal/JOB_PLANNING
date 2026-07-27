package repository

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
)

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

func fmtSeq(n int) string {
	return fmt.Sprintf("%04d", n)
}

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

// NormalizeModelCode 모델/제품명을 ID용으로 정규화
func NormalizeModelCode(model, product string) string {
	s := strings.TrimSpace(model)
	if s == "" {
		s = strings.TrimSpace(product)
	}
	if s == "" {
		s = "MODEL"
	}
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToUpper(r))
			prevDash = false
		case r == '-' || r == '_' || r == ' ' || r == '/':
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "MODEL"
	}
	if len(out) > 40 {
		out = out[:40]
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

// NextCustomerID 고객번호: {지역}-{YYYY}-{NNNN}
func NextCustomerID(db *sql.DB, mainPhone string) (string, error) {
	area := ExtractAreaCode(mainPhone)
	year := time.Now().Format("2006")
	key := fmt.Sprintf("customer:%s:%s", area, year)
	n, err := NextSeq(db, key)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s-%s", area, year, fmtSeq(n)), nil
}

// NextAssetID 설치자산: {모델}-{구분}-{NNNN}
func NextAssetID(db *sql.DB, modelName, productName, productType string) (string, error) {
	model := NormalizeModelCode(modelName, productName)
	code := ProductTypeCode(productType)
	key := fmt.Sprintf("asset:%s:%s", model, code)
	n, err := NextSeq(db, key)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s-%s", model, code, fmtSeq(n)), nil
}

// NextASNumber 접수번호: {YYYYMM}-{NNNN} (as_id와 as_number 동일 사용)
func NextASNumber(db *sql.DB, at time.Time) (string, error) {
	ym := at.Format("200601")
	key := fmt.Sprintf("as:%s", ym)
	n, err := NextSeq(db, key)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s", ym, fmtSeq(n)), nil
}

// NextProcessNumber 처리번호: {접수번호}-{YYYYMM}-{NNNN}
func NextProcessNumber(db *sql.DB, asNumber string, at time.Time) (string, error) {
	ym := at.Format("200601")
	key := fmt.Sprintf("process:%s:%s", asNumber, ym)
	n, err := NextSeq(db, key)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s-%s", asNumber, ym, fmtSeq(n)), nil
}
