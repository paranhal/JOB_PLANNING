package repository

import (
	"fmt"
	"strings"

	"customer-support/internal/model"
)

// 95-193: 주간 보고서 「사업이 연결되지 않은 업무」에 제품 라벨을 채운다.
// 항목마다 쿼리하지 않고 종류별로 ID 를 모아 한 번씩 조회한다(N+1 금지). 연결·분류 로직은 건드리지 않는다.

const unassignedProductChunk = 400

// assetProductLabel 자산 한 개의 표시: 「제품명 · 모델 (분류)」. 둘 다 비면 빈 문자열.
func assetProductLabel(name, modelName, category string) string {
	name, modelName, category = strings.TrimSpace(name), strings.TrimSpace(modelName), strings.TrimSpace(category)
	label := name
	if modelName != "" {
		if label != "" {
			label += " · " + modelName
		} else {
			label = modelName
		}
	}
	if label == "" {
		return ""
	}
	if category != "" {
		if c := ProductCategoryLabel(category); c != "" && c != "—" {
			label += " (" + c + ")"
		}
	}
	return label
}

// joinProductLabels 여러 개면 「첫 개 외 n」. 첫 개는 정렬 기준(입력 순서)이다.
func joinProductLabels(labels []string) string {
	var out []string
	seen := map[string]bool{}
	for _, l := range labels {
		l = strings.TrimSpace(l)
		if l == "" || seen[l] {
			continue
		}
		seen[l] = true
		out = append(out, l)
	}
	switch len(out) {
	case 0:
		return ""
	case 1:
		return out[0]
	}
	return fmt.Sprintf("%s 외 %d", out[0], len(out)-1)
}

func chunkIDs(ids []string, n int) [][]string {
	var out [][]string
	for len(ids) > n {
		out = append(out, ids[:n])
		ids = ids[n:]
	}
	if len(ids) > 0 {
		out = append(out, ids)
	}
	return out
}

func inClause(ids []string) (string, []interface{}) {
	ph := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		ph[i], args[i] = "?", id
	}
	return strings.Join(ph, ","), args
}

func (r *StatsRepo) fillUnassignedProductLabels(items []model.CompanyWeeklyUnassigned) {
	if r == nil || r.db == nil || len(items) == 0 {
		return
	}
	var asIDs, mntIDs, taskIDs []string
	for _, it := range items {
		id := strings.TrimSpace(it.ID)
		if id == "" {
			continue
		}
		switch it.Kind {
		case model.ScopeWorkAS:
			asIDs = append(asIDs, id)
		case model.ScopeWorkMaintenance:
			mntIDs = append(mntIDs, id)
		default:
			taskIDs = append(taskIDs, id)
		}
	}
	asLabels := map[string][]string{}
	for _, ch := range chunkIDs(asIDs, unassignedProductChunk) {
		in, args := inClause(ch)
		rows, err := r.db.Query(`SELECT ar.as_id, COALESCE(a.product_name,''), COALESCE(a.model_name,''), COALESCE(a.product_category,'')
			FROM as_receipts ar JOIN assets a ON a.asset_id = ar.asset_id
			WHERE ar.as_id IN (`+in+`) ORDER BY ar.as_id, a.asset_id`, args...)
		if err != nil {
			continue
		}
		for rows.Next() {
			var id, n, m, c string
			if rows.Scan(&id, &n, &m, &c) == nil {
				asLabels[id] = append(asLabels[id], assetProductLabel(n, m, c))
			}
		}
		rows.Close()
	}
	mntLabels := map[string][]string{}
	var mntNoType []string
	for _, ch := range chunkIDs(mntIDs, unassignedProductChunk) {
		in, args := inClause(ch)
		rows, err := r.db.Query(`SELECT visit_id, TRIM(COALESCE(product_type,'')) FROM maintenance_visits WHERE visit_id IN (`+in+`)`, args...)
		if err != nil {
			continue
		}
		for rows.Next() {
			var id, pt string
			if rows.Scan(&id, &pt) == nil {
				if pt != "" {
					mntLabels[id] = []string{pt}
				} else {
					mntNoType = append(mntNoType, id)
				}
			}
		}
		rows.Close()
	}
	// 제품 구분이 빈 점검은 점검 대상(장비)으로 보충한다.
	for _, ch := range chunkIDs(mntNoType, unassignedProductChunk) {
		in, args := inClause(ch)
		rows, err := r.db.Query(`SELECT t.visit_id, COALESCE(NULLIF(TRIM(a.product_name),''), TRIM(COALESCE(t.name,''))), COALESCE(a.model_name,''), COALESCE(a.product_category,'')
			FROM maintenance_visit_targets t LEFT JOIN assets a ON a.asset_id = t.asset_id
			WHERE t.visit_id IN (`+in+`) ORDER BY t.visit_id, t.sort_order, t.target_id`, args...)
		if err != nil {
			continue
		}
		for rows.Next() {
			var id, n, m, c string
			if rows.Scan(&id, &n, &m, &c) == nil {
				mntLabels[id] = append(mntLabels[id], assetProductLabel(n, m, c))
			}
		}
		rows.Close()
	}
	taskLabels := map[string][]string{}
	for _, ch := range chunkIDs(taskIDs, unassignedProductChunk) {
		in, args := inClause(ch)
		rows, err := r.db.Query(`SELECT wa.task_id, COALESCE(a.product_name,''), COALESCE(a.model_name,''), COALESCE(a.product_category,'')
			FROM work_task_assets wa JOIN assets a ON a.asset_id = wa.asset_id
			WHERE wa.task_id IN (`+in+`) ORDER BY wa.task_id, a.asset_id`, args...)
		if err != nil {
			continue // work_task_assets 가 없는 옛 DB
		}
		for rows.Next() {
			var id, n, m, c string
			if rows.Scan(&id, &n, &m, &c) == nil {
				taskLabels[id] = append(taskLabels[id], assetProductLabel(n, m, c))
			}
		}
		rows.Close()
	}
	for i := range items {
		id := strings.TrimSpace(items[i].ID)
		switch items[i].Kind {
		case model.ScopeWorkAS:
			items[i].ProductLabel = joinProductLabels(asLabels[id])
		case model.ScopeWorkMaintenance:
			items[i].ProductLabel = joinProductLabels(mntLabels[id])
		default:
			items[i].ProductLabel = joinProductLabels(taskLabels[id])
		}
	}
}
