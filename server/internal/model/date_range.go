package model

import (
	"fmt"
	"strings"
	"time"
)

// 날짜 연도 허용 범위 (§8.2.3). 0206-08-12 같은 오타가 예정일 경과·워킹데이를 오염시키지 않게 막는다.
const (
	AppDateMinYear = 2000
	AppDateMaxYear = 2100
	AppDateMin     = "2000-01-01"
	AppDateMax     = "2100-12-31"
	AppDateTimeMin = "2000-01-01T00:00"
	AppDateTimeMax = "2100-12-31T23:59"
)

var ErrAppDateYear = fmt.Errorf("날짜 연도는 2000~2100 사이여야 합니다")

func AppDateYearOK(t time.Time) bool {
	if t.IsZero() {
		return true
	}
	y := t.Year()
	return y >= AppDateMinYear && y <= AppDateMaxYear
}

// NormalizeAppDate YYYY-MM-DD. 빈 값은 빈 문자열. 형식이 아니거나 연도가 범위 밖이면 빈 문자열.
func NormalizeAppDate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil || !AppDateYearOK(t) {
		return ""
	}
	return t.Format("2006-01-02")
}

// ParseAppDate 빈 값은 ("", nil). 값이 있으면 정규화하거나 ErrAppDateYear / 형식 오류.
func ParseAppDate(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return "", fmt.Errorf("날짜 형식이 아닙니다")
	}
	if !AppDateYearOK(t) {
		return "", ErrAppDateYear
	}
	return t.Format("2006-01-02"), nil
}

// RequireAppDateYear 비어 있지 않은 날짜 문자열이 연도 범위 안인지. 저장 경로에서 쓴다.
func RequireAppDateYear(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if NormalizeAppDate(s) != "" {
		return nil
	}
	// YYYY-MM-DD HH:MM[:SS] 도 허용 (receipt_datetime · complete_datetime)
	for _, f := range []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
	} {
		if t, err := time.Parse(f, s); err == nil {
			if !AppDateYearOK(t) {
				return ErrAppDateYear
			}
			return nil
		}
	}
	if len(s) >= 10 {
		if t, err := time.Parse("2006-01-02", s[:10]); err == nil {
			if !AppDateYearOK(t) {
				return ErrAppDateYear
			}
			return nil
		}
	}
	return ErrAppDateYear
}
