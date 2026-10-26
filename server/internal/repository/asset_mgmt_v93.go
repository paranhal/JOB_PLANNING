package repository

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"customer-support/internal/model"
)

type AssetMgmtChange struct {
	AssetID string
	OrgName string
	Field   string
	Before  string
	After   string
}

type AssetMgmtV93Result struct {
	MgmtBefore     map[string]int
	MgmtAfter      map[string]int
	NoneCycleAfter map[string]int
	Changes        []AssetMgmtChange
	Unresolved     []AssetMgmtChange
	Contradict     map[string]int
	BackupPath     string
}

func applyAssetMgmtTypesV93(db *sql.DB) {
	if db == nil {
		return
	}
	if err := applyAssetMgmtCodesV93(db); err != nil {
		log.Printf("093 asset mgmt codes: %v", err)
	}
}

func ApplyAssetMgmtTypesV93ForTest(db *sql.DB) (*AssetMgmtV93Result, error) {
	return ApplyAssetMgmtTypesV93(db, true)
}

func ApplyAssetMgmtTypesV93(db *sql.DB, write bool) (*AssetMgmtV93Result, error) {
	res := &AssetMgmtV93Result{
		MgmtBefore:     map[string]int{},
		MgmtAfter:      map[string]int{},
		NoneCycleAfter: map[string]int{},
		Contradict:     map[string]int{},
	}
	if db == nil {
		return res, nil
	}
	if write {
		if err := applyAssetMgmtCodesV93(db); err != nil {
			return res, err
		}
	}
	tx, err := db.Begin()
	if err != nil {
		return res, err
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.Query(`
		SELECT a.asset_id,
		       COALESCE(NULLIF(TRIM(c.org_name),''), a.customer_id),
		       COALESCE(a.management_type,''),
		       COALESCE(a.maint_contract_type,''),
		       COALESCE(a.maint_cycle,'')
		  FROM assets a
		  LEFT JOIN customers c ON c.customer_id=a.customer_id
		 ORDER BY a.asset_id`)
	if err != nil {
		return res, err
	}
	type row struct {
		id, org, mgmt, contract, cycle string
	}
	var all []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.org, &r.mgmt, &r.contract, &r.cycle); err != nil {
			rows.Close()
			return res, err
		}
		all = append(all, r)
		res.MgmtBefore[mgmtKey(r.mgmt)]++
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return res, err
	}
	rows.Close()

	upMgmt, err := tx.Prepare(`UPDATE assets SET management_type=? WHERE asset_id=?`)
	if err != nil {
		return res, err
	}
	defer upMgmt.Close()
	upCycle, err := tx.Prepare(`UPDATE assets SET maint_cycle=? WHERE asset_id=?`)
	if err != nil {
		return res, err
	}
	defer upCycle.Close()

	for _, r := range all {
		nextMgmt := model.MapBulkManagementType(r.mgmt, r.contract)
		if nextMgmt != r.mgmt {
			if write {
				if _, err := upMgmt.Exec(nextMgmt, r.id); err != nil {
					return res, err
				}
			}
			res.Changes = append(res.Changes, AssetMgmtChange{r.id, r.org, "관리유형", r.mgmt, nextMgmt})
			r.mgmt = nextMgmt
		} else if strings.TrimSpace(r.mgmt) == "" && strings.TrimSpace(r.contract) == "" {
			res.Unresolved = append(res.Unresolved, AssetMgmtChange{r.id, r.org, "관리유형", "", ""})
		}
		nextCycle := model.MapBulkMaintCycle(r.contract, r.cycle)
		if nextCycle != r.cycle {
			if write {
				if _, err := upCycle.Exec(nextCycle, r.id); err != nil {
					return res, err
				}
			}
			res.Changes = append(res.Changes, AssetMgmtChange{r.id, r.org, "점검주기", r.cycle, nextCycle})
			r.cycle = nextCycle
		}
		res.MgmtAfter[mgmtKey(r.mgmt)]++
		if strings.ToLower(strings.TrimSpace(r.contract)) == model.MaintContractNone {
			ck := strings.TrimSpace(r.cycle)
			if ck == "" {
				res.NoneCycleAfter["(빈값)"]++
			} else {
				res.NoneCycleAfter[ck]++
			}
		}
		if strings.EqualFold(strings.TrimSpace(r.mgmt), model.ManagementTypeFault) &&
			strings.EqualFold(strings.TrimSpace(r.contract), model.MaintContractPaid) {
			res.Contradict["fault+paid"]++
		}
		if strings.EqualFold(strings.TrimSpace(r.mgmt), model.ManagementTypePeriodic) &&
			strings.EqualFold(strings.TrimSpace(r.contract), model.MaintContractNone) {
			res.Contradict["periodic+none"]++
		}
	}
	sort.Slice(res.Changes, func(i, j int) bool {
		if res.Changes[i].AssetID == res.Changes[j].AssetID {
			return res.Changes[i].Field < res.Changes[j].Field
		}
		return res.Changes[i].AssetID < res.Changes[j].AssetID
	})
	sort.Slice(res.Unresolved, func(i, j int) bool { return res.Unresolved[i].AssetID < res.Unresolved[j].AssetID })
	if err := tx.Commit(); err != nil {
		return res, err
	}
	return res, nil
}

func mgmtKey(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "(미입력)"
	}
	return s
}

func applyAssetMgmtCodesV93(db *sql.DB) error {
	stmts := []string{
		`UPDATE codes SET is_active=0 WHERE code_group='management_type' AND code_value IN ('direct','on_demand','reference')`,
		`UPDATE codes SET sort_order=1, is_active=1, code_name='장애대응' WHERE code_group='management_type' AND code_value='fault'`,
		`UPDATE codes SET sort_order=2, is_active=1, code_name='정기점검' WHERE code_group='management_type' AND code_value='periodic'`,
		`UPDATE codes SET sort_order=3, is_active=1, code_name='타사장비' WHERE code_group='management_type' AND code_value='third_party'`,
		`INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order, is_active) VALUES ('MCY007','maint_cycle','call','CALL',6,1)`,
		`UPDATE codes SET sort_order=6, is_active=1, code_name='CALL' WHERE code_group='maint_cycle' AND code_value='call'`,
		`UPDATE codes SET sort_order=7 WHERE code_group='maint_cycle' AND code_value='custom'`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}
	return nil
}

func BackupAppDB(dbPath string) (string, error) {
	dbPath = strings.TrimSpace(dbPath)
	if dbPath == "" || dbPath == ":memory:" {
		return "", fmt.Errorf("백업할 DB 경로가 없습니다")
	}
	src, err := os.Open(dbPath)
	if err != nil {
		return "", err
	}
	defer src.Close()
	stamp := time.Now().Format("20060102-150405")
	dstPath := filepath.Join(filepath.Dir(dbPath), fmt.Sprintf("app.db.bak-093-%s", stamp))
	if filepath.Base(dbPath) != "app.db" {
		dstPath = dbPath + ".bak-093-" + stamp
	}
	dst, err := os.Create(dstPath)
	if err != nil {
		return "", err
	}
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		return "", err
	}
	return dstPath, nil
}

func WriteMigration093Reports(outDir string, res *AssetMgmtV93Result) error {
	if res == nil {
		return fmt.Errorf("결과 없음")
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return err
	}
	if err := writeChangeCSV(filepath.Join(outDir, "migration_093_changes.csv"), res.Changes); err != nil {
		return err
	}
	return writeChangeCSV(filepath.Join(outDir, "migration_093_unresolved.csv"), res.Unresolved)
}

func writeChangeCSV(path string, rows []AssetMgmtChange) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	_ = w.Write([]string{"asset_id", "기관명", "변경항목", "이전값", "이후값"})
	for _, r := range rows {
		_ = w.Write([]string{r.AssetID, r.OrgName, r.Field, r.Before, r.After})
	}
	w.Flush()
	return w.Error()
}

func RevertAssetMgmtFromCSV(db *sql.DB, csvPath string) (int, error) {
	f, err := os.Open(csvPath)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	rd := csv.NewReader(f)
	rec, err := rd.ReadAll()
	if err != nil {
		return 0, err
	}
	n := 0
	for i, row := range rec {
		if i == 0 || len(row) < 5 {
			continue
		}
		id, field, before := strings.TrimSpace(row[0]), strings.TrimSpace(row[2]), row[3]
		col := ""
		switch field {
		case "관리유형":
			col = "management_type"
		case "점검주기":
			col = "maint_cycle"
		default:
			continue
		}
		if _, err := db.Exec(`UPDATE assets SET `+col+`=? WHERE asset_id=?`, before, id); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
