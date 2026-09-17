package repository

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestQueryPlanUsesLookupIndexes(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "plan.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	seedQueryPlanFixture(t, db)
	if _, err := db.Exec(`ANALYZE`); err != nil {
		t.Fatal(err)
	}

	const customerList = `
		SELECT c.customer_id, c.org_name, c.official_name,
		       COALESCE(c.industry,''), COALESCE(c.main_phone,''), c.is_active,
		       COUNT(DISTINCT a.asset_id) AS asset_count,
		       COUNT(DISTINCT ar.as_id) AS as_count,
		       COALESCE(c.parent_customer_id,''),
		       COALESCE(p.org_name,''),
		       CASE WHEN c.has_parent=1 AND COALESCE(c.parent_customer_id,'')!='' THEN 1 ELSE 0 END,
		       COALESCE(c.needs_review,0), COALESCE(c.review_reason,''),
		       COALESCE(NULLIF(TRIM(c.party_kind),''),'customer')
		FROM customers c
		LEFT JOIN customers p ON p.customer_id = c.parent_customer_id
		LEFT JOIN assets a ON a.customer_id = c.customer_id
		LEFT JOIN as_receipts ar ON ar.customer_id = c.customer_id
		WHERE 1=1
		GROUP BY c.customer_id
		LIMIT 20 OFFSET 0`

	cases := []struct {
		name string
		sql  string
		args []any
		want []string
		ban  []string
	}{
		{
			name: "as_processes",
			sql:  `SELECT process_id FROM as_processes WHERE as_id=?`,
			args: []any{"R1"},
			want: []string{"SEARCH", "USING INDEX idx_as_processes_as_"},
			ban:  []string{"AUTOMATIC", "SCAN as_processes"},
		},
		{
			name: "customer_list",
			sql:  customerList,
			ban:  []string{"AUTOMATIC"},
		},
		{
			name: "customer_rooms",
			sql:  `SELECT room_id FROM customer_rooms WHERE floor_id=?`,
			args: []any{"FL1"},
			want: []string{"USING INDEX"},
			ban:  []string{"AUTOMATIC", "SCAN customer_rooms"},
		},
		{
			name: "attachments",
			sql:  `SELECT attachment_id FROM attachments WHERE ref_type=? AND ref_id=?`,
			args: []any{"as", "R1"},
			want: []string{"USING INDEX"},
			ban:  []string{"AUTOMATIC", "SCAN attachments"},
		},
		{
			name: "customer_floors",
			sql:  `SELECT floor_id FROM customer_floors WHERE building_id=?`,
			args: []any{"B1"},
			want: []string{"USING INDEX"},
			ban:  []string{"AUTOMATIC", "SCAN customer_floors"},
		},
		{
			name: "assets_by_customer",
			sql:  `SELECT asset_id FROM assets WHERE customer_id=?`,
			args: []any{"C1"},
			want: []string{"USING INDEX"},
			ban:  []string{"AUTOMATIC", "SCAN assets"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan := explainQueryPlan(t, db, tc.sql, tc.args...)
			for _, w := range tc.want {
				if !strings.Contains(plan, w) {
					t.Fatalf("want %q\nplan:\n%s", w, plan)
				}
			}
			for _, b := range tc.ban {
				if strings.Contains(plan, b) {
					t.Fatalf("ban %q\nplan:\n%s", b, plan)
				}
			}
		})
	}

	for _, name := range []string{
		"idx_as_processes_as_id",
		"idx_as_receipts_customer_id",
		"idx_assets_customer_id",
		"idx_customer_rooms_floor_id",
		"idx_attachments_ref",
		"idx_as_receipts_open",
		"ux_mnt_visit_unique",
	} {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?`, name).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("index %s missing", name)
		}
	}
	for _, name := range []string{"idx_maintenance_visit_dedup2", "idx_data_backups_folder"} {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?`, name).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("duplicate index %s still present", name)
		}
	}
}

func seedQueryPlanFixture(t *testing.T, db *sql.DB) {
	t.Helper()
	// 행이 적으면 SQLite 가 인덱스가 있어도 SCAN 을 고른다. 선택도가 나오게 흩뿌린다.
	for i := 0; i < 80; i++ {
		cid := fmt.Sprintf("C%d", i)
		if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
			VALUES (?,?,?,1)`, cid, "기관"+cid, "기관"+cid); err != nil {
			t.Fatal(err)
		}
		bid := fmt.Sprintf("B%d", i)
		if _, err := db.Exec(`INSERT INTO customer_buildings (building_id, customer_id, building_name)
			VALUES (?,?,?)`, bid, cid, "본관"); err != nil {
			t.Fatal(err)
		}
		fid := fmt.Sprintf("FL%d", i)
		if _, err := db.Exec(`INSERT INTO customer_floors (floor_id, building_id, floor_name, sort_order)
			VALUES (?,?,?,1)`, fid, bid, "1층"); err != nil {
			t.Fatal(err)
		}
		for r := 0; r < 4; r++ {
			if _, err := db.Exec(`INSERT INTO customer_rooms (room_id, floor_id, room_name) VALUES (?,?,?)`,
				fmt.Sprintf("RM%d-%d", i, r), fid, fmt.Sprintf("실%d", r)); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := db.Exec(`INSERT INTO assets (asset_id, customer_id, product_name) VALUES (?,?,?)`,
			fmt.Sprintf("A%d", i), cid, "KLAS"); err != nil {
			t.Fatal(err)
		}
		asID := fmt.Sprintf("R%d", i)
		if _, err := db.Exec(`INSERT INTO as_receipts (as_id, as_number, customer_id, receipt_datetime, status, schedule_confirmed)
			VALUES (?,?,?,'2026-09-01 10:00:00','completed',0)`, asID, asID, cid); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO as_processes (process_id, as_id, process_datetime, time_spent) VALUES (?,?,?,30)`,
			fmt.Sprintf("P%d", i), asID, "2026-09-01 11:00:00"); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO attachments (attachment_id, ref_type, ref_id, file_name, file_path)
			VALUES (?,'as',?, 'a.jpg','/tmp/a.jpg')`, fmt.Sprintf("ATT%d", i), asID); err != nil {
			t.Fatal(err)
		}
	}
}

func explainQueryPlan(t *testing.T, db *sql.DB, q string, args ...any) string {
	t.Helper()
	rows, err := db.Query("EXPLAIN QUERY PLAN "+q, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatal(err)
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		detail := vals[len(vals)-1]
		switch v := detail.(type) {
		case string:
			b.WriteString(v)
		case []byte:
			b.WriteString(string(v))
		default:
			b.WriteString(fmt.Sprint(v))
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return b.String()
}
