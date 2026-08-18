package audit

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

var restoreTables = map[string]string{
	"customers":                "customer_id",
	"assets":                   "asset_id",
	"as_receipts":              "as_id",
	"as_work_items":            "work_id",
	"as_processes":             "process_id",
	"contacts":                 "contact_id",
	"contact_history":          "history_id",
	"codes":                    "code_id",
	"users":                    "user_id",
	"attachments":              "attachment_id",
	"performance_relations":    "relation_id",
	"asset_sw_details":         "sw_detail_id",
	"work_projects":            "project_id",
	"work_tasks":               "task_id",
	"work_actions":             "action_id",
	"work_activities":          "activity_id",
	"maintenance_visits":       "visit_id",
	"maintenance_site_config":  "customer_id",
	"customer_buildings":       "building_id",
	"customer_floors":          "floor_id",
	"customer_rooms":           "room_id",
	"app_settings":             "setting_key",
}

// Rollback 수정·삭제 이력을 변경 전 값으로 되돌린다. 등록은 해당 행을 삭제한다.
func Rollback(logID string) error {
	if dbRef == nil {
		return fmt.Errorf("DB가 없습니다")
	}
	l, err := getLog(logID)
	if err != nil || l == nil {
		return fmt.Errorf("이력을 찾을 수 없습니다")
	}
	if l.RolledBack {
		return fmt.Errorf("이미 롤백된 이력입니다")
	}
	pk, ok := restoreTables[l.TableName]
	if !ok {
		if l.PKColumn != "" {
			pk = l.PKColumn
		} else {
			return fmt.Errorf("이 테이블은 롤백할 수 없습니다: %s", l.TableName)
		}
	}
	if !safeIdent(l.TableName) || !safeIdent(pk) {
		return fmt.Errorf("잘못된 테이블")
	}

	switch l.Action {
	case ActionCreate:
		_, err = dbRef.Exec(fmt.Sprintf(`DELETE FROM %s WHERE %s=?`, l.TableName, pk), l.EntityID)
	case ActionUpdate, ActionDelete:
		err = restoreRow(l.TableName, pk, l.BeforeJSON)
	default:
		return fmt.Errorf("알 수 없는 동작: %s", l.Action)
	}
	if err != nil {
		return err
	}
	_, err = dbRef.Exec(`UPDATE data_change_logs SET rolled_back=1, rolled_back_at=? WHERE log_id=?`,
		time.Now().Format("2006-01-02 15:04:05"), logID)
	return err
}

func safeIdent(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func restoreRow(table, pk, raw string) error {
	var m map[string]any
	if err := json.Unmarshal([]byte(nullEmpty(raw)), &m); err != nil {
		return fmt.Errorf("이전 값 파싱: %w", err)
	}
	if len(m) == 0 {
		return fmt.Errorf("되돌릴 이전 값이 없습니다")
	}
	cols, err := tableColumns(table)
	if err != nil {
		return err
	}
	var names []string
	var ph []string
	var args []any
	for _, c := range cols {
		v, ok := m[c]
		if !ok {
			continue
		}
		if c == "password_hash" && fmt.Sprint(v) == "***" {
			continue
		}
		names = append(names, c)
		ph = append(ph, "?")
		args = append(args, jsonToSQL(v))
	}
	if len(names) == 0 {
		return fmt.Errorf("복원할 컬럼이 없습니다")
	}
	q := fmt.Sprintf(`INSERT OR REPLACE INTO %s (%s) VALUES (%s)`,
		table, strings.Join(names, ","), strings.Join(ph, ","))
	_, err = dbRef.Exec(q, args...)
	return err
}

func tableColumns(table string) ([]string, error) {
	rows, err := dbRef.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		cols = append(cols, name)
	}
	return cols, rows.Err()
}

func jsonToSQL(v any) any {
	switch t := v.(type) {
	case nil:
		return nil
	case bool:
		if t {
			return 1
		}
		return 0
	case float64:
		if t == float64(int64(t)) {
			return int64(t)
		}
		return t
	default:
		return fmt.Sprint(t)
	}
}
