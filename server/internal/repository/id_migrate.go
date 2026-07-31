package repository

import (
	"database/sql"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	reCustomerIDV2 = regexp.MustCompile(`^C\d{2,3}-\d{2}-\d{3}$`)
	reASNumberV2   = regexp.MustCompile(`^R\d{4}-\d{3}$`)
	reProcessIDV2  = regexp.MustCompile(`^R\d{4}-\d{3}-P\d{2}$`)
)

// migrateBusinessIDsV2 고객·설치자산·접수·처리 고유번호를 신규 형식으로 일괄 이관한다.
func migrateBusinessIDsV2(db *sql.DB) error {
	var done int
	err := db.QueryRow(`SELECT last_no FROM id_sequences WHERE seq_key=?`, idFormatV2MetaKey).Scan(&done)
	if err == nil && done >= 1 {
		return nil
	}

	if _, err := db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
		return err
	}
	defer db.Exec(`PRAGMA foreign_keys=ON`)

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	custMap, custSeq, err := buildCustomerIDMap(tx)
	if err != nil {
		return err
	}
	assetMap, assetSeq, err := buildAssetIDMap(tx)
	if err != nil {
		return err
	}
	asMap, asSeq, err := buildASIDMap(tx)
	if err != nil {
		return err
	}
	procMap, procSeq, err := buildProcessIDMap(tx, asMap)
	if err != nil {
		return err
	}

	if err := applyCustomerIDMap(tx, custMap); err != nil {
		return err
	}
	if err := applyAssetIDMap(tx, assetMap); err != nil {
		return err
	}
	if err := applyASIDMap(tx, asMap); err != nil {
		return err
	}
	if err := applyProcessIDMap(tx, procMap); err != nil {
		return err
	}

	// 시퀀스 재설정 (메타 키 제외 후 재삽입)
	if _, err := tx.Exec(`DELETE FROM id_sequences WHERE seq_key != ?`, idFormatV2MetaKey); err != nil {
		return err
	}
	for k, n := range custSeq {
		if _, err := tx.Exec(`INSERT INTO id_sequences(seq_key,last_no) VALUES(?,?)`, k, n); err != nil {
			return err
		}
	}
	for k, n := range assetSeq {
		if _, err := tx.Exec(`INSERT INTO id_sequences(seq_key,last_no) VALUES(?,?)`, k, n); err != nil {
			return err
		}
	}
	for k, n := range asSeq {
		if _, err := tx.Exec(`INSERT INTO id_sequences(seq_key,last_no) VALUES(?,?)`, k, n); err != nil {
			return err
		}
	}
	for k, n := range procSeq {
		if _, err := tx.Exec(`INSERT INTO id_sequences(seq_key,last_no) VALUES(?,?)`, k, n); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`INSERT OR REPLACE INTO id_sequences(seq_key,last_no) VALUES(?,1)`, idFormatV2MetaKey); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	log.Printf("id migrate v2: customers=%d assets=%d as=%d processes=%d",
		len(custMap), len(assetMap), len(asMap), len(procMap))
	return nil
}

func buildCustomerIDMap(tx *sql.Tx) (map[string]string, map[string]int, error) {
	rows, err := tx.Query(`
		SELECT customer_id, COALESCE(main_phone,''), COALESCE(created_at,'')
		FROM customers ORDER BY created_at ASC, customer_id ASC`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	type row struct{ id, phone, created string }
	var list []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.phone, &r.created); err != nil {
			return nil, nil, err
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	out := make(map[string]string, len(list))
	seq := map[string]int{}
	used := map[string]string{} // new -> old

	for _, r := range list {
		if reCustomerIDV2.MatchString(r.id) {
			out[r.id] = r.id
			area, yy, n := parseCustomerV2(r.id)
			key := fmt.Sprintf("customer:%s:%s", area, yy)
			if n > seq[key] {
				seq[key] = n
			}
			used[r.id] = r.id
			continue
		}
		area := ExtractAreaCode(r.phone)
		yy := yearYY(r.created)
		key := fmt.Sprintf("customer:%s:%s", area, yy)
		for {
			seq[key]++
			cand := fmt.Sprintf("C%s-%s-%s", area, yy, fmtSeq3(seq[key]))
			if _, taken := used[cand]; !taken {
				out[r.id] = cand
				used[cand] = r.id
				break
			}
		}
	}
	return out, seq, nil
}

func parseCustomerV2(id string) (area, yy string, n int) {
	// C{area}-{yy}-{nnn}
	s := strings.TrimPrefix(id, "C")
	parts := strings.Split(s, "-")
	if len(parts) != 3 {
		return "000", time.Now().Format("06"), 0
	}
	fmt.Sscanf(parts[2], "%d", &n)
	return parts[0], parts[1], n
}

func yearYY(created string) string {
	t := parseTime(created)
	if t.IsZero() {
		return time.Now().Format("06")
	}
	return t.Format("06")
}

func buildAssetIDMap(tx *sql.Tx) (map[string]string, map[string]int, error) {
	rows, err := tx.Query(`
		SELECT asset_id, COALESCE(model_name,''), COALESCE(product_name,''), COALESCE(product_type,''), COALESCE(created_at,'')
		FROM assets ORDER BY created_at ASC, asset_id ASC`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	type row struct{ id, model, product, ptype, created string }
	var list []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.model, &r.product, &r.ptype, &r.created); err != nil {
			return nil, nil, err
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	out := make(map[string]string, len(list))
	seq := map[string]int{}
	used := map[string]string{}

	for _, r := range list {
		model := NormalizeModelCode(r.model, r.product)
		code := ProductTypeCode(r.ptype)
		key := fmt.Sprintf("asset:%s:%s", model, code)

		if strings.HasPrefix(r.id, "A") && strings.Contains(r.id, "-") && !strings.HasPrefix(r.id, "AST-") {
			// 이미 신규형으로 보이면 유지하고 시퀀스만 반영
			if n := trailingSeq3(r.id); n > 0 {
				out[r.id] = r.id
				used[r.id] = r.id
				if n > seq[key] {
					seq[key] = n
				}
				continue
			}
		}
		for {
			seq[key]++
			cand := fmt.Sprintf("A%s%s-%s", model, code, fmtSeq3(seq[key]))
			if _, taken := used[cand]; !taken {
				out[r.id] = cand
				used[cand] = r.id
				break
			}
		}
	}
	return out, seq, nil
}

func trailingSeq3(id string) int {
	i := strings.LastIndex(id, "-")
	if i < 0 || i+1 >= len(id) {
		return 0
	}
	var n int
	fmt.Sscanf(id[i+1:], "%d", &n)
	return n
}

func buildASIDMap(tx *sql.Tx) (map[string]string, map[string]int, error) {
	rows, err := tx.Query(`
		SELECT as_id, COALESCE(as_number,''), COALESCE(receipt_datetime,'')
		FROM as_receipts ORDER BY receipt_datetime ASC, as_id ASC`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	type row struct{ id, num, receipt string }
	var list []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.num, &r.receipt); err != nil {
			return nil, nil, err
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	out := make(map[string]string, len(list))
	seq := map[string]int{}
	used := map[string]string{}

	for _, r := range list {
		if reASNumberV2.MatchString(r.id) {
			out[r.id] = r.id
			used[r.id] = r.id
			ym, n := parseASV2(r.id)
			key := fmt.Sprintf("as:%s", ym)
			if n > seq[key] {
				seq[key] = n
			}
			continue
		}
		ym := receiptYYMM(r.receipt, r.id, r.num)
		key := fmt.Sprintf("as:%s", ym)
		for {
			seq[key]++
			cand := fmt.Sprintf("R%s-%s", ym, fmtSeq3(seq[key]))
			if _, taken := used[cand]; !taken {
				out[r.id] = cand
				used[cand] = r.id
				break
			}
		}
	}
	return out, seq, nil
}

func parseASV2(id string) (ym string, n int) {
	// R{YYMM}-{NNN}
	s := strings.TrimPrefix(id, "R")
	parts := strings.Split(s, "-")
	if len(parts) != 2 {
		return time.Now().Format("0601"), 0
	}
	fmt.Sscanf(parts[1], "%d", &n)
	return parts[0], n
}

func receiptYYMM(receipt, asID, asNum string) string {
	t := parseTime(receipt)
	if !t.IsZero() {
		return t.Format("0601")
	}
	// 구형식 202606-0008
	for _, s := range []string{asNum, asID} {
		s = strings.TrimSpace(s)
		if len(s) >= 6 && s[0] >= '0' && s[0] <= '9' {
			ym := s[:6]
			if len(ym) == 6 {
				if t2, err := time.Parse("200601", ym); err == nil {
					return t2.Format("0601")
				}
			}
		}
	}
	return time.Now().Format("0601")
}

func buildProcessIDMap(tx *sql.Tx, asMap map[string]string) (map[string]string, map[string]int, error) {
	rows, err := tx.Query(`
		SELECT process_id, as_id, COALESCE(process_datetime,'')
		FROM as_processes ORDER BY process_datetime ASC, process_id ASC`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	type row struct{ id, asID, dt string }
	var list []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.asID, &r.dt); err != nil {
			return nil, nil, err
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	// 접수번호별로 묶어서 정렬 유지
	byAS := map[string][]row{}
	for _, r := range list {
		byAS[r.asID] = append(byAS[r.asID], r)
	}
	asKeys := make([]string, 0, len(byAS))
	for k := range byAS {
		asKeys = append(asKeys, k)
	}
	sort.Strings(asKeys)

	out := make(map[string]string, len(list))
	seq := map[string]int{}
	used := map[string]string{}

	for _, oldAS := range asKeys {
		newAS := asMap[oldAS]
		if newAS == "" {
			newAS = oldAS
		}
		key := fmt.Sprintf("process:%s", newAS)
		items := byAS[oldAS]
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].dt != items[j].dt {
				return items[i].dt < items[j].dt
			}
			return items[i].id < items[j].id
		})
		for _, r := range items {
			if reProcessIDV2.MatchString(r.id) && strings.HasPrefix(r.id, newAS+"-P") {
				out[r.id] = r.id
				used[r.id] = r.id
				var n int
				fmt.Sscanf(r.id[len(newAS)+2:], "%d", &n)
				if n > seq[key] {
					seq[key] = n
				}
				continue
			}
			for {
				seq[key]++
				cand := fmt.Sprintf("%s-P%s", newAS, fmtSeq2(seq[key]))
				if _, taken := used[cand]; !taken {
					out[r.id] = cand
					used[cand] = r.id
					break
				}
			}
		}
	}
	return out, seq, nil
}

func applyCustomerIDMap(tx *sql.Tx, m map[string]string) error {
	// 자기참조·자식 테이블을 먼저 임시값으로 바꿀 필요 없이, FK off 상태에서 직접 갱신
	// parent는 customer_id 변경 후 매핑 적용
	updates := []struct{ table, col string }{
		{"customer_buildings", "customer_id"},
		{"contacts", "customer_id"},
		{"contact_history", "customer_id"},
		{"assets", "customer_id"},
		{"performance_relations", "customer_id"},
		{"as_receipts", "customer_id"},
		{"maintenance_site_config", "customer_id"},
		{"maintenance_visits", "customer_id"},
		{"attachments", "ref_id"}, // ref_type=customer 만 아래에서 필터
	}
	for old, newID := range m {
		if old == newID {
			continue
		}
		for _, u := range updates {
			if u.table == "attachments" {
				if _, err := tx.Exec(`UPDATE attachments SET ref_id=? WHERE ref_type='customer' AND ref_id=?`, newID, old); err != nil {
					return fmt.Errorf("attachments customer: %w", err)
				}
				continue
			}
			q := fmt.Sprintf(`UPDATE %s SET %s=? WHERE %s=?`, u.table, u.col, u.col)
			if _, err := tx.Exec(q, newID, old); err != nil {
				return fmt.Errorf("%s.%s: %w", u.table, u.col, err)
			}
		}
		if _, err := tx.Exec(`UPDATE customers SET parent_customer_id=? WHERE parent_customer_id=?`, newID, old); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE customers SET customer_id=? WHERE customer_id=?`, newID, old); err != nil {
			return err
		}
	}
	return nil
}

func applyAssetIDMap(tx *sql.Tx, m map[string]string) error {
	for old, newID := range m {
		if old == newID {
			continue
		}
		for _, q := range []string{
			`UPDATE asset_sw_details SET asset_id=? WHERE asset_id=?`,
			`UPDATE access_info_references SET asset_id=? WHERE asset_id=?`,
			`UPDATE performance_relations SET asset_id=? WHERE asset_id=?`,
			`UPDATE as_receipts SET asset_id=? WHERE asset_id=?`,
			`UPDATE attachments SET ref_id=? WHERE ref_type='asset' AND ref_id=?`,
			`UPDATE assets SET asset_id=? WHERE asset_id=?`,
		} {
			if _, err := tx.Exec(q, newID, old); err != nil {
				return fmt.Errorf("asset map %s: %w", q, err)
			}
		}
	}
	return nil
}

func applyASIDMap(tx *sql.Tx, m map[string]string) error {
	for old, newID := range m {
		if old == newID {
			continue
		}
		if _, err := tx.Exec(`UPDATE as_processes SET as_id=? WHERE as_id=?`, newID, old); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE attachments SET ref_id=? WHERE ref_type='as' AND ref_id=?`, newID, old); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE as_receipts SET as_id=?, as_number=? WHERE as_id=?`, newID, newID, old); err != nil {
			return err
		}
	}
	// as_id는 이미 새값인데 as_number만 구형인 경우 보정
	if _, err := tx.Exec(`UPDATE as_receipts SET as_number=as_id WHERE as_number IS NULL OR TRIM(as_number)='' OR as_number!=as_id`); err != nil {
		return err
	}
	return nil
}

func applyProcessIDMap(tx *sql.Tx, m map[string]string) error {
	for old, newID := range m {
		if old == newID {
			continue
		}
		if _, err := tx.Exec(`UPDATE as_processes SET process_id=?, process_number=? WHERE process_id=?`, newID, newID, old); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE as_processes SET process_number=process_id WHERE process_number IS NULL OR TRIM(process_number)='' OR process_number!=process_id`); err != nil {
		return err
	}
	return nil
}
