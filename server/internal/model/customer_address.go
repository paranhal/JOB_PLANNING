package model

import (
	"regexp"
	"strings"
)

// KoreaSidoOptions 고객 주소 시도 콤보 값(정식 표기)
var KoreaSidoOptions = []string{
	"서울특별시", "부산광역시", "대구광역시", "인천광역시", "광주광역시", "대전광역시", "울산광역시",
	"세종특별자치시",
	"경기도", "강원특별자치도", "충청북도", "충청남도", "전북특별자치도", "전라남도", "경상북도", "경상남도",
	"제주특별자치도",
}

var (
	reLegacyPostal = regexp.MustCompile(`\(\s*우?\s*(\d{5})\s*\)?`)
	reLegacyPostalLoose = regexp.MustCompile(`(?:^|[^\d])(\d{5})(?:[^\d]|$)`)
)

// SyncCombinedAddress 구조화 주소 → address(검색·표시용 한 줄) 동기화
func (c *Customer) SyncCombinedAddress() {
	parts := make([]string, 0, 3)
	for _, p := range []string{c.AddrSido, c.AddrSigungu, c.AddrDong} {
		p = strings.TrimSpace(p)
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) > 0 {
		c.Address = strings.Join(parts, " ")
	}
}

// DisplayAddress 화면 표시용 전체 주소
func (c Customer) DisplayAddress() string {
	var b strings.Builder
	pc := strings.TrimSpace(c.PostalCode)
	if pc != "" {
		b.WriteString("(")
		b.WriteString(pc)
		b.WriteString(") ")
	}
	line := strings.TrimSpace(strings.Join([]string{
		strings.TrimSpace(c.AddrSido),
		strings.TrimSpace(c.AddrSigungu),
		strings.TrimSpace(c.AddrDong),
		strings.TrimSpace(c.AddressDetail),
	}, " "))
	if line != "" {
		b.WriteString(line)
		return strings.TrimSpace(b.String())
	}
	// 구 데이터 호환
	legacy := strings.TrimSpace(c.Address + " " + c.AddressDetail)
	if pc != "" && legacy != "" {
		return "(" + pc + ") " + legacy
	}
	return legacy
}

// ParseLegacyCustomerAddress 구 address(+detail)를 우편번호·시도·군구·동·상세로 분해
func ParseLegacyCustomerAddress(raw, detail string) (postal, sido, sigungu, dong, det string) {
	det = strings.TrimSpace(detail)
	s := strings.TrimSpace(raw)
	if s == "" && det == "" {
		return
	}
	s = strings.ReplaceAll(s, "(우)", " ")
	if m := reLegacyPostal.FindStringSubmatch(s); len(m) > 1 {
		postal = m[1]
		s = reLegacyPostal.ReplaceAllString(s, " ")
	} else if m := reLegacyPostalLoose.FindStringSubmatch(s); len(m) > 1 {
		postal = m[1]
		s = strings.Replace(s, postal, " ", 1)
	}
	s = strings.ReplaceAll(s, "(", " ")
	s = strings.ReplaceAll(s, ")", " ")
	s = strings.Join(strings.Fields(s), " ")

	// 시도: 정식/약칭 표기
	for _, opt := range KoreaSidoOptions {
		if strings.HasPrefix(s, opt) {
			sido = opt
			s = strings.TrimSpace(s[len(opt):])
			break
		}
	}
	if sido == "" {
		shorts := []struct{ Key, Full string }{
			{"서울", "서울특별시"}, {"부산", "부산광역시"}, {"대구", "대구광역시"},
			{"인천", "인천광역시"}, {"광주", "광주광역시"}, {"대전", "대전광역시"},
			{"울산", "울산광역시"}, {"세종", "세종특별자치시"}, {"경기", "경기도"},
			{"강원", "강원특별자치도"}, {"충북", "충청북도"}, {"충남", "충청남도"},
			{"전북", "전북특별자치도"}, {"전남", "전라남도"}, {"경북", "경상북도"},
			{"경남", "경상남도"}, {"제주", "제주특별자치도"},
			{"충청남도", "충청남도"}, {"충청북도", "충청북도"},
			{"전라북도", "전북특별자치도"}, {"전라남도", "전라남도"},
			{"경상북도", "경상북도"}, {"경상남도", "경상남도"},
			{"강원도", "강원특별자치도"}, {"세종특별자치", "세종특별자치시"},
		}
		for _, sh := range shorts {
			if strings.HasPrefix(s, sh.Key) {
				sido = sh.Full
				s = strings.TrimSpace(s[len(sh.Key):])
				break
			}
		}
	}

	tokens := strings.Fields(s)
	i := 0
	if i < len(tokens) && looksSigungu(tokens[i]) {
		sigungu = tokens[i]
		i++
		// 구가 시 다음에 오는 경우: 성남시 분당구
		if i < len(tokens) && strings.HasSuffix(tokens[i], "구") {
			sigungu = sigungu + " " + tokens[i]
			i++
		}
	}
	if i < len(tokens) && looksDong(tokens[i]) {
		dong = tokens[i]
		i++
	}
	rest := strings.Join(tokens[i:], " ")
	if det == "" {
		det = rest
	} else if rest != "" {
		det = strings.TrimSpace(rest + " " + det)
	}
	return
}

func looksSigungu(tok string) bool {
	return strings.HasSuffix(tok, "시") || strings.HasSuffix(tok, "군") || strings.HasSuffix(tok, "구")
}

func looksDong(tok string) bool {
	return strings.HasSuffix(tok, "동") || strings.HasSuffix(tok, "읍") ||
		strings.HasSuffix(tok, "면") || strings.HasSuffix(tok, "리")
}
