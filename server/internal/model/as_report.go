package model

import (
	"strings"
	"time"
	"unicode"
)

// ASReportIssuerPrefix attachments.keywords 에 발급자를 남긴다. 새 컬럼을 만들지 않는다.
const ASReportIssuerPrefix = "발급자:"

// ASReportDraft 조치완료보고서 미리보기·치환 값. as_receipts 에 되쓰지 않는다. §12.10.3 · §12.10.5
type ASReportDraft struct {
	CustomerName string // {{고객명}}
	Department   string // {{부서}}
	Manager      string // {{담당자}}
	Phone        string // {{연락처}}
	Service      string // {{서비스}}
	Symptom      string // {{장애사항}}
	CauseDetail  string // {{장애원인}} — cause_type 코드가 아님
	Conclusion   string // {{결론}}
	ReportDate   string // {{보고일자}}
	Inspector    string // {{점검자}}
	Confirmer    string // {{확인자}}
	WorkDates    string // {{작업일자}} 날짜별 여러 줄
	Actions      string // {{조치내용}} 날짜별 여러 줄
}

// Values 자리표시자 → 값. 빈 칸도 키를 넣어 {{ 가 남지 않게 한다. §12.10.6
func (d ASReportDraft) Values() map[string]string {
	return map[string]string{
		"고객명":  d.CustomerName,
		"부서":   d.Department,
		"담당자":  d.Manager,
		"연락처":  d.Phone,
		"서비스":  d.Service,
		"장애사항": d.Symptom,
		"장애원인": d.CauseDetail,
		"결론":   d.Conclusion,
		"보고일자": d.ReportDate,
		"점검자":  d.Inspector,
		"확인자":  d.Confirmer,
		"작업일자": d.WorkDates,
		"조치내용": d.Actions,
	}
}

// MissingReportFields 발급에 필요한 서술. 비면 무엇이 없는지 보여 준다. §12.10.5
func (d ASReportDraft) MissingReportFields() []string {
	var miss []string
	if strings.TrimSpace(d.CauseDetail) == "" {
		miss = append(miss, "장애원인")
	}
	if strings.TrimSpace(d.Conclusion) == "" {
		miss = append(miss, "결론")
	}
	return miss
}

// Filename {고객명}_{증상요약 20자}_조치완료보고서_{YYYYMMDD}.hwpx §12.10.7
func (d ASReportDraft) Filename(issued time.Time) string {
	if issued.IsZero() {
		issued = time.Now()
	}
	org := SanitizeReportFilename(strings.TrimSpace(d.CustomerName))
	if org == "" {
		org = "고객"
	}
	sym := SanitizeReportFilename(clipRunes(strings.TrimSpace(d.Symptom), 20))
	if sym == "" {
		sym = "증상없음"
	}
	return org + "_" + sym + "_조치완료보고서_" + issued.Format("20060102") + ".hwpx"
}

// BuildASReportDraft §12.10.3 필드 매핑. 조치 여러 건은 한 셀에 날짜별 여러 줄. §12.10.2
func BuildASReportDraft(as *ASReceipt, processes []ASProcess, customer *Customer, asset *Asset, contacts []Contact, now time.Time) ASReportDraft {
	d := ASReportDraft{}
	if now.IsZero() {
		now = time.Now()
	}
	d.ReportDate = now.Format("2006-01-02")
	if as == nil {
		return d
	}

	d.CustomerName = strings.TrimSpace(as.OrgName)
	if d.CustomerName == "" && customer != nil {
		d.CustomerName = strings.TrimSpace(customer.OrgName)
	}
	d.Manager = firstNonEmpty(as.Requester, as.RequesterName)
	d.Phone = strings.TrimSpace(as.ConfirmContact)
	d.Service = strings.TrimSpace(as.ProductName)
	if d.Service == "" && asset != nil {
		d.Service = strings.TrimSpace(asset.ProductName)
	}
	d.Symptom = as.Symptom
	d.CauseDetail = as.CauseDetail
	d.Conclusion = as.Conclusion
	d.Inspector = strings.TrimSpace(as.AssignedTo)
	d.Confirmer = strings.TrimSpace(as.CustomerConfirmer)
	if d.Confirmer == "" {
		d.Confirmer = strings.TrimSpace(as.ConfirmTarget)
	}

	ct := matchReportContact(contacts, d.Manager, d.Confirmer)
	if ct != nil {
		d.Department = firstNonEmpty(ct.JobRole, ct.Title)
		if d.Phone == "" {
			d.Phone = firstNonEmpty(ct.Phone, ct.Mobile)
		}
		if d.Manager == "" {
			d.Manager = strings.TrimSpace(ct.FullName)
		}
	}
	if d.Phone == "" && customer != nil {
		d.Phone = strings.TrimSpace(customer.MainPhone)
	}

	d.WorkDates, d.Actions = formatReportProcesses(processes)
	return d
}

func formatReportProcesses(processes []ASProcess) (dates, actions string) {
	var dateLines, actionLines []string
	for _, p := range processes {
		day := ""
		if !p.ProcessDatetime.IsZero() {
			day = p.ProcessDatetime.Format("2006-01-02")
		}
		content := strings.TrimSpace(p.WorkContent)
		if day == "" && content == "" {
			continue
		}
		if day != "" {
			dateLines = append(dateLines, day)
		}
		if content == "" {
			content = "—"
		}
		if day != "" {
			actionLines = append(actionLines, day+" "+content)
		} else {
			actionLines = append(actionLines, content)
		}
	}
	return strings.Join(dateLines, "\n"), strings.Join(actionLines, "\n")
}

func matchReportContact(contacts []Contact, names ...string) *Contact {
	want := map[string]struct{}{}
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n != "" {
			want[n] = struct{}{}
		}
	}
	for i := range contacts {
		if _, ok := want[strings.TrimSpace(contacts[i].FullName)]; ok {
			return &contacts[i]
		}
	}
	for i := range contacts {
		if contacts[i].IsPrimary || contacts[i].ContactRole == "primary" {
			return &contacts[i]
		}
	}
	if len(contacts) > 0 {
		return &contacts[0]
	}
	return nil
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if t := strings.TrimSpace(s); t != "" {
			return t
		}
	}
	return ""
}

func clipRunes(s string, n int) string {
	r := []rune(s)
	if n <= 0 || len(r) <= n {
		return s
	}
	return string(r[:n])
}

// SanitizeReportFilename \ / : * ? " < > | 를 _ 로 바꾼다. §12.10.7
func SanitizeReportFilename(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '\\', '/', ':', '*', '?', '"', '<', '>', '|':
			b.WriteByte('_')
		case '\n', '\r', '\t':
			b.WriteByte('_')
		default:
			if r < 32 || unicode.IsControl(r) {
				continue
			}
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// ReportIssuer attachments.keywords 에서 발급자 이름을 꺼낸다.
func (a Attachment) ReportIssuer() string {
	k := strings.TrimSpace(a.Keywords)
	if strings.HasPrefix(k, ASReportIssuerPrefix) {
		return strings.TrimSpace(strings.TrimPrefix(k, ASReportIssuerPrefix))
	}
	return k
}
