package model

import (
	"fmt"
	"strings"
	"time"
)

const (
	ActionSetStateDraft = "draft"
	ActionSetStateSaved = "saved"
	ActionSetStateFinal = "final"
	MaxASActionSets     = 20
)

// ASActionSet 조치 1·2·3… 한 세트. 95-85.
type ASActionSet struct {
	SetID             string
	ASID              string
	Seq               int
	State             string
	AssignedTo        string
	AssignedUserID    string
	WorkPlace         string
	ProcessType       string
	ProcessTypeReason string
	VisitDate         string
	CauseCat1         string
	CauseCat1Custom   string
	CauseCat2         string
	CauseCat3         string
	CauseDetail       string
	CompleteDatetime  string
	WorkDate          string
	StartTime         string
	DurationMin       int
	EndTime           string
	LeaveSame         bool
	LeaveTime         string
	ActionTaken       string
	ActionNA          bool
	ActionNAReason    string
	ResultCode        string
	ResultExtra       string
	CreatedBy         string
	CreatedAt         string
	UpdatedAt         string
	PhotoN            int
	Open              bool
	Virtual           bool // 세트 행 없음. 접수 본문으로 만든 가상 조치 1
	OverlapWarn       string
}

func IsASActionResultAllowed(code string) bool {
	return IsSelectableActionResult(code)
}

func (s ASActionSet) ResultLabel() string {
	return ActionResultLabel(s.ResultCode)
}

func (s ASActionSet) SummaryLine() string {
	parts := []string{}
	if s.ResultCode != "" {
		parts = append(parts, s.ResultLabel())
	} else {
		parts = append(parts, "결과 미선택")
	}
	if n := strings.TrimSpace(s.AssignedTo); n != "" {
		parts = append(parts, n)
	}
	if t := strings.TrimSpace(s.ActionTaken); t != "" {
		r := []rune(t)
		if len(r) > 28 {
			t = string(r[:28]) + "…"
		}
		parts = append(parts, t)
	}
	if tw := s.TimeSummary(); tw != "" {
		parts = append(parts, tw)
	}
	return strings.Join(parts, " · ")
}

func (s ASActionSet) TimeSummary() string {
	st := ClockHHMMStrict(s.StartTime)
	en := ClockHHMMStrict(s.EndTime)
	if st == "" && en == "" {
		return ""
	}
	left, right := "—", "—"
	if st != "" {
		left = FormatKoreanTime(st)
	}
	if en != "" {
		right = FormatKoreanTime(en)
	}
	out := left + "~" + right
	lv := ClockHHMMStrict(s.LeaveTime)
	if !s.LeaveSame && lv != "" && lv != en {
		out += " (철수 " + FormatKoreanTime(lv) + ")"
	}
	return out
}

func (s ASActionSet) LeaveDisplay() string {
	lv := ClockHHMMStrict(s.LeaveTime)
	if lv == "" {
		return ""
	}
	return "철수 " + FormatKoreanTime(lv)
}

func (s ASActionSet) SavedTag() string {
	switch s.State {
	case ActionSetStateSaved, ActionSetStateFinal:
		return "임시 저장됨"
	case ActionSetStateDraft:
		if s.Virtual {
			return "작성 중"
		}
		return "임시 저장됨"
	default:
		if s.Virtual {
			return "작성 중"
		}
		return "작성 중"
	}
}

func (s ASActionSet) Prefix() string {
	if s.Seq < 1 {
		return "s1_"
	}
	return fmt.Sprintf("s%d_", s.Seq)
}

// CopyActionSetOntoReceipt 마지막 조치 값을 접수 본문에 넣는다. 95-85 §2-3.
func CopyActionSetOntoReceipt(as *ASReceipt, s ASActionSet) {
	if as == nil {
		return
	}
	if strings.TrimSpace(s.AssignedTo) != "" || strings.TrimSpace(s.AssignedUserID) != "" {
		as.AssignedTo = s.AssignedTo
		as.AssignedUserID = s.AssignedUserID
	}
	if wp := NormalizeWorkPlace(s.WorkPlace); wp != "" {
		as.WorkPlace = wp
	}
	as.ProcessType = strings.TrimSpace(s.ProcessType)
	as.ProcessTypeReason = strings.TrimSpace(s.ProcessTypeReason)
	as.VisitDate = strings.TrimSpace(s.VisitDate)
	as.CauseCat1 = strings.TrimSpace(s.CauseCat1)
	as.CauseCat1Custom = strings.TrimSpace(s.CauseCat1Custom)
	as.CauseCat2 = strings.TrimSpace(s.CauseCat2)
	as.CauseCat3 = strings.TrimSpace(s.CauseCat3)
	as.CauseDetail = strings.TrimSpace(s.CauseDetail)
	as.ActionTaken = strings.TrimSpace(s.ActionTaken)
	as.ResultCode = strings.TrimSpace(s.ResultCode)
	syncReceiptTimesFromSet(as, s)
}

func syncReceiptTimesFromSet(as *ASReceipt, s ASActionSet) {
	day := strings.TrimSpace(s.WorkDate)
	st := ClockHHMMStrict(s.StartTime)
	en := ClockHHMMStrict(s.EndTime)
	if day == "" {
		return
	}
	if st != "" {
		if t, ok := parseLocalDateTime(day + "T" + st); ok {
			as.StartDatetime = &t
		}
	}
	if en != "" {
		if t, ok := parseLocalDateTime(day + "T" + en); ok {
			as.CompleteDatetime = &t
		}
	} else if strings.TrimSpace(s.CompleteDatetime) != "" {
		if t, ok := parseLocalDateTime(s.CompleteDatetime); ok {
			as.CompleteDatetime = &t
		}
	}
}

func parseLocalDateTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " ", "T")
	layouts := []string{"2006-01-02T15:04", "2006-01-02T15:04:05", "2006-01-02 15:04", "2006-01-02 15:04:05"}
	for _, l := range layouts {
		if t, err := time.ParseInLocation(l, s, KST()); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// VirtualActionSetFromReceipt 세트 없는 기존 접수. 완료일시만 채우고 시작·작업 시간은 비움. 95-85 §2-1A.7.
func VirtualActionSetFromReceipt(as *ASReceipt) ASActionSet {
	s := ASActionSet{Seq: 1, State: ActionSetStateDraft, LeaveSame: true, Virtual: true, Open: true}
	if as == nil {
		return s
	}
	s.ASID = as.ASID
	s.AssignedTo = as.AssignedTo
	s.AssignedUserID = as.AssignedUserID
	s.WorkPlace = as.WorkPlace
	s.ProcessType = as.ProcessType
	s.ProcessTypeReason = as.ProcessTypeReason
	s.VisitDate = as.VisitDate
	s.CauseCat1 = as.CauseCat1
	s.CauseCat1Custom = as.CauseCat1Custom
	s.CauseCat2 = as.CauseCat2
	s.CauseCat3 = as.CauseCat3
	s.CauseDetail = as.CauseDetail
	s.ActionTaken = as.ActionTaken
	s.ResultCode = as.ResultCode
	if as.CompleteDatetime != nil && !as.CompleteDatetime.IsZero() {
		t := as.CompleteDatetime.In(KST())
		s.WorkDate = t.Format("2006-01-02")
		s.EndTime = t.Format("15:04")
		s.CompleteDatetime = t.Format("2006-01-02T15:04")
		s.LeaveTime = s.EndTime
	} else if strings.TrimSpace(as.VisitDate) != "" {
		s.WorkDate = as.VisitDate
	}
	return s
}

// ValidateActionSetWorkTime 작업 시간 서버 재검증. 시작·작업 시간은 선택. 95-85 §2-1A.
func ValidateActionSetWorkTime(s ASActionSet) string {
	st := ClockHHMMStrict(s.StartTime)
	dur := s.DurationMin
	hasStart := st != ""
	hasDur := dur > 0
	if hasStart != hasDur {
		return "work_time_pair"
	}
	if !hasStart {
		if !s.LeaveSame {
			lv := ClockHHMMStrict(s.LeaveTime)
			if NormalizeWorkPlace(s.WorkPlace) == WorkPlaceField && lv == "" {
				return "leave_time_required"
			}
		}
		return ""
	}
	if !IsTenMinuteClock(st) {
		return "start_time_step"
	}
	if dur < 10 || dur > 12*60 || dur%10 != 0 {
		return "duration_step"
	}
	end, ok := AddMinutesClock(st, dur)
	if !ok {
		return "work_time_midnight"
	}
	if ClockHHMMStrict(s.EndTime) != "" && ClockHHMMStrict(s.EndTime) != end {
		s.EndTime = end
	}
	lv := ClockHHMMStrict(s.LeaveTime)
	if s.LeaveSame {
		return ""
	}
	if lv == "" {
		if NormalizeWorkPlace(s.WorkPlace) == WorkPlaceField {
			return "leave_time_required"
		}
		return ""
	}
	if !IsTenMinuteClock(lv) {
		return "leave_time_step"
	}
	if !ClockAfterOrEqual(lv, end) {
		return "leave_before_end"
	}
	return ""
}

func FillActionSetComputedTimes(s *ASActionSet) {
	if s == nil {
		return
	}
	st := ClockHHMMStrict(s.StartTime)
	if st == "" || s.DurationMin <= 0 {
		return
	}
	if end, ok := AddMinutesClock(st, s.DurationMin); ok {
		s.EndTime = end
		if s.LeaveSame || ClockHHMMStrict(s.LeaveTime) == "" {
			s.LeaveTime = end
		}
	}
}

func ActionSetOverlapWarning(sets []ASActionSet) []ASActionSet {
	for i := range sets {
		sets[i].OverlapWarn = ""
	}
	for i := 0; i < len(sets); i++ {
		a := sets[i]
		if ClockHHMMStrict(a.StartTime) == "" || ClockHHMMStrict(a.EndTime) == "" || strings.TrimSpace(a.WorkDate) == "" {
			continue
		}
		for j := 0; j < i; j++ {
			b := sets[j]
			if strings.TrimSpace(a.WorkDate) != strings.TrimSpace(b.WorkDate) {
				continue
			}
			if clocksOverlap(a.StartTime, a.EndTime, b.StartTime, b.EndTime) {
				sets[i].OverlapWarn = fmt.Sprintf("조치 %d과 시간이 겹칩니다", b.Seq)
				break
			}
		}
	}
	return sets
}

func clocksOverlap(a0, a1, b0, b1 string) bool {
	aStart, aEnd := clockMin(a0), clockMin(a1)
	bStart, bEnd := clockMin(b0), clockMin(b1)
	if aEnd <= aStart || bEnd <= bStart {
		return false
	}
	return aStart < bEnd && bStart < aEnd
}

func clockMin(s string) int {
	h, m, ok := parseClockHHMM(s)
	if !ok {
		return 0
	}
	return h*60 + m
}

// ApplyVisitDateFromWork 현장방문이면 작업일이 방문일. 작업일이 비면 옛 visit_date 폴백.
func ApplyVisitDateFromWork(s *ASActionSet, postedVisit string) {
	if s == nil {
		return
	}
	if strings.TrimSpace(s.ProcessType) != ProcessTypeVisit {
		s.VisitDate = ""
		return
	}
	if strings.TrimSpace(s.WorkDate) == "" {
		s.WorkDate = strings.TrimSpace(postedVisit)
	}
	s.VisitDate = strings.TrimSpace(s.WorkDate)
}

func LastCommittedActionSet(sets []ASActionSet) *ASActionSet {
	for i := len(sets) - 1; i >= 0; i-- {
		if strings.TrimSpace(sets[i].ResultCode) == "" {
			continue
		}
		if sets[i].State == ActionSetStateDraft {
			continue
		}
		return &sets[i]
	}
	return nil
}

func ActionSetMissingFields(s ASActionSet) []string {
	var out []string
	if strings.TrimSpace(s.ResultCode) == "" {
		out = append(out, "result_code")
	}
	if strings.TrimSpace(s.WorkPlace) == "" {
		out = append(out, "work_place")
	}
	if strings.TrimSpace(s.ProcessType) == "" {
		out = append(out, "process_type")
	}
	if strings.TrimSpace(s.ProcessType) == ProcessTypeVisit && strings.TrimSpace(s.WorkDate) == "" {
		out = append(out, "work_date")
	}
	if strings.TrimSpace(s.CauseCat1) == "" {
		out = append(out, "cause_cat1")
	}
	if strings.TrimSpace(s.ActionTaken) == "" && !s.ActionNA {
		out = append(out, "action_taken")
	}
	return out
}

func MissingFieldLabel(code string) string {
	switch strings.TrimSpace(code) {
	case "result_code":
		return "결과"
	case "work_place":
		return "근무구분"
	case "process_type":
		return "처리유형"
	case "work_date", "work_date_required", "visit_date":
		return "작업일"
	case "cause_cat1":
		return "원인분류"
	case "action_taken":
		return "조치내용"
	default:
		return code
	}
}

func JoinActionSetsForReport(sets []ASActionSet) string {
	if len(sets) == 0 {
		return ""
	}
	if len(sets) == 1 {
		return strings.TrimSpace(sets[0].ActionTaken)
	}
	var b strings.Builder
	for i, s := range sets {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "[조치 %d] %s", s.Seq, strings.TrimSpace(s.ActionTaken))
		if lv := s.LeaveDisplay(); lv != "" {
			fmt.Fprintf(&b, " (%s)", lv)
		}
	}
	return b.String()
}
