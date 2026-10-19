package hwpx

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"customer-support/internal/model"
)

// LeavePermitKeys 근태허가원 §6.2 자리표시자. 빠진 키가 있으면 Replace가 {{ 잔여로 막는다.
var LeavePermitKeys = []string{
	"문서번호", "작성일", "작성자",
	"ck연차", "ck반차", "ck월차", "ck휴가", "ck조퇴", "ck외출", "ck결근", "ck기타", "기타내용",
	"ck시간", "시간_년", "시간_월", "시간_일", "시간_시작", "시간_종료", "시간_수",
	"ck일", "일_시작", "일_종료", "일_수",
	"ck대체", "대체_일자", "ck기근무", "ck근무예정",
	"사유", "행선지", "연락처", "업무대행자",
	"작성사인", "작성결재일", "검토사인", "검토결재일", "승인사인", "승인결재일", "승인표기",
}

const (
	LeavePeriodHours = "hours"
	LeavePeriodDays  = "days"
	LeavePeriodSwap  = "swap"
	leaveChecked     = "☑"
	leaveUnchecked   = "☐"
)

// DefaultLeaveTemplatePath 서버가 읽는 작업 템플릿. 원본 양식은 OfficialLeaveFormPath.
func DefaultLeaveTemplatePath() string {
	return filepath.Join("templates", "report", "근태허가원.hwpx")
}

// LeavePermitInput 출력 채움값. 원장·UI와 무관하다. §6.2
type LeavePermitInput struct {
	DocNo, WrittenOn, Author                                    string
	TypeKey, TypeNote                                           string
	PeriodMode                                                  string
	HourYear, HourMonth, HourDay, HourStart, HourEnd, HourCount string
	DayStart, DayEnd, DayCount                                  string
	SwapDate                                                    string
	SwapWorked, SwapScheduled                                   bool
	Reason, Destination, Phone, Deputy                          string
	AuthorStampDate, ApproveStampDate                           string
	ApproveMark                                                 string
}

// LeavePermitValues §6.2 키를 모두 채운다. 해당 없는 기간 칸·검토란은 공백.
func LeavePermitValues(in LeavePermitInput) map[string]string {
	m := map[string]string{}
	for _, k := range LeavePermitKeys {
		m[k] = ""
	}
	m["문서번호"] = in.DocNo
	m["작성일"] = in.WrittenOn
	m["작성자"] = in.Author
	m["사유"] = in.Reason
	m["행선지"] = in.Destination
	m["연락처"] = in.Phone
	m["업무대행자"] = in.Deputy
	m["작성결재일"] = in.AuthorStampDate
	m["승인결재일"] = in.ApproveStampDate
	m["승인표기"] = in.ApproveMark
	// 검토란은 항상 공백(양식 사선). §6.2
	m["검토사인"] = ""
	m["검토결재일"] = ""

	typeOn := map[string]string{
		"annual": "ck연차", "half": "ck반차", "monthly": "ck월차", "vacation": "ck휴가",
		"early_leave": "ck조퇴", "outing": "ck외출", "absence": "ck결근", "other": "ck기타",
	}
	for _, k := range []string{"ck연차", "ck반차", "ck월차", "ck휴가", "ck조퇴", "ck외출", "ck결근", "ck기타"} {
		m[k] = leaveUnchecked
	}
	if key, ok := typeOn[in.TypeKey]; ok {
		m[key] = leaveChecked
	}
	if in.TypeKey == "other" {
		m["기타내용"] = in.TypeNote
	}

	m["ck시간"] = leaveUnchecked
	m["ck일"] = leaveUnchecked
	m["ck대체"] = leaveUnchecked
	m["ck기근무"] = leaveUnchecked
	m["ck근무예정"] = leaveUnchecked
	switch in.PeriodMode {
	case LeavePeriodHours:
		m["ck시간"] = leaveChecked
		m["시간_년"], m["시간_월"], m["시간_일"] = in.HourYear, in.HourMonth, in.HourDay
		m["시간_시작"], m["시간_종료"], m["시간_수"] = in.HourStart, in.HourEnd, in.HourCount
	case LeavePeriodDays:
		m["ck일"] = leaveChecked
		m["일_시작"], m["일_종료"], m["일_수"] = in.DayStart, in.DayEnd, in.DayCount
	case LeavePeriodSwap:
		m["ck대체"] = leaveChecked
		m["대체_일자"] = in.SwapDate
		if in.SwapWorked {
			m["ck기근무"] = leaveChecked
		}
		if in.SwapScheduled {
			m["ck근무예정"] = leaveChecked
		}
	}
	return m
}

// FillLeavePermit 서명은 InlineImageAt, 본문은 Replace. 검토 서명은 넣지 않는다.
// 처리된 칸만 stamps/names를 넘긴다. 둘 다 비면 공란(결재 중). D-94
func FillLeavePermit(tpl []byte, vals map[string]string, stamps map[string][]byte, names map[string]string, box SignatureBox) ([]byte, error) {
	out := tpl
	for _, key := range []string{"작성사인", "승인사인"} {
		png := stamps[key]
		name := strings.TrimSpace(names[key])
		if len(png) == 0 && name == "" {
			continue
		}
		mark := model.SignatureOrName(png, name, "", "")
		fallback := mark.Name
		if fallback == "" {
			fallback = name
		}
		var err error
		out, err = InlineImageAt(out, key, mark.PNG, box, fallback)
		if err != nil {
			return nil, err
		}
	}
	return Replace(out, vals)
}

// OfficialLeaveFormPath docs/서식 의 원본 근태허가원. 자리표시자 작업본은 건너뛴다.
func OfficialLeaveFormPath() (string, error) {
	roots := leaveSearchRoots()
	var found []string
	for _, root := range roots {
		dir := filepath.Join(root, "docs", "서식")
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !strings.Contains(norm.NFC.String(name), "근태허가원") || !strings.HasSuffix(strings.ToLower(name), ".hwpx") {
				continue
			}
			path := filepath.Join(dir, name)
			data, err := os.ReadFile(path)
			if err != nil || len(data) == 0 {
				continue
			}
			if hwpxHasPlaceholder(data, "문서번호") {
				continue
			}
			found = append(found, path)
		}
		if len(found) > 0 {
			break
		}
	}
	if len(found) == 0 {
		return "", fmt.Errorf("docs/서식 에서 근태허가원 원본 HWPX를 찾지 못했습니다")
	}
	if len(found) > 1 {
		return "", fmt.Errorf("근태허가원 원본이 여러 개입니다: %s", strings.Join(found, ", "))
	}
	return found[0], nil
}

func leaveSearchRoots() []string {
	var roots []string
	wd, err := os.Getwd()
	if err == nil {
		cur := wd
		for i := 0; i < 6; i++ {
			roots = append(roots, cur)
			parent := filepath.Dir(cur)
			if parent == cur {
				break
			}
			cur = parent
		}
	}
	return roots
}

func hwpxHasPlaceholder(data []byte, key string) bool {
	entries, err := unzipEntries(data)
	if err != nil {
		return false
	}
	token := "{{" + key + "}}"
	for _, e := range entries {
		if strings.Contains(normalizePlaceholders(string(e.Body)), token) {
			return true
		}
	}
	return false
}

// BuildLeavePermitTemplate 원본을 읽기만 하고 자리표시자 작업본을 만든다. §6.2 · §6.3
func BuildLeavePermitTemplate(original []byte) ([]byte, error) {
	if len(original) == 0 {
		return nil, fmt.Errorf("근태허가원 원본이 비어 있습니다")
	}
	entries, err := unzipEntries(original)
	if err != nil {
		return nil, fmt.Errorf("근태허가원 원본을 열 수 없습니다: %w", err)
	}
	var secOK bool
	for i, e := range entries {
		n := filepath.ToSlash(e.Name)
		switch {
		case n == "Contents/section0.xml":
			body, err := injectLeavePlaceholders(string(e.Body))
			if err != nil {
				return nil, err
			}
			entries[i].Body = []byte(body)
			secOK = true
		case strings.EqualFold(n, "Preview/PrvImage.png"):
			entries[i].Body = previewPNG()
		case strings.EqualFold(n, "Preview/PrvText.txt"):
			entries[i].Body = encodeUTF16LE(leavePreviewText())
		case n == "Contents/content.hpf":
			entries[i].Body = sanitizeLeavePackageMeta(e.Body)
		}
	}
	if !secOK {
		return nil, fmt.Errorf("Contents/section0.xml 이 없습니다")
	}
	return packHWPX(entries)
}

// WriteLeavePermitTemplate 작업 템플릿을 새로 쓴다. 원본 파일은 덮어쓰지 않는다.
func WriteLeavePermitTemplate(path string) error {
	if path == "" {
		path = DefaultLeaveTemplatePath()
	}
	src, err := OfficialLeaveFormPath()
	if err != nil {
		return err
	}
	if sameLeavePath(path, src) {
		return fmt.Errorf("원본 양식 경로에 작업 템플릿을 쓸 수 없습니다: %s", path)
	}
	orig, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	tpl, err := BuildLeavePermitTemplate(orig)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, tpl, 0644)
}

func sameLeavePath(a, b string) bool {
	aa, err1 := filepath.Abs(a)
	bb, err2 := filepath.Abs(b)
	if err1 != nil || err2 != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return aa == bb
}

func leavePreviewText() string {
	var b strings.Builder
	b.WriteString("근태허가원\n")
	for _, key := range LeavePermitKeys {
		b.WriteString("{{")
		b.WriteString(key)
		b.WriteString("}}\n")
	}
	return b.String()
}

func sanitizeLeavePackageMeta(raw []byte) []byte {
	s := string(raw)
	metaInner := regexp.MustCompile(`(<opf:meta name="(?:creator|lastsaveby)"[^>]*>)[^<]*(</opf:meta>)`)
	s = metaInner.ReplaceAllString(s, `${1}${2}`)
	return []byte(s)
}

func injectLeavePlaceholders(sec string) (string, error) {
	repls := [][2]string{
		{`문서번호 : VI-근태-260807-001`, `문서번호 : {{문서번호}}`},
		{`작 성 일 : 2026년 08월 07일`, `작 성 일 : {{작성일}}`},
		{`작 성 자 : 이    해   진   `, `작 성 자 : {{작성자}}`},
		{
			`<hp:t>☑</hp:t></hp:run><hp:run charPrIDRef="5"><hp:t>연차 ☐반차 ☐월차 ☐휴가 ☐조퇴 ☐외출 ☐결근 ☐기타(            )</hp:t>`,
			`<hp:t>{{ck연차}}연차 {{ck반차}}반차 {{ck월차}}월차 {{ck휴가}}휴가 {{ck조퇴}}조퇴 {{ck외출}}외출 {{ck결근}}결근 {{ck기타}}기타({{기타내용}})</hp:t>`,
		},
		{
			` ☐ 2026년   월   일   시 ~     시(  시간)  `,
			` {{ck시간}} {{시간_년}}년 {{시간_월}}월 {{시간_일}}일 {{시간_시작}}시 ~ {{시간_종료}}시({{시간_수}}시간)`,
		},
		{
			`<hp:t> </hp:t></hp:run><hp:run charPrIDRef="10"><hp:t>☑</hp:t></hp:run><hp:run charPrIDRef="5"><hp:t> 2026년 08월 10일  ~ 08월 10일( 1 일간)</hp:t>`,
			`<hp:t>{{ck일}} {{일_시작}} ~ {{일_종료}}({{일_수}} 일간)</hp:t>`,
		},
		{
			` ☐ 대체근무 : 2026년   월   일(☐旣근무 ☐근무예정)`,
			` {{ck대체}} 대체근무 : {{대체_일자}}({{ck기근무}}旣근무 {{ck근무예정}}근무예정)`,
		},
		{`<hp:t>개인 사정</hp:t>`, `<hp:t>{{사유}}</hp:t>`},
		{`<hp:t>천안시</hp:t>`, `<hp:t>{{행선지}}</hp:t>`},
		{`<hp:t>010-4206-3197</hp:t>`, `<hp:t>{{연락처}}</hp:t>`},
		{`<hp:t>태자운</hp:t>`, `<hp:t>{{업무대행자}}</hp:t>`},
		{
			`<hp:p id="0" paraPrIDRef="14" styleIDRef="0" pageBreak="0" columnBreak="0" merged="0"><hp:run charPrIDRef="5"/><hp:linesegarray><hp:lineseg textpos="0" vertpos="0" vertsize="1100" textheight="1100" baseline="935" spacing="660" horzpos="0" horzsize="6360" flags="393216"/></hp:linesegarray></hp:p></hp:subList><hp:cellAddr colAddr="6" rowAddr="1"/>`,
			`<hp:p id="0" paraPrIDRef="14" styleIDRef="0" pageBreak="0" columnBreak="0" merged="0"><hp:run charPrIDRef="5"><hp:t>{{작성사인}}</hp:t></hp:run><hp:linesegarray><hp:lineseg textpos="0" vertpos="0" vertsize="1100" textheight="1100" baseline="935" spacing="660" horzpos="0" horzsize="6360" flags="393216"/></hp:linesegarray></hp:p></hp:subList><hp:cellAddr colAddr="6" rowAddr="1"/>`,
		},
		{
			`<hp:p id="0" paraPrIDRef="14" styleIDRef="0" pageBreak="0" columnBreak="0" merged="0"><hp:run charPrIDRef="5"/><hp:linesegarray><hp:lineseg textpos="0" vertpos="0" vertsize="1100" textheight="1100" baseline="935" spacing="660" horzpos="0" horzsize="6332" flags="393216"/></hp:linesegarray></hp:p></hp:subList><hp:cellAddr colAddr="9" rowAddr="1"/>`,
			`<hp:p id="0" paraPrIDRef="14" styleIDRef="0" pageBreak="0" columnBreak="0" merged="0"><hp:run charPrIDRef="5"><hp:t>{{검토사인}}</hp:t></hp:run><hp:linesegarray><hp:lineseg textpos="0" vertpos="0" vertsize="1100" textheight="1100" baseline="935" spacing="660" horzpos="0" horzsize="6332" flags="393216"/></hp:linesegarray></hp:p></hp:subList><hp:cellAddr colAddr="9" rowAddr="1"/>`,
		},
		{
			`<hp:p id="0" paraPrIDRef="14" styleIDRef="0" pageBreak="0" columnBreak="0" merged="0"><hp:run charPrIDRef="5"/><hp:linesegarray><hp:lineseg textpos="0" vertpos="0" vertsize="1100" textheight="1100" baseline="935" spacing="660" horzpos="0" horzsize="6524" flags="393216"/></hp:linesegarray></hp:p></hp:subList><hp:cellAddr colAddr="11" rowAddr="1"/>`,
			`<hp:p id="0" paraPrIDRef="14" styleIDRef="0" pageBreak="0" columnBreak="0" merged="0"><hp:run charPrIDRef="5"><hp:t>{{승인사인}}</hp:t></hp:run><hp:linesegarray><hp:lineseg textpos="0" vertpos="0" vertsize="1100" textheight="1100" baseline="935" spacing="660" horzpos="0" horzsize="6524" flags="393216"/></hp:linesegarray></hp:p></hp:subList><hp:cellAddr colAddr="11" rowAddr="1"/>`,
		},
		{
			`<hp:p id="0" paraPrIDRef="14" styleIDRef="0" pageBreak="0" columnBreak="0" merged="0"><hp:run charPrIDRef="5"><hp:t>/</hp:t></hp:run><hp:linesegarray><hp:lineseg textpos="0" vertpos="0" vertsize="1100" textheight="1100" baseline="935" spacing="660" horzpos="0" horzsize="6360" flags="393216"/></hp:linesegarray></hp:p></hp:subList><hp:cellAddr colAddr="6" rowAddr="2"/>`,
			`<hp:p id="0" paraPrIDRef="14" styleIDRef="0" pageBreak="0" columnBreak="0" merged="0"><hp:run charPrIDRef="5"><hp:t>{{작성결재일}}</hp:t></hp:run><hp:linesegarray><hp:lineseg textpos="0" vertpos="0" vertsize="1100" textheight="1100" baseline="935" spacing="660" horzpos="0" horzsize="6360" flags="393216"/></hp:linesegarray></hp:p></hp:subList><hp:cellAddr colAddr="6" rowAddr="2"/>`,
		},
		{
			`<hp:p id="0" paraPrIDRef="14" styleIDRef="0" pageBreak="0" columnBreak="0" merged="0"><hp:run charPrIDRef="5"/><hp:linesegarray><hp:lineseg textpos="0" vertpos="0" vertsize="1100" textheight="1100" baseline="935" spacing="660" horzpos="0" horzsize="6332" flags="393216"/></hp:linesegarray></hp:p></hp:subList><hp:cellAddr colAddr="9" rowAddr="2"/>`,
			`<hp:p id="0" paraPrIDRef="14" styleIDRef="0" pageBreak="0" columnBreak="0" merged="0"><hp:run charPrIDRef="5"><hp:t>{{검토결재일}}</hp:t></hp:run><hp:linesegarray><hp:lineseg textpos="0" vertpos="0" vertsize="1100" textheight="1100" baseline="935" spacing="660" horzpos="0" horzsize="6332" flags="393216"/></hp:linesegarray></hp:p></hp:subList><hp:cellAddr colAddr="9" rowAddr="2"/>`,
		},
		{
			`<hp:p id="0" paraPrIDRef="14" styleIDRef="0" pageBreak="0" columnBreak="0" merged="0"><hp:run charPrIDRef="5"><hp:t>/</hp:t></hp:run><hp:linesegarray><hp:lineseg textpos="0" vertpos="0" vertsize="1100" textheight="1100" baseline="935" spacing="660" horzpos="0" horzsize="6524" flags="393216"/></hp:linesegarray></hp:p></hp:subList><hp:cellAddr colAddr="11" rowAddr="2"/>`,
			`<hp:p id="0" paraPrIDRef="14" styleIDRef="0" pageBreak="0" columnBreak="0" merged="0"><hp:run charPrIDRef="5"><hp:t>{{승인결재일}}{{승인표기}}</hp:t></hp:run><hp:linesegarray><hp:lineseg textpos="0" vertpos="0" vertsize="1100" textheight="1100" baseline="935" spacing="660" horzpos="0" horzsize="6524" flags="393216"/></hp:linesegarray></hp:p></hp:subList><hp:cellAddr colAddr="11" rowAddr="2"/>`,
		},
	}
	out := sec
	for _, r := range repls {
		n := strings.Count(out, r[0])
		if n != 1 {
			return "", fmt.Errorf("원본 치환 지점이 %d개입니다 (1개여야 함): %s", n, snippet(r[0], 0))
		}
		out = strings.Replace(out, r[0], r[1], 1)
	}
	out = normalizePlaceholders(out)
	for _, key := range LeavePermitKeys {
		token := "{{" + key + "}}"
		if !strings.Contains(out, token) {
			return "", fmt.Errorf("자리표시자 %s 가 없습니다", token)
		}
		if !placeholderInSingleT(out, key) {
			return "", fmt.Errorf("자리표시자 %s 가 한 <hp:t> 안에 없습니다", key)
		}
	}
	return out, nil
}

func placeholderInSingleT(sec, key string) bool {
	token := "{{" + key + "}}"
	i := strings.Index(sec, token)
	if i < 0 {
		return false
	}
	open := strings.LastIndex(sec[:i], "<hp:t")
	if open < 0 {
		return false
	}
	gt := strings.IndexByte(sec[open:], '>')
	if gt < 0 {
		return false
	}
	start := open + gt + 1
	close := strings.Index(sec[start:], "</hp:t>")
	if close < 0 {
		return false
	}
	inner := sec[start : start+close]
	return strings.Contains(inner, token) && !strings.Contains(inner, "<")
}

func officialLeaveSampleStrings(original []byte) ([]string, error) {
	entries, err := unzipEntries(original)
	if err != nil {
		return nil, err
	}
	var sec string
	var prv []byte
	for _, e := range entries {
		n := filepath.ToSlash(e.Name)
		if n == "Contents/section0.xml" {
			sec = string(e.Body)
		}
		if strings.EqualFold(n, "Preview/PrvText.txt") {
			prv = e.Body
		}
	}
	if sec == "" {
		return nil, fmt.Errorf("원본 본문이 없습니다")
	}
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	if author := between(sec, "작 성 자 : ", "</hp:t>"); author != "" {
		add(author)
		add(compactSpaces(author))
	}
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile(`<hp:t>(개인 사정)</hp:t>`),
		regexp.MustCompile(`<hp:t>(010-\d{3,4}-\d{4})</hp:t>`),
		regexp.MustCompile(`<hp:t>행선지</hp:t>[\s\S]*?<hp:t>([^<]+)</hp:t>`),
		regexp.MustCompile(`<hp:t>업무대행자</hp:t>[\s\S]*?<hp:t>([^<]+)</hp:t>`),
	} {
		if m := re.FindStringSubmatch(sec); len(m) > 1 {
			add(m[1])
			add(compactSpaces(m[1]))
		}
	}
	if prvTxt := decodeUTF16LE(prv); prvTxt != "" {
		for _, s := range out {
			if strings.Contains(prvTxt, s) {
				add(s)
			}
		}
	}
	if len(out) < 3 {
		return nil, fmt.Errorf("원본에서 샘플 개인정보 문자열을 충분히 찾지 못했습니다")
	}
	return out, nil
}

func compactSpaces(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsSpace(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func between(s, a, b string) string {
	i := strings.Index(s, a)
	if i < 0 {
		return ""
	}
	i += len(a)
	if b == "" {
		return s[i:]
	}
	j := strings.Index(s[i:], b)
	if j < 0 {
		return ""
	}
	return s[i : i+j]
}

func decodeUTF16LE(b []byte) string {
	if len(b) < 2 {
		return ""
	}
	start := 0
	if b[0] == 0xFF && b[1] == 0xFE {
		start = 2
	}
	u := b[start:]
	if len(u)%2 == 1 {
		u = u[:len(u)-1]
	}
	runes := make([]rune, 0, len(u)/2)
	for i := 0; i+1 < len(u); i += 2 {
		runes = append(runes, rune(uint16(u[i])|uint16(u[i+1])<<8))
	}
	return string(runes)
}

func zipContainsAny(doc []byte, needles []string) (string, bool) {
	entries, err := unzipEntries(doc)
	if err != nil {
		return err.Error(), true
	}
	for _, e := range entries {
		body := e.Body
		text := string(body)
		if isPlainPreview(e.Name) {
			text = decodeUTF16LE(body)
		}
		for _, n := range needles {
			if n == "" {
				continue
			}
			if strings.Contains(text, n) || strings.Contains(string(body), n) {
				return n + " in " + e.Name, true
			}
			if compact := compactSpaces(n); compact != n && compact != "" {
				if strings.Contains(compactSpaces(text), compact) {
					return compact + " in " + e.Name, true
				}
			}
		}
	}
	return "", false
}
