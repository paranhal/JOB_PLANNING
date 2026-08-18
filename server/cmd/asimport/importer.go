package main

import (
	"database/sql"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"customer-support/internal/passwd"
	"customer-support/internal/repository"
)

const importPrefix = "as_excel_v1"

// 엑셀 기관명이 기존 고객과 표기만 다른 경우의 대응표
var customerAliases = map[string]string{
	"어진동행정복지커뮤니티센터":  "어진동행정복합커뮤니티센터",
	"아산중앙도서관":        "아산시중앙도서관",
	"한국기술교육대학교다산도서관": "한국기술교육대학교다산정보관",
	"충남교육청학생문화교육원":   "충남교육청학생교육문화원",
}

// 접수형태 → receipt_channel 코드
var channelCodes = map[string]string{
	"전화":    "phone",
	"메일":    "email",
	"이메일":   "email",
	"현장":    "visit",
	"방문":    "visit",
	"원콜":    "onecall",
	"제조사요청": "maker",
	"협력사요청": "partner",
}

type importRow struct {
	excelRow    int
	importKey   string
	receiptDate string // YYYY-MM-DD
	processDate string
	customerRaw string
	customerID  string
	newCustomer bool
	assetID     string
	assetNote   string // 자산 미연결 시 남길 제품 정보
	channel     string
	requester   string
	worker      string
	workerUID   string
	symptom     string
	action      string
}

type importPlan struct {
	rows           []importRow
	skipDuplicate  []string
	skipImported   []string
	newCustomers   map[string]int
	newUsers       map[string]int
	assetLinked    int
	assetAmbiguous int
	badDate        []string
	innerDup       int // 엑셀 안에서 기관+접수일자+증상이 똑같이 반복되는 행
}

func buildPlan(db *sql.DB, rows [][]string) *importPlan {
	plan := &importPlan{
		newCustomers: map[string]int{},
		newUsers:     map[string]int{},
	}
	customers := loadCustomerIndex(db)
	users := loadUsers(db)
	assets := loadAssetIndex(db)
	existing := loadExistingReceiptKeys(db)
	imported := loadImportedKeys(db)

	for i := dataStartRow; i < len(rows); i++ {
		row := rows[i]
		custRaw := strings.TrimSpace(cell(row, colCustomer))
		recvRaw := cell(row, colReceiptDate)
		if custRaw == "" && recvRaw == "" && cell(row, colSymptom) == "" {
			continue
		}

		key := fmt.Sprintf("%s:%d", importPrefix, i+1)
		if imported[key] {
			plan.skipImported = append(plan.skipImported, key)
			continue
		}

		recvDate, ok := parseExcelDate(recvRaw)
		if !ok {
			plan.badDate = append(plan.badDate, fmt.Sprintf("%d행 접수일자 %q", i+1, recvRaw))
			continue
		}
		procDate, ok := parseExcelDate(cell(row, colProcessDate))
		if !ok {
			procDate = recvDate
		}

		r := importRow{
			excelRow:    i + 1,
			importKey:   key,
			receiptDate: recvDate,
			processDate: procDate,
			customerRaw: custRaw,
			channel:     channelCodes[cell(row, colChannel)],
			requester:   cell(row, colRequester),
			worker:      cell(row, colWorker),
			symptom:     cell(row, colSymptom),
			action:      cell(row, colAction),
		}
		r.workerUID = users[r.worker]
		if r.worker != "" && r.workerUID == "" {
			plan.newUsers[r.worker]++
		}

		name := resolveCustomerName(custRaw)
		if cid, ok := customers[normalizeName(name)]; ok {
			r.customerID = cid
		} else {
			r.newCustomer = true
			plan.newCustomers[custRaw]++
		}

		category, model := cell(row, colCategory), cell(row, colModel)
		if r.customerID != "" && model != "" {
			ids := assets[r.customerID+"|"+normalizeName(model)]
			switch {
			case len(ids) == 1:
				r.assetID = ids[0]
				plan.assetLinked++
			case len(ids) > 1:
				plan.assetAmbiguous++
			}
		}
		if r.assetID == "" {
			var parts []string
			if category != "" {
				parts = append(parts, "제품분류: "+category)
			}
			if model != "" {
				parts = append(parts, "모델: "+model)
			}
			r.assetNote = strings.Join(parts, " / ")
		}

		if r.customerID != "" && existing[r.customerID+"|"+recvDate+"|"+normalizeName(r.symptom)] {
			plan.skipDuplicate = append(plan.skipDuplicate,
				fmt.Sprintf("%d행 %s %s %s", r.excelRow, recvDate, custRaw, firstLine(r.symptom)))
			continue
		}

		plan.rows = append(plan.rows, r)
	}

	seen := map[string]bool{}
	for _, r := range plan.rows {
		k := r.customerRaw + "|" + r.receiptDate + "|" + normalizeName(r.symptom)
		if seen[k] {
			plan.innerDup++
		}
		seen[k] = true
	}
	return plan
}

func resolveCustomerName(raw string) string {
	if alias, ok := customerAliases[normalizeName(raw)]; ok {
		return alias
	}
	return raw
}

// loadCustomerIndex 기관명·공식명칭·점검사이트 표시명 → customer_id
func loadCustomerIndex(db *sql.DB) map[string]string {
	out := loadCustomers(db)
	rows, err := db.Query(`SELECT customer_id, short_name FROM maintenance_site_config`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			continue
		}
		key := normalizeName(name)
		if key != "" {
			if _, exists := out[key]; !exists {
				out[key] = id
			}
		}
	}
	return out
}

func loadAssetIndex(db *sql.DB) map[string][]string {
	out := map[string][]string{}
	rows, err := db.Query(`SELECT customer_id, COALESCE(model_name,''), COALESCE(product_name,''), asset_id FROM assets`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var cid, model, product, aid string
		if err := rows.Scan(&cid, &model, &product, &aid); err != nil {
			continue
		}
		for _, k := range []string{normalizeName(model), normalizeName(product)} {
			if k == "" {
				continue
			}
			key := cid + "|" + k
			if !contains(out[key], aid) {
				out[key] = append(out[key], aid)
			}
		}
	}
	return out
}

func contains(list []string, v string) bool {
	for _, e := range list {
		if e == v {
			return true
		}
	}
	return false
}

// loadExistingReceiptKeys 기관+접수일자+증상 조합 (중복 판정용)
func loadExistingReceiptKeys(db *sql.DB) map[string]bool {
	out := map[string]bool{}
	rows, err := db.Query(`SELECT customer_id, DATE(receipt_datetime), COALESCE(symptom,'') FROM as_receipts`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var cid, d, sym string
		if err := rows.Scan(&cid, &d, &sym); err != nil {
			continue
		}
		out[cid+"|"+d+"|"+normalizeName(sym)] = true
	}
	return out
}

func loadImportedKeys(db *sql.DB) map[string]bool {
	out := map[string]bool{}
	rows, err := db.Query(`SELECT import_key FROM as_receipts WHERE COALESCE(import_key,'') != ''`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err == nil {
			out[k] = true
		}
	}
	return out
}

func printPlan(plan *importPlan) {
	fmt.Printf("적재 대상: %d건\n", len(plan.rows))
	fmt.Printf("중복으로 제외(기관+접수일자+증상 동일): %d건\n", len(plan.skipDuplicate))
	fmt.Printf("이미 적재된 행(재실행 방지): %d건\n", len(plan.skipImported))
	fmt.Printf("자산 연결: %d건 (동일모델 다수라 미연결: %d건)\n", plan.assetLinked, plan.assetAmbiguous)
	fmt.Printf("엑셀 안에서 기관+접수일자+증상이 같은 반복 행: %d건 (그대로 각각 등록)\n", plan.innerDup)
	if len(plan.badDate) > 0 {
		fmt.Printf("날짜 오류로 제외: %d건 %v\n", len(plan.badDate), plan.badDate)
	}

	printMapCount("신규 등록할 기관", plan.newCustomers)
	printMapCount("신규 등록할 담당자", plan.newUsers)

	for i, s := range plan.skipDuplicate {
		if i == 0 {
			fmt.Printf("\n--- 중복 제외 상세 ---\n")
		}
		fmt.Printf("  %s\n", s)
	}
}

func printMapCount(title string, m map[string]int) {
	if len(m) == 0 {
		return
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if m[keys[i]] != m[keys[j]] {
			return m[keys[i]] > m[keys[j]]
		}
		return keys[i] < keys[j]
	})
	fmt.Printf("\n--- %s (%d종) ---\n", title, len(keys))
	for _, k := range keys {
		fmt.Printf("  %-30s %d건\n", k, m[k])
	}
}

func applyPlan(db *sql.DB, plan *importPlan) error {
	custIDs := map[string]string{}
	for name := range plan.newCustomers {
		id, err := createCustomer(db, name)
		if err != nil {
			return fmt.Errorf("기관 %q 등록: %w", name, err)
		}
		custIDs[normalizeName(name)] = id
		fmt.Printf("기관 신규 등록: %s (%s)\n", name, id)
	}
	userIDs := map[string]string{}
	for name := range plan.newUsers {
		id, err := createUser(db, name)
		if err != nil {
			return fmt.Errorf("사용자 %q 등록: %w", name, err)
		}
		userIDs[name] = id
		fmt.Printf("사용자 신규 등록: %s (%s)\n", name, id)
	}

	var inserted int
	for _, r := range plan.rows {
		cid := r.customerID
		if cid == "" {
			cid = custIDs[normalizeName(r.customerRaw)]
		}
		if cid == "" {
			return fmt.Errorf("%d행 기관 확인 실패: %s", r.excelRow, r.customerRaw)
		}
		uid := r.workerUID
		if uid == "" {
			uid = userIDs[r.worker]
		}

		receiptAt, err := time.Parse("2006-01-02 15:04:05", r.receiptDate+" 09:00:00")
		if err != nil {
			return err
		}
		asNumber, err := repository.NextASNumber(db, receiptAt)
		if err != nil {
			return err
		}
		completeAt := r.processDate + " 18:00:00"
		now := time.Now().Format("2006-01-02 15:04:05")

		_, err = db.Exec(`
			INSERT INTO as_receipts (
				as_id, as_number, receipt_datetime, customer_id, asset_id,
				receipt_channel, requester, requester_name, symptom,
				urgency, priority, assigned_to, assigned_user_id,
				schedule_confirmed, status, complete_datetime, action_taken, result_code,
				import_key, created_at, updated_at
			) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			asNumber, asNumber, r.receiptDate+" 09:00:00", cid, nullIfEmpty(r.assetID),
			r.channel, r.requester, r.requester, r.symptom,
			"normal", "normal", r.worker, uid,
			1, "completed", completeAt, r.action, "done",
			r.importKey, now, now)
		if err != nil {
			return fmt.Errorf("%d행 접수 등록: %w", r.excelRow, err)
		}

		procNum, err := repository.NextProcessNumber(db, asNumber, receiptAt)
		if err != nil {
			return err
		}
		_, err = db.Exec(`
			INSERT INTO as_processes
			(process_id, process_number, as_id, process_datetime, worker, work_content, notes)
			VALUES (?,?,?,?,?,?,?)`,
			procNum, procNum, asNumber, completeAt, r.worker, r.action, r.assetNote)
		if err != nil {
			return fmt.Errorf("%d행 처리이력 등록: %w", r.excelRow, err)
		}
		inserted++
	}
	fmt.Printf("접수 %d건 + 처리이력 %d건 등록 완료\n", inserted, inserted)
	return nil
}

func nullIfEmpty(s string) interface{} {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func createCustomer(db *sql.DB, name string) (string, error) {
	id, err := repository.NextCustomerID(db, "")
	if err != nil {
		return "", err
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err = db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, notes, created_at, updated_at)
		VALUES (?,?,?,1,?,?,?)`, id, name, name, "AS 완료내역 엑셀 적재 시 자동 생성", now, now)
	if err != nil {
		return "", err
	}
	return id, nil
}

func createUser(db *sql.DB, fullName string) (string, error) {
	username := makeUsername(db, fullName)
	id := fmt.Sprintf("USR%d", time.Now().UnixNano()%1000000000)
	hash := passwd.Hash("1234")
	_, err := db.Exec(`INSERT INTO users (user_id, username, password_hash, full_name, role, is_active, created_at)
		VALUES (?,?,?,?,?,1,?)`,
		id, username, hash, fullName, "user", time.Now().Format("2006-01-02 15:04:05"))
	if err != nil {
		return "", err
	}
	return id, nil
}

// makeUsername 한글 이름은 계정명으로 쓸 수 없어 user1, user2 형태로 비어 있는 번호를 찾는다.
func makeUsername(db *sql.DB, fullName string) string {
	for i := 1; i < 1000; i++ {
		candidate := fmt.Sprintf("user%d", i)
		var n int
		db.QueryRow(`SELECT COUNT(*) FROM users WHERE username=?`, candidate).Scan(&n)
		if n == 0 {
			return candidate
		}
	}
	log.Fatalf("사용자 계정명을 만들 수 없습니다: %s", fullName)
	return ""
}
