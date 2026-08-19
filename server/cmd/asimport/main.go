// asimport 완료된 AS 내역 엑셀을 AS 접수/처리 데이터로 적재하는 일회성 도구.
//
//	분석:   go run ./cmd/asimport -file X.xlsx -db data/app.db -analyze
//	미리보기: go run ./cmd/asimport -file X.xlsx -db data/app.db
//	적재:   go run ./cmd/asimport -file X.xlsx -db data/app.db -apply
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

const (
	colReceiptDate = 0  // A 접수일자
	colProcessDate = 1  // B 처리일자
	colCategory    = 2  // C 제품분류
	colModel       = 3  // D 제품모델명
	colChannel     = 4  // E 접수형태
	colCustomer    = 5  // F 거래처명
	colRequester   = 6  // G 고객담당자
	colWorker      = 7  // H 수행담당자
	colSymptom     = 8  // I 내용
	colStatus      = 9  // J 최종완료여부
	colAction      = 10 // K 답변 및 처리
	dataStartRow   = 3  // 0-based: 4행부터 데이터
)

func main() {
	file := flag.String("file", "", "엑셀 파일 경로")
	dbPath := flag.String("db", "data/app.db", "SQLite 경로")
	analyze := flag.Bool("analyze", false, "값 분포·매칭 가능성 분석")
	apply := flag.Bool("apply", false, "실제 적재 (미지정 시 미리보기)")
	dump := flag.Int("dump", 0, "앞에서 N행 원본 출력")
	dates := flag.Bool("dates", false, "접수일자 파싱 결과·월별 분포 확인")
	flag.Parse()

	f, err := excelize.OpenFile(*file)
	if err != nil {
		log.Fatalf("엑셀 열기 실패: %v", err)
	}
	defer f.Close()
	rows, err := f.GetRows(f.GetSheetList()[0])
	if err != nil {
		log.Fatalf("시트 읽기 실패: %v", err)
	}

	if *dates {
		hist := counter{}
		for i := dataStartRow; i < len(rows); i++ {
			raw := cell(rows[i], colReceiptDate)
			if raw == "" {
				continue
			}
			if d, ok := parseExcelDate(raw); ok {
				hist.add(d[:7])
				if i < dataStartRow+3 || (i%150 == 0) {
					fmt.Printf("  %4d행 원본=%-12s → %s\n", i+1, raw, d)
				}
			} else {
				fmt.Printf("  %4d행 파싱실패 %q\n", i+1, raw)
			}
		}
		hist.print("접수 월별 분포", 0)
		return
	}

	if *dump > 0 {
		for i := 0; i < *dump && i < len(rows); i++ {
			fmt.Printf("%3d행: %v\n", i+1, rows[i])
		}
		return
	}

	db, err := repository.InitDB(*dbPath)
	if err != nil {
		log.Fatalf("DB 열기 실패: %v", err)
	}
	defer db.Close()

	if *analyze {
		runAnalyze(db, rows)
		return
	}

	plan := buildPlan(db, rows)
	printPlan(plan)
	if !*apply {
		fmt.Println("\n미리보기입니다. 실제 적재하려면 -apply 를 붙여 실행하세요.")
		return
	}
	if err := applyPlan(db, plan); err != nil {
		log.Fatalf("적재 실패: %v", err)
	}
}

func cell(row []string, idx int) string {
	if idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

type counter map[string]int

func (c counter) add(v string) { c[v]++ }

func (c counter) print(title string, limit int) {
	type kv struct {
		k string
		n int
	}
	list := make([]kv, 0, len(c))
	for k, n := range c {
		list = append(list, kv{k, n})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].n != list[j].n {
			return list[i].n > list[j].n
		}
		return list[i].k < list[j].k
	})
	fmt.Printf("\n--- %s (%d종) ---\n", title, len(list))
	for i, e := range list {
		if limit > 0 && i >= limit {
			fmt.Printf("  ... 외 %d종\n", len(list)-limit)
			break
		}
		label := e.k
		if label == "" {
			label = "(빈값)"
		}
		fmt.Printf("  %-40s %d\n", label, e.n)
	}
}

func runAnalyze(db *sql.DB, rows [][]string) {
	customers := loadCustomers(db)
	users := loadUsers(db)

	cats, channels, workers, statuses := counter{}, counter{}, counter{}, counter{}
	models := counter{}
	unmatchedCustomer := counter{}
	badReceipt, badProcess := counter{}, counter{}
	var total, emptyRow, matchedCustomer, emptyProcessDate, emptyAction, emptySymptom int
	minDate, maxDate := "9999", "0000"

	for i := dataStartRow; i < len(rows); i++ {
		row := rows[i]
		cust := cell(row, colCustomer)
		recv := cell(row, colReceiptDate)
		if cust == "" && recv == "" && cell(row, colSymptom) == "" {
			emptyRow++
			continue
		}
		total++

		cats.add(cell(row, colCategory))
		channels.add(cell(row, colChannel))
		workers.add(cell(row, colWorker))
		statuses.add(cell(row, colStatus))
		models.add(cell(row, colModel))

		if _, ok := customers[normalizeName(cust)]; ok {
			matchedCustomer++
		} else {
			unmatchedCustomer.add(cust)
		}

		if d, ok := parseExcelDate(recv); ok {
			if d < minDate {
				minDate = d
			}
			if d > maxDate {
				maxDate = d
			}
		} else {
			badReceipt.add(recv)
		}
		proc := cell(row, colProcessDate)
		if proc == "" {
			emptyProcessDate++
		} else if _, ok := parseExcelDate(proc); !ok {
			badProcess.add(proc)
		}
		if cell(row, colAction) == "" {
			emptyAction++
		}
		if cell(row, colSymptom) == "" {
			emptySymptom++
		}
	}

	fmt.Printf("데이터 행: %d건 (빈 행 %d건 제외)\n", total, emptyRow)
	fmt.Printf("접수일자 범위: %s ~ %s\n", minDate, maxDate)
	fmt.Printf("기관 매칭: %d/%d건\n", matchedCustomer, total)
	fmt.Printf("처리일자 없음: %d건 / 조치내용 없음: %d건 / 증상내용 없음: %d건\n",
		emptyProcessDate, emptyAction, emptySymptom)
	fmt.Printf("DB 사용자(수행담당자 후보): %d명 %v\n", len(users), userNames(users))

	reportExistingAS(db)
	reportOverlap(db, rows, customers)
	reportCustomerCandidates(db, unmatchedCustomer)
	reportAssetMatch(db, rows, customers)

	statuses.print("최종완료여부(J)", 0)
	channels.print("접수형태(E)", 0)
	cats.print("제품분류(C)", 0)
	workers.print("수행담당자(H)", 0)
	unmatchedCustomer.print("매칭 실패 기관(F)", 40)
	models.print("제품모델명(D)", 25)
	badReceipt.print("접수일자 파싱 실패", 15)
	badProcess.print("처리일자 파싱 실패", 15)
}

func reportExistingAS(db *sql.DB) {
	var n int
	var min, max sql.NullString
	db.QueryRow(`SELECT COUNT(*), MIN(receipt_datetime), MAX(receipt_datetime) FROM as_receipts`).Scan(&n, &min, &max)
	fmt.Printf("\n--- 기존 DB AS 접수 ---\n  건수=%d 기간=%s ~ %s\n", n, min.String, max.String)
	rows, err := db.Query(`SELECT as_number, COALESCE(c.org_name,''), ar.receipt_datetime, ar.status
		FROM as_receipts ar LEFT JOIN customers c ON c.customer_id=ar.customer_id
		ORDER BY ar.receipt_datetime DESC LIMIT 10`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var num, org, dt, st string
		rows.Scan(&num, &org, &dt, &st)
		fmt.Printf("  %s | %s | %s | %s\n", num, org, dt, st)
	}
}

// reportOverlap 기존 접수와 (기관+접수일자)가 겹치는 엑셀 행 수
func reportOverlap(db *sql.DB, rows [][]string, customers map[string]string) {
	existing := map[string]string{}
	q, err := db.Query(`SELECT customer_id, DATE(receipt_datetime), as_number, COALESCE(symptom,'') FROM as_receipts`)
	if err != nil {
		return
	}
	defer q.Close()
	for q.Next() {
		var cid, d, num, sym string
		q.Scan(&cid, &d, &num, &sym)
		existing[cid+"|"+d] = num + " " + firstLine(sym)
	}

	var overlap, sameRange int
	fmt.Printf("\n--- 기존 접수와 겹치는 행 ---\n")
	for i := dataStartRow; i < len(rows); i++ {
		d, ok := parseExcelDate(cell(rows[i], colReceiptDate))
		if !ok {
			continue
		}
		if d >= "2026-05-26" {
			sameRange++
		}
		cid, ok := customers[normalizeName(cell(rows[i], colCustomer))]
		if !ok {
			continue
		}
		if hit, dup := existing[cid+"|"+d]; dup {
			overlap++
			if overlap <= 12 {
				fmt.Printf("  %3d행 %s %-18s | 기존: %s\n  %s      엑셀: %s\n",
					i+1, d, cell(rows[i], colCustomer), hit, strings.Repeat(" ", 3), firstLine(cell(rows[i], colSymptom)))
			}
		}
	}
	fmt.Printf("  기존 데이터 기간(2026-05-26 이후) 엑셀 행=%d, 기관+접수일자 일치=%d\n", sameRange, overlap)
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	r := []rune(s)
	if len(r) > 40 {
		return string(r[:40]) + "…"
	}
	return s
}

// reportCustomerCandidates 매칭 실패 기관에 대해 부분일치 후보 제시
func reportCustomerCandidates(db *sql.DB, unmatched counter) {
	names := make([]string, 0, len(unmatched))
	for k := range unmatched {
		names = append(names, k)
	}
	sort.Strings(names)
	fmt.Printf("\n--- 매칭 실패 기관 후보 탐색 ---\n")
	for _, name := range names {
		key := normalizeName(name)
		best := searchCustomerLike(db, key)
		fmt.Printf("  %-24s (%2d건) → %s\n", name, unmatched[name], best)
	}
}

func searchCustomerLike(db *sql.DB, key string) string {
	runes := []rune(key)
	probes := map[string]bool{}
	for size := len(runes); size >= 2; size-- {
		for start := 0; start+size <= len(runes); start++ {
			probes[string(runes[start:start+size])] = true
		}
		if size <= 3 {
			break
		}
	}
	type hit struct {
		name string
		n    int
	}
	seen := map[string]bool{}
	var out []string
	for probe := range probes {
		if len([]rune(probe)) < 3 {
			continue
		}
		rows, err := db.Query(`SELECT org_name FROM customers
			WHERE REPLACE(org_name,' ','') LIKE '%' || ? || '%' LIMIT 3`, probe)
		if err != nil {
			continue
		}
		for rows.Next() {
			var n string
			rows.Scan(&n)
			n = strings.TrimSpace(n)
			if !seen[n] {
				seen[n] = true
				out = append(out, n)
			}
		}
		rows.Close()
		if len(out) >= 3 {
			break
		}
	}
	if len(out) == 0 {
		return "(후보 없음 · 신규 기관)"
	}
	sort.Strings(out)
	return strings.Join(out, " / ")
}

// reportAssetMatch 기관+모델명으로 자산 연결 가능한 비율
func reportAssetMatch(db *sql.DB, rows [][]string, customers map[string]string) {
	assets := map[string][]string{} // customer_id → 모델·제품명 키
	q, err := db.Query(`SELECT customer_id, COALESCE(model_name,''), COALESCE(product_name,''), asset_id FROM assets`)
	if err != nil {
		return
	}
	defer q.Close()
	for q.Next() {
		var cid, model, product, aid string
		q.Scan(&cid, &model, &product, &aid)
		for _, k := range []string{normalizeName(model), normalizeName(product)} {
			if k == "" {
				continue
			}
			assets[cid+"|"+k] = append(assets[cid+"|"+k], aid)
		}
	}

	var withModel, matched, ambiguous int
	missKeys := counter{}
	for i := dataStartRow; i < len(rows); i++ {
		model := cell(rows[i], colModel)
		if model == "" {
			continue
		}
		withModel++
		cid, ok := customers[normalizeName(cell(rows[i], colCustomer))]
		if !ok {
			continue
		}
		ids := assets[cid+"|"+normalizeName(model)]
		switch {
		case len(ids) == 1:
			matched++
		case len(ids) > 1:
			ambiguous++
		default:
			missKeys.add(cell(rows[i], colCustomer) + " / " + model)
		}
	}
	fmt.Printf("\n--- 자산 연결 가능성 ---\n  모델명 있는 행=%d, 단일 자산 매칭=%d, 동일모델 다수=%d\n",
		withModel, matched, ambiguous)
	missKeys.print("자산 매칭 실패 (기관/모델)", 15)
}

func userNames(users map[string]string) []string {
	out := make([]string, 0, len(users))
	for name := range users {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// loadCustomers 정규화된 기관명 → customer_id
func loadCustomers(db *sql.DB) map[string]string {
	out := map[string]string{}
	rows, err := db.Query(`SELECT customer_id, COALESCE(org_name,''), COALESCE(official_name,'') FROM customers`)
	if err != nil {
		log.Fatalf("고객 조회 실패: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, org, official string
		if err := rows.Scan(&id, &org, &official); err != nil {
			log.Fatal(err)
		}
		for _, n := range []string{org, official} {
			key := normalizeName(n)
			if key == "" {
				continue
			}
			if _, exists := out[key]; !exists {
				out[key] = id
			}
		}
	}
	return out
}

// loadUsers 이름 → user_id
func loadUsers(db *sql.DB) map[string]string {
	out := map[string]string{}
	rows, err := db.Query(`SELECT user_id, full_name FROM users WHERE is_active=1`)
	if err != nil {
		log.Fatalf("사용자 조회 실패: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			log.Fatal(err)
		}
		out[strings.TrimSpace(name)] = id
	}
	return out
}

// normalizeName 공백·괄호 등을 제거한 기관명 비교키
func normalizeName(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "\u00a0", "")
	s = strings.ReplaceAll(s, "(", "")
	s = strings.ReplaceAll(s, ")", "")
	s = strings.ReplaceAll(s, "·", "")
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
		out := fmt.Sprintf("%s-%s-%s", parts[0], pad2(parts[1]), pad2(parts[2]))
		if model.NormalizeAppDate(out) == "" {
			return "", false
		}
		return out, true
	}
	mm, dd, yy := pad2(parts[0]), pad2(parts[1]), parts[2]
	if len(yy) == 2 {
		yy = "20" + yy
	}
	if mm > "12" || dd > "31" {
		return "", false
	}
	out := fmt.Sprintf("%s-%s-%s", yy, mm, dd)
	if model.NormalizeAppDate(out) == "" {
		return "", false
	}
	return out, true
}

func pad2(s string) string {
	if len(s) == 1 {
		return "0" + s
	}
	return s
}
