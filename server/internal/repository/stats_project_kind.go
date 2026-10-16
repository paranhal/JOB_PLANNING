package repository

import "customer-support/internal/model"

// statsProjectKindSQL 유지보수 집계에서 공사·구축·개발·기타 사업을 뺀다.
// project_id 가 비어 있으면(미귀속) 포함한다. 20f0702 와 같은 목적.
func statsProjectKindSQL(projectIDExpr string) string {
	return ` AND (
		TRIM(COALESCE(` + projectIDExpr + `,'')) = ''
		OR EXISTS (
			SELECT 1 FROM work_projects _pk
			WHERE _pk.project_id = ` + projectIDExpr + `
			  AND COALESCE(_pk.project_kind,'maintenance') = '` + model.ProjectKindMaintenance + `'
		)
	)`
}

// statsASProjectKindSQL asFilterSQL 용. 접수 또는 자산의 사업이 유지보수가 아니면 뺀다.
// asFilterSQL 을 assets 별칭 없이 쓰는 곳이 있어 a.project_id 를 직접 쓰지 않는다.
func statsASProjectKindSQL() string {
	return ` AND NOT EXISTS (
		SELECT 1 FROM work_projects _pk
		WHERE COALESCE(_pk.project_kind,'maintenance') != '` + model.ProjectKindMaintenance + `'
		  AND (
			_pk.project_id = NULLIF(TRIM(COALESCE(ar.project_id,'')),'')
			OR EXISTS (
				SELECT 1 FROM assets _a
				WHERE _a.asset_id = ar.asset_id
				  AND _pk.project_id = NULLIF(TRIM(COALESCE(_a.project_id,'')),'')
			)
		  )
	)`
}
