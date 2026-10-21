package model

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	ASPartsMaxRows     = 30
	ASPartNameMaxRunes = 100
	ASPartCodeMaxRunes = 50
)

// ASPartUsed 조치 화면 구조화 부품 한 줄. 단가·금액 없음.
type ASPartUsed struct {
	PartID     string
	ASID       string
	ProcessID  string
	ItemID     string
	PartName   string
	PartCode   string
	Qty        int
	SortOrder  int
	CreatedAt  string
	UpdatedAt  string
	RowError   string
}

func FormatPartsUsedSummary(rows []ASPartUsed) string {
	parts := make([]string, 0, len(rows))
	for _, r := range rows {
		name := strings.TrimSpace(r.PartName)
		if name == "" {
			continue
		}
		code := strings.TrimSpace(r.PartCode)
		qty := r.Qty
		if qty < 1 {
			qty = 1
		}
		if code != "" {
			parts = append(parts, fmt.Sprintf("%s(%s) ×%d", name, code, qty))
		} else {
			parts = append(parts, fmt.Sprintf("%s ×%d", name, qty))
		}
	}
	return strings.Join(parts, ", ")
}

func ParsePartQty(raw string) (int, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 1, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("수량은 1 이상 정수여야 합니다")
	}
	return n, nil
}

func ValidateASPartRow(name, code string, qty int) string {
	name = strings.TrimSpace(name)
	code = strings.TrimSpace(code)
	if name == "" {
		return "품명을 입력해 주세요"
	}
	if utf8.RuneCountInString(name) > ASPartNameMaxRunes {
		return "품명은 100자 이내여야 합니다"
	}
	if utf8.RuneCountInString(code) > ASPartCodeMaxRunes {
		return "코드는 50자 이내여야 합니다"
	}
	if qty < 1 {
		return "수량은 1 이상 정수여야 합니다"
	}
	return ""
}

func ParseASPartsFromForm(names, codes, qtys, itemIDs []string) ([]ASPartUsed, error) {
	n := len(names)
	if n > ASPartsMaxRows {
		return nil, fmt.Errorf("자재는 최대 30행입니다")
	}
	pad := func(s []string) []string {
		if len(s) >= n {
			return s
		}
		out := make([]string, n)
		copy(out, s)
		return out
	}
	codes, qtys, itemIDs = pad(codes), pad(qtys), pad(itemIDs)
	var out []ASPartUsed
	for i := 0; i < n; i++ {
		name := strings.TrimSpace(names[i])
		code := strings.TrimSpace(codes[i])
		itemID := strings.TrimSpace(itemIDs[i])
		qtyRaw := strings.TrimSpace(qtys[i])
		if name == "" && code == "" && itemID == "" && (qtyRaw == "" || qtyRaw == "1") {
			continue
		}
		qty, err := ParsePartQty(qtyRaw)
		row := ASPartUsed{ItemID: itemID, PartName: name, PartCode: code, Qty: qty, SortOrder: len(out)}
		if err != nil {
			row.RowError = err.Error()
			return append(out, row), err
		}
		if msg := ValidateASPartRow(name, code, qty); msg != "" {
			row.RowError = msg
			return append(out, row), fmt.Errorf("%s", msg)
		}
		out = append(out, row)
	}
	return out, nil
}

func ItemPartCode(item SalesItem) string {
	if s := strings.TrimSpace(item.GovItemNo); s != "" {
		return s
	}
	return strings.TrimSpace(item.Model)
}
