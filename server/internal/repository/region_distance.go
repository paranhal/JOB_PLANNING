package repository

import (
	"database/sql"
	"log"
	"strings"

	"customer-support/internal/model"
)

// applyRegionDistanceOrder 마이그레이션 029. §34.4.3
func applyRegionDistanceOrder(db *sql.DB) {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS region_distance_order (
			sido       TEXT NOT NULL,
			sigungu    TEXT NOT NULL DEFAULT '',
			sort_order INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (sido, sigungu)
		)`); err != nil {
		log.Printf("029 region_distance_order: %v", err)
	}
}

func (r *MaintenanceRepo) ListRegionDistanceOrder() ([]model.RegionDistanceOrder, error) {
	rows, err := r.db.Query(`
		SELECT sido, COALESCE(sigungu,''), sort_order
		FROM region_distance_order
		ORDER BY sort_order, sido, sigungu`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.RegionDistanceOrder
	for rows.Next() {
		var it model.RegionDistanceOrder
		if err := rows.Scan(&it.Sido, &it.Sigungu, &it.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// ReplaceRegionDistanceOrder 관리자 화면에서 정한 순서로 통째로 갈아끼운다.
func (r *MaintenanceRepo) ReplaceRegionDistanceOrder(items []model.RegionDistanceOrder) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM region_distance_order`); err != nil {
		return err
	}
	st, err := tx.Prepare(`INSERT INTO region_distance_order (sido, sigungu, sort_order) VALUES (?,?,?)`)
	if err != nil {
		return err
	}
	defer st.Close()
	seen := map[string]bool{}
	for _, it := range items {
		sido := strings.TrimSpace(it.Sido)
		sigungu := strings.TrimSpace(it.Sigungu)
		if sido == "" && sigungu == "" {
			continue
		}
		key := model.RegionKey(sido, sigungu)
		if seen[key] {
			continue
		}
		seen[key] = true
		if _, err := st.Exec(sido, sigungu, it.SortOrder); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListSiteRegionCandidates 사이트 설정에 있는 고객의 시·도/시군구.
func (r *MaintenanceRepo) ListSiteRegionCandidates() ([]model.RegionDistanceOrder, error) {
	rows, err := r.db.Query(`
		SELECT DISTINCT
			TRIM(COALESCE(c.addr_sido,'')),
			TRIM(COALESCE(c.addr_sigungu,''))
		FROM maintenance_site_config s
		JOIN customers c ON c.customer_id = s.customer_id
		WHERE TRIM(COALESCE(c.addr_sido,'')) != ''
		   OR TRIM(COALESCE(c.addr_sigungu,'')) != ''
		ORDER BY 1, 2`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.RegionDistanceOrder
	for rows.Next() {
		var it model.RegionDistanceOrder
		if err := rows.Scan(&it.Sido, &it.Sigungu); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}
