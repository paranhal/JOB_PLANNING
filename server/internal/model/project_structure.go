package model

import (
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	TrackModeSite   = "site"
	TrackModePhase  = "phase"
	BasisSite       = "site"
	BasisQty        = "qty"
	PkgAuto         = "auto"
	PkgDone         = "done"
	PkgHold         = "hold"
	PaceBefore      = "판정 전"
	PaceOK          = "정상"
	PaceWarn        = "주의"
	PaceRisk        = "지연 위험"
	PaceOver        = "기한 지남"
	StatusCandidate = "candidate"
)

const siteChoSung = "ㄱㄲㄴㄷㄸㄹㅁㅂㅃㅅㅆㅇㅈㅉㅊㅋㅌㅍㅎ"

// FormatIntComma 천 단위 쉼표.  labor_upload 의 사적 함수를 공용으로 올렸다.
func FormatIntComma(n int) string {
	if n < 0 {
		return "-" + FormatIntComma(-n)
	}
	s := fmt.Sprintf("%d", n)
	var out []byte
	for i := 0; i < len(s); i++ {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, s[i])
	}
	return string(out)
}

func PlanNameOK(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("계획명을 입력하세요")
	}
	if utf8.RuneCountInString(name) > 100 {
		return "", fmt.Errorf("계획명은 100자까지입니다")
	}
	return name, nil
}

func PackageNameOK(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("하부 업무명을 입력하세요")
	}
	if utf8.RuneCountInString(name) > 100 {
		return "", fmt.Errorf("하부 업무명은 100자까지입니다")
	}
	return name, nil
}

func NormalizePlanMergeName(name string) string {
	return NormalizePhaseMergeName(name)
}

func ValidTrackMode(v string) bool {
	v = strings.TrimSpace(v)
	return v == TrackModeSite || v == TrackModePhase
}

func ValidBasis(v string) bool {
	v = strings.TrimSpace(v)
	return v == "" || v == BasisSite || v == BasisQty
}

// QtyCredit 수량 인식. 완료는 목표 전체, 진행은 done_qty(0..목표), 예정·제외는 0.
func QtyCredit(status string, target, done int) int {
	switch strings.TrimSpace(status) {
	case SiteDone:
		if target < 0 {
			return 0
		}
		return target
	case SiteInProgress, PkgHold:
		if done < 0 {
			done = 0
		}
		if target > 0 && done > target {
			done = target
		}
		if target == 0 && done > 0 {
			return 0
		}
		return done
	default:
		return 0
	}
}

// EffectiveStatus 저장값과 활동으로 화면 상태. allDone 이고 저장이 auto 이면 완료 후보.
func EffectiveStatus(stored string, hasActivity, allDone bool) string {
	switch strings.TrimSpace(stored) {
	case PhaseDone:
		return PhaseDone
	case PkgHold:
		return PkgHold
	}
	if allDone && (stored == "" || stored == PkgAuto || stored == PhasePlanned) {
		return StatusCandidate
	}
	if hasActivity {
		return PhaseActive
	}
	if stored == PhaseActive {
		return PhaseActive
	}
	return PhasePlanned
}

// SiteImportMatch 95-169 pwSiteImportMatch 와 같은 규칙. regionFilter 가 있으면 지역은 정확히 같아야 한다.
func SiteImportMatch(name, parent, region, q, regionFilter string) bool {
	want := strings.TrimSpace(regionFilter)
	if want != "" && strings.TrimSpace(region) != want {
		return false
	}
	blob := siteNorm(name + parent + region)
	q = siteNorm(q)
	if q == "" {
		return true
	}
	kind := siteQueryKind(q)
	switch kind {
	case "cho":
		return strings.Contains(siteChoOfString(blob), q)
	case "mix":
		return siteMixMatch(blob, q)
	default:
		return strings.Contains(blob, q)
	}
}

// RegionAuxMatch 지역은 보조 필터. 비어 있으면 통과, 아니면 포함(시·군 일부만 적어도 맞음).
func RegionAuxMatch(region, filter string) bool {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return true
	}
	return strings.Contains(siteNorm(region), siteNorm(filter))
}

func siteNorm(s string) string {
	s = strings.ToLower(s)
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
}

func siteIsCho(r rune) bool {
	return strings.ContainsRune(siteChoSung, r)
}

func siteChoRune(r rune) rune {
	if r < 0xAC00 || r > 0xD7A3 {
		return 0
	}
	idx := int((r - 0xAC00) / 588)
	ch := []rune(siteChoSung)
	if idx < 0 || idx >= len(ch) {
		return 0
	}
	return ch[idx]
}

func siteChoOfString(s string) string {
	var b strings.Builder
	for _, r := range s {
		if ch := siteChoRune(r); ch != 0 {
			b.WriteRune(ch)
		}
	}
	return b.String()
}

func siteQueryKind(q string) string {
	all, any := true, false
	if q == "" {
		return "text"
	}
	for _, r := range q {
		if siteIsCho(r) {
			any = true
		} else {
			all = false
		}
	}
	if all {
		return "cho"
	}
	if any {
		return "mix"
	}
	return "text"
}

func siteMixMatch(hay, q string) bool {
	hr, qr := []rune(hay), []rune(q)
	if len(qr) > len(hr) {
		return false
	}
	last := len(hr) - len(qr)
	for i := 0; i <= last; i++ {
		ok := true
		for j, qc := range qr {
			hc := hr[i+j]
			if siteIsCho(qc) {
				ch := siteChoRune(hc)
				if ch != qc && hc != qc {
					ok = false
					break
				}
			} else if hc != qc {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// PlanPace 최근 영업일 실적이 PaceMinDays 미만이거나 종료일이 없으면 판정 전.
// biz 는 휴일 달력의 영업일(개인 휴가는 보지 않음).
func PlanPace(planEnd, today string, done, total, recentDays int, recentDone float64, biz func(string) bool) string {
	planEnd = strings.TrimSpace(planEnd)
	today = strings.TrimSpace(today)
	if planEnd == "" || recentDays < 3 || biz == nil {
		return PaceBefore
	}
	if total > 0 && done >= total {
		return PaceOK
	}
	if today > planEnd {
		return PaceOver
	}
	remain := total - done
	if remain < 0 {
		remain = 0
	}
	if recentDone <= 0 {
		if remain == 0 {
			return PaceOK
		}
		return PaceRisk
	}
	per := recentDone / float64(recentDays)
	if per <= 0 {
		return PaceRisk
	}
	need := float64(remain) / per
	left := bizDaysUntil(today, planEnd, biz)
	if need <= 0 {
		return PaceOK
	}
	slack := float64(left) / need
	switch {
	case slack >= 1:
		return PaceOK
	case slack >= 0.8:
		return PaceWarn
	default:
		return PaceRisk
	}
}

func bizDaysUntil(today, end string, biz func(string) bool) int {
	t, err1 := time.Parse("2006-01-02", today)
	e, err2 := time.Parse("2006-01-02", end)
	if err1 != nil || err2 != nil || e.Before(t) {
		return 0
	}
	n := 0
	for d := t.AddDate(0, 0, 1); !d.After(e); d = d.AddDate(0, 0, 1) {
		if biz(d.Format("2006-01-02")) {
			n++
		}
	}
	return n
}

// WeekendBiz 주말만 빼고 휴일은 모르는 영업일. 달력이 없을 때의 보조.
func WeekendBiz(date string) bool {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(date))
	if err != nil {
		return false
	}
	w := t.Weekday()
	return w != time.Saturday && w != time.Sunday
}

func BothRateText(sitesDone, sitesTotal, qtyDone, qtyTotal int, unit string) string {
	site := "—"
	if sitesTotal > 0 {
		site = fmt.Sprintf("%d/%d곳 (%s)", sitesDone, sitesTotal, SiteCheckRateLabel(sitesDone, sitesTotal))
	}
	if strings.TrimSpace(unit) == "" {
		unit = "식"
	}
	qty := "—"
	if qtyTotal > 0 {
		qty = fmt.Sprintf("수량 %s/%s%s (%s)", FormatIntComma(qtyDone), FormatIntComma(qtyTotal), unit, SiteCheckRateLabel(qtyDone, qtyTotal))
	} else {
		qty = "수량 —"
	}
	return site + " · " + qty
}
