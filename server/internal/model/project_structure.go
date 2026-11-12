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
	PaceDone        = "완료"
	StatusCandidate = "candidate"
	// 페이스 임계값. 실적이 있는 영업일이 이보다 적으면 판정 전.
	PaceMinActualDays = 3
	// 최근 평균의 기본 창. 계획이 더 짧으면 첫 실적일부터 센다.
	PaceLookbackBiz = 7
	// 여유율 = 필요한 하루 평균 ÷ 평균 처리량. 낮을수록 여유.
	PaceSlackOK    = 0.8
	PaceSlackLimit = 1.0
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

// PlanPace 는 AssessPace 의 판정 이름만 돌려준다.
// recentDays 는 실적이 있는 영업일 수, recentDone 은 그 구간의 처리량이다.
func PlanPace(planEnd, today string, done, total, recentDays int, recentDone float64, biz func(string) bool) string {
	window := recentDays
	if window < 1 {
		window = 1
	}
	return AssessPace(PaceFacts{
		PlanEnd: planEnd, Today: today, Done: done, Total: total,
		ActualDays: recentDays, WindowDays: window, Amount: recentDone, Biz: biz,
	}).Label
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

// BadgeMaxRunes 하부 업무 배지 최대 글자 수. 95-179.
const BadgeMaxRunes = 8

// BadgeOK 배지 이름을 다듬는다. 비어 있으면 「배지 없음」이다(표시하지 않는다). 같은 이름을 여러 하부 업무가 가져도 된다.
func BadgeOK(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	if strings.ContainsAny(s, "\r\n\t<>") {
		return "", fmt.Errorf("배지에는 줄바꿈과 < > 를 쓸 수 없습니다")
	}
	if utf8.RuneCountInString(s) > BadgeMaxRunes {
		return "", fmt.Errorf("배지는 %d자까지입니다", BadgeMaxRunes)
	}
	return s, nil
}

// 마감일(plan_end) 변경 종류. 95-179: 연장(뒤로 미룸)만 사유가 필수다.
const (
	EndChangeNone    = ""
	EndChangeExtend  = "extend"
	EndChangeShorten = "shorten"
	EndChangeClear   = "clear"
	EndChangeSet     = "set" // 처음 정함. 이력에 남기지 않는다
)

// EndChangeKind 마감일 변경 종류를 가른다. 날짜는 YYYY-MM-DD(문자열 비교가 날짜 순서와 같다).
func EndChangeKind(oldEnd, newEnd string) string {
	oldEnd, newEnd = strings.TrimSpace(oldEnd), strings.TrimSpace(newEnd)
	switch {
	case oldEnd == newEnd:
		return EndChangeNone
	case oldEnd == "" && newEnd != "":
		return EndChangeSet
	case oldEnd != "" && newEnd == "":
		return EndChangeClear
	case newEnd > oldEnd:
		return EndChangeExtend
	default:
		return EndChangeShorten
	}
}

// ExtendReasonOK 연장 사유(200자 이내).
func ExtendReasonOK(s string) (string, error) {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > 200 {
		return "", fmt.Errorf("연장 사유는 200자까지입니다")
	}
	return s, nil
}

// BadgeTone 배지 색. 이름이 같으면 어디서나 같은 색이 나오고, 이름이 다르면 대체로 다른 색이 나온다(고정 팔레트 4색, 이미
// 빌드된 badge-* 클래스만 쓴다. badge-danger 는 오류용이라 쓰지 않는다). 95-179 색 제안.
func BadgeTone(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return "badge-neutral"
	}
	var h uint32 = 2166136261
	for _, r := range label {
		h ^= uint32(r)
		h *= 16777619
	}
	return [...]string{"badge-info", "badge-success", "badge-warning", "badge-neutral"}[h%4]
}
