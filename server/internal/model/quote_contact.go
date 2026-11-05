package model

import (
	"fmt"
	"strings"
	"unicode"
)

const SalesCreatedViaQuickQuote = "quick_quote"

func CollapseNewlines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.Join(strings.Fields(s), " ")
}

func FormatContactLine(mobile, phone, email string) string {
	tel := strings.TrimSpace(mobile)
	if tel == "" {
		tel = strings.TrimSpace(phone)
	}
	email = strings.TrimSpace(email)
	switch {
	case tel != "" && email != "":
		return tel + " / " + email
	case tel != "":
		return tel
	default:
		return email
	}
}

type ParsedContactLine struct {
	Phone string
	Email string
}

func ParseContactLine(s string) ParsedContactLine {
	s = CollapseNewlines(s)
	if s == "" {
		return ParsedContactLine{}
	}
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '/' || r == ',' || unicode.IsSpace(r)
	})
	var phone, email string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.Contains(p, "@") {
			if email == "" {
				email = p
			}
			continue
		}
		if phone == "" {
			phone = p
		}
	}
	return ParsedContactLine{Phone: phone, Email: email}
}

func ContactPhoneDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func NormalizeEmailCmp(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func ValidateContactLine(s string) error {
	p := ParseContactLine(s)
	if p.Phone == "" && p.Email == "" {
		return fmt.Errorf("연락처(전화 또는 이메일)를 입력하세요")
	}
	if p.Phone != "" && len(ContactPhoneDigits(p.Phone)) < 9 {
		return fmt.Errorf("전화는 숫자 9자리 이상이어야 합니다")
	}
	if p.Email != "" && !strings.Contains(p.Email, "@") {
		return fmt.Errorf("이메일 형식이 아닙니다")
	}
	if p.Email != "" {
		at := strings.Index(p.Email, "@")
		if at <= 0 || at == len(p.Email)-1 || !strings.Contains(p.Email[at+1:], ".") {
			return fmt.Errorf("이메일 형식이 아닙니다")
		}
	}
	return nil
}

func IsMobilePhone(s string) bool {
	d := ContactPhoneDigits(s)
	return strings.HasPrefix(d, "01")
}

func ContactLineChanged(mobile, phone, email, line string) bool {
	parsed := ParseContactLine(line)
	oldTel := ContactPhoneDigits(mobile)
	if oldTel == "" {
		oldTel = ContactPhoneDigits(phone)
	}
	newTel := ContactPhoneDigits(parsed.Phone)
	return oldTel != newTel || NormalizeEmailCmp(email) != NormalizeEmailCmp(parsed.Email)
}

func OwnerPhoneFromUser(u *User) string {
	if u == nil {
		return ""
	}
	if s := strings.TrimSpace(u.Mobile); s != "" {
		return s
	}
	return strings.TrimSpace(u.Tel)
}

func AutoQuoteTitle(lines []SalesQuoteLine) string {
	var names []string
	for _, ln := range lines {
		n := CollapseNewlines(ln.Name)
		if n == "" {
			continue
		}
		names = append(names, n)
	}
	if len(names) == 0 {
		return ""
	}
	if len(names) == 1 {
		return names[0]
	}
	return fmt.Sprintf("%s 외 %d건", names[0], len(names)-1)
}
