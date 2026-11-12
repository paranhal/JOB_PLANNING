package repository

import (
	"sort"
	"strings"

	"customer-support/internal/model"
)

// 95-179D: 하부 업무 배지. 하부 업무에 배지가 있고 그 하부 업무가 사이트에서 완료(package_sites.status='done')되면
// 그 사이트에 배지가 붙는다. 하부 업무마다 따로 쌓이므로 여러 하부 업무가 끝나면 배지가 여러 개 보인다.
// 같은 배지 이름을 여러 하부 업무가 가지면 사이트 단위로는 한 번만 보인다(가장 이른 완료일).

// SiteBadge 사이트에 붙은 배지 하나.
type SiteBadge struct {
	Label, Tone string
	Package     string // 어느 하부 업무가 끝났는가(툴팁)
	Date        string // 그 하부 업무가 그 사이트에서 완료된 날(YYYY-MM-DD)
}

// BadgeCount 사업 단위 배지 요약(주간 보고서·업무 회의): 그 배지가 붙은 사이트 수.
type BadgeCount struct {
	Label, Tone string
	Count       int
}

// SiteBadgesByCustomer 사업의 사이트별(고객 ID 기준) 배지. 완료일 순, 같은 이름은 한 번만.
// 사업 한 번에 묶어서 조회한다(N+1 없음).
func (r *ProjectRepo) SiteBadgesByCustomer(projectID string) (map[string][]SiteBadge, error) {
	out := map[string][]SiteBadge{}
	if strings.TrimSpace(projectID) == "" {
		return out, nil
	}
	rows, err := r.db.Query(`
		SELECT s.customer_id, pk.badge, pk.name,
		       COALESCE((SELECT MIN(e.event_date) FROM package_events e
		                 WHERE e.package_site_id=ps.package_site_id AND e.event_type='complete'), substr(ps.updated_at,1,10)) AS done_at
		FROM package_sites ps
		JOIN phase_packages pk ON pk.package_id=ps.package_id AND pk.is_active=1
		JOIN project_phases ph ON ph.phase_id=pk.phase_id
		JOIN project_sites s ON s.id=ps.project_site_id
		WHERE ph.project_id=? AND ps.status='done' AND TRIM(COALESCE(pk.badge,''))!=''
		  AND TRIM(COALESCE(s.customer_id,''))!=''
		ORDER BY s.customer_id, done_at, pk.seq, pk.package_id`, projectID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") || strings.Contains(err.Error(), "no such column") {
			return out, nil
		}
		return out, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var cid, label, pkg, date string
		if err := rows.Scan(&cid, &label, &pkg, &date); err != nil {
			return out, err
		}
		label = strings.TrimSpace(label)
		if seen[cid+"\t"+label] {
			continue
		}
		seen[cid+"\t"+label] = true
		out[cid] = append(out[cid], SiteBadge{Label: label, Tone: model.BadgeTone(label), Package: pkg, Date: date})
	}
	return out, rows.Err()
}

// BadgeSummary 사업별 배지 요약(배지가 붙은 사이트 수, 많은 순). 주간 보고서·업무 회의용으로 사업 여러 개를 한 번에 조회한다.
func (r *ProjectRepo) BadgeSummary(projectIDs []string) (map[string][]BadgeCount, error) {
	out := map[string][]BadgeCount{}
	if len(projectIDs) == 0 {
		return out, nil
	}
	ph, args := inArgs(projectIDs)
	rows, err := r.db.Query(`
		SELECT ph.project_id, TRIM(pk.badge), COUNT(DISTINCT ps.project_site_id)
		FROM package_sites ps
		JOIN phase_packages pk ON pk.package_id=ps.package_id AND pk.is_active=1
		JOIN project_phases ph ON ph.phase_id=pk.phase_id
		WHERE ph.project_id IN (`+ph+`) AND ps.status='done' AND TRIM(COALESCE(pk.badge,''))!=''
		GROUP BY ph.project_id, TRIM(pk.badge)`, args...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") || strings.Contains(err.Error(), "no such column") {
			return out, nil
		}
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var pid, label string
		var n int
		if err := rows.Scan(&pid, &label, &n); err != nil {
			return out, err
		}
		out[pid] = append(out[pid], BadgeCount{Label: label, Tone: model.BadgeTone(label), Count: n})
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	for _, list := range out {
		sort.SliceStable(list, func(i, j int) bool {
			if list[i].Count != list[j].Count {
				return list[i].Count > list[j].Count
			}
			return list[i].Label < list[j].Label
		})
	}
	return out, nil
}
