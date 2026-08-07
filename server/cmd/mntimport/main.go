// mntimport 월간 정기점검 일정 엑셀을 읽어 방문 계획으로 넣는 도구.
// 열: A=기관명 B=구분 C=제품분류 D=원담당자 E=방문예정일
//
//	미리보기:  go run ./cmd/mntimport -file 8월정기점검.xlsx -db data/app.db
//	실제 반영:  ... -apply -done-until 2026-08-04
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"sort"
	"strings"
	"unicode"

	"github.com/xuri/excelize/v2"
	"golang.org/x/text/unicode/norm"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

const (
	colOrg      = 0
	colKind     = 1
	colProduct  = 2
	colAssignee = 3
	colDate     = 4

	dataStartRow = 1 // 0-based: 1행은 머리글
)

// customerAliases 엑셀 표기 → DB 기관명
var customerAliases = map[string]string{
	"어진동행정복지커뮤니티센터":  "어진동행정복합커뮤니티센터",
	"아산중앙도서관":        "아산시중앙도서관",
	"한국기술교육대학교다산도서관": "한국기술교육대학교다산정보관",
	"충남교육청학생문화교육원":   "충남교육청학생교육문화원",
}

type visitRow struct {
	excelRow   int
	orgRaw     string
	customerID string
	product    string
	assignee   string
	date       string
	completed  bool
}

type importPlan struct {
	rows       []visitRow
	noDate     []string
	noCustomer []string
	duplicate  []string
	existing   []string
}

func main() {
	file := flag.String("file", "", "엑셀 경로")
	dbPath := flag.String("db", "data/app.db", "SQLite 경로")
	year := flag.Int("year", 0, "계획 연도 (0이면 엑셀 날짜에서 추론)")
	doneUntil := flag.String("done-until", "", "이 날짜까지는 방문 완료로 표시 (YYYY-MM-DD)")
	apply := flag.Bool("apply", false, "실제로 저장 (미지정 시 미리보기)")
	find := flag.String("find", "", "매칭 확인용 — 이 글자가 든 기관명 키를 바이트까지 출력")
	flag.Parse()

	if *file == "" {
		log.Fatal("-file 을 지정하세요")
	}

	rows, err := readSheet(*file)
	if err != nil {
		log.Fatalf("엑셀 읽기 실패: %v", err)
	}

	db, err := repository.InitDB(*dbPath)
	if err != nil {
		log.Fatalf("DB 열기 실패: %v", err)
	}
	defer db.Close()

	repo := repository.NewMaintenanceRepo(db)
	custIndex := loadCustomerIndex(db)

	if *find != "" {
		for key, id := range custIndex {
			if strings.Contains(key, *find) {
				fmt.Printf("%s | %q | % x\n", id, key, []byte(key))
			}
		}
		return
	}

	plan := buildPlan(rows, custIndex, *doneUntil)
	planYear := *year
	if planYear == 0 {
		planYear = guessYear(plan.rows)
	}
	if planYear == 0 {
		log.Fatal("연도를 알 수 없습니다. -year 로 지정하세요")
	}

	existing, err := repo.GetPlanByYear(planYear)
	if err != nil {
		log.Fatalf("계획 조회 실패: %v", err)
	}
	if existing != nil {
		markExisting(db, existing.PlanID, &plan)
	}

	printPlan(plan, planYear, existing != nil, *doneUntil)
	if !*apply {
		fmt.Println("\n미리보기입니다. 실제로 넣으려면 -apply 를 붙이세요.")
		return
	}

	planID := ""
	if existing != nil {
		planID = existing.PlanID
	} else {
		p, err := repo.CreatePlan(planYear, fmt.Sprintf("%d년 정기점검", planYear))
		if err != nil {
			log.Fatalf("계획 생성 실패: %v", err)
		}
		planID = p.PlanID
		fmt.Printf("계획 생성: %d년 (%s)\n", planYear, planID)
	}

	inserted := 0
	for _, r := range plan.rows {
		v := model.MaintenanceVisit{
			PlanID:      planID,
			VisitDate:   r.date,
			CustomerID:  r.customerID,
			ProductType: r.product,
			Assignee:    r.assignee,
			Completed:   r.completed,
		}
		if err := repo.InsertVisitFull(v); err != nil {
			fmt.Printf("  [%d행] %s %s 실패: %v\n", r.excelRow, r.date, r.orgRaw, err)
			continue
		}
		inserted++
	}
	repo.TouchPlanUpdated(planID)
	fmt.Printf("\n방문 %d건 등록 완료 (완료 표시 %d건)\n", inserted, countDone(plan.rows))
}

func readSheet(path string) ([][]string, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("시트가 없습니다")
	}
	return f.GetRows(sheets[0])
}

func buildPlan(rows [][]string, custIndex map[string]string, doneUntil string) importPlan {
	var plan importPlan
	seen := map[string]int{}

	for i, row := range rows {
		if i < dataStartRow {
			continue
		}
		org := cell(row, colOrg)
		if org == "" {
			continue
		}
		excelRow := i + 1

		date, ok := parseExcelDate(cell(row, colDate))
		if !ok {
			plan.noDate = append(plan.noDate,
				fmt.Sprintf("%d행 %s (%s)", excelRow, org, cell(row, colProduct)))
			continue
		}

		cid, ok := lookupCustomer(custIndex, org)
		if !ok {
			plan.noCustomer = append(plan.noCustomer, fmt.Sprintf("%d행 %s", excelRow, org))
			continue
		}

		product := cell(row, colProduct)
		key := date + "|" + cid + "|" + product
		if prev, dup := seen[key]; dup {
			plan.duplicate = append(plan.duplicate,
				fmt.Sprintf("%d행 %s %s %s (=%d행)", excelRow, date, org, product, prev))
			continue
		}
		seen[key] = excelRow

		plan.rows = append(plan.rows, visitRow{
			excelRow:   excelRow,
			orgRaw:     org,
			customerID: cid,
			product:    product,
			assignee:   cell(row, colAssignee),
			date:       date,
			completed:  doneUntil != "" && date <= doneUntil,
		})
	}
	return plan
}

// markExisting 이미 같은 계획에 들어 있는 방문은 빼고 넣는다(중복 실행 대비).
func markExisting(db *sql.DB, planID string, plan *importPlan) {
	rows, err := db.Query(`
		SELECT visit_date, customer_id, COALESCE(product_type,'')
		FROM maintenance_visits WHERE plan_id = ?`, planID)
	if err != nil {
		return
	}
	defer rows.Close()
	have := map[string]bool{}
	for rows.Next() {
		var d, cid, p string
		if err := rows.Scan(&d, &cid, &p); err != nil {
			continue
		}
		have[d+"|"+cid+"|"+p] = true
	}
	var keep []visitRow
	for _, r := range plan.rows {
		if have[r.date+"|"+r.customerID+"|"+r.product] {
			plan.existing = append(plan.existing,
				fmt.Sprintf("%d행 %s %s %s", r.excelRow, r.date, r.orgRaw, r.product))
			continue
		}
		keep = append(keep, r)
	}
	plan.rows = keep
}

func printPlan(plan importPlan, year int, planExists bool, doneUntil string) {
	fmt.Printf("== 정기점검 일정 적재 계획 (%d년) ==\n", year)
	if planExists {
		fmt.Println("연도 계획: 기존 계획에 추가")
	} else {
		fmt.Println("연도 계획: 신규 생성")
	}

	byDate := map[string]int{}
	byProduct := map[string]int{}
	byAssignee := map[string]int{}
	for _, r := range plan.rows {
		byDate[r.date]++
		byProduct[r.product]++
		byAssignee[r.assignee]++
	}

	fmt.Printf("\n등록 대상 %d건 (완료 표시 %d건, 기준 %s)\n",
		len(plan.rows), countDone(plan.rows), orDash(doneUntil))
	printCount("점검 대상", byProduct)
	printCount("담당자", byAssignee)

	fmt.Println("\n-- 날짜별 --")
	dates := make([]string, 0, len(byDate))
	for d := range byDate {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	for _, d := range dates {
		fmt.Printf("  %s : %d건\n", d, byDate[d])
	}

	printList("날짜 없음(건너뜀)", plan.noDate)
	printList("기관 매칭 실패(건너뜀)", plan.noCustomer)
	printList("엑셀 내 중복(건너뜀)", plan.duplicate)
	printList("이미 등록됨(건너뜀)", plan.existing)
}

func printCount(title string, m map[string]int) {
	if len(m) == 0 {
		return
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s %d", orDash(k), m[k]))
	}
	fmt.Printf("  %s: %s\n", title, strings.Join(parts, " · "))
}

func printList(title string, list []string) {
	if len(list) == 0 {
		return
	}
	fmt.Printf("\n-- %s %d건 --\n", title, len(list))
	for _, s := range list {
		fmt.Println("  " + s)
	}
}

func countDone(rows []visitRow) int {
	n := 0
	for _, r := range rows {
		if r.completed {
			n++
		}
	}
	return n
}

func guessYear(rows []visitRow) int {
	for _, r := range rows {
		if len(r.date) >= 4 {
			y := 0
			fmt.Sscanf(r.date[:4], "%d", &y)
			return y
		}
	}
	return 0
}

func loadCustomerIndex(db *sql.DB) map[string]string {
	out := map[string]string{}
	add := func(name, id string) {
		key := normalizeName(name)
		if key == "" {
			return
		}
		if _, exists := out[key]; !exists {
			out[key] = id
		}
	}
	rows, err := db.Query(`SELECT customer_id, org_name, COALESCE(official_name,'') FROM customers`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id, org, official string
			if err := rows.Scan(&id, &org, &official); err != nil {
				continue
			}
			add(org, id)
			add(official, id)
		}
	}
	srows, err := db.Query(`SELECT customer_id, short_name FROM maintenance_site_config`)
	if err == nil {
		defer srows.Close()
		for srows.Next() {
			var id, name string
			if err := srows.Scan(&id, &name); err != nil {
				continue
			}
			add(name, id)
		}
	}
	return out
}

// lookupCustomer 엑셀 기관명 → customer_id.
// 적힌 이름을 먼저 찾고, 없을 때만 별칭·'작은도서관' 같은 표기 차이를 시도한다.
func lookupCustomer(index map[string]string, raw string) (string, bool) {
	key := normalizeName(raw)
	if id, ok := index[key]; ok {
		return id, true
	}
	if alias, ok := customerAliases[key]; ok {
		if id, ok := index[normalizeName(alias)]; ok {
			return id, true
		}
	}
	// 도서관 ↔ 작은도서관처럼 한쪽에만 붙는 수식어
	if strings.HasSuffix(key, "도서관") && !strings.HasSuffix(key, "작은도서관") {
		if id, ok := index[strings.TrimSuffix(key, "도서관")+"작은도서관"]; ok {
			return id, true
		}
	}
	return "", false
}

func cell(row []string, idx int) string {
	if idx < len(row) {
		return strings.TrimSpace(row[idx])
	}
	return ""
}

// normalizeName 기관명 비교용 키. 공백·괄호는 물론 눈에 안 보이는
// 제로폭·서식 문자까지 지운다(엑셀·기존 데이터에 섞여 들어오는 경우가 있다).
func normalizeName(s string) string {
	// 한글은 조합형(NFD)으로 저장된 데이터가 섞여 있어 완성형(NFC)으로 맞춘다.
	s = norm.NFC.String(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case unicode.IsSpace(r), unicode.IsControl(r):
			continue
		case r == '\u200b', r == '\u200c', r == '\u200d', r == '\ufeff', r == '\u00a0':
			continue
		case unicode.In(r, unicode.Cf):
			continue
		case strings.ContainsRune("()[]·-_.,/", r):
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// parseExcelDate "MM-DD-YY", "YYYY-MM-DD", "YYYY/MM/DD" → "YYYY-MM-DD"
func parseExcelDate(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, ".", "-")
	parts := strings.Split(s, "-")
	if len(parts) != 3 {
		return "", false
	}
	for _, p := range parts {
		if p == "" {
			return "", false
		}
		for _, r := range p {
			if r < '0' || r > '9' {
				return "", false
			}
		}
	}
	if len(parts[0]) == 4 {
		return fmt.Sprintf("%s-%s-%s", parts[0], pad2(parts[1]), pad2(parts[2])), true
	}
	mm, dd, yy := pad2(parts[0]), pad2(parts[1]), parts[2]
	if len(yy) == 2 {
		yy = "20" + yy
	}
	if mm > "12" || dd > "31" {
		return "", false
	}
	return fmt.Sprintf("%s-%s-%s", yy, mm, dd), true
}

func pad2(s string) string {
	if len(s) == 1 {
		return "0" + s
	}
	return s
}
