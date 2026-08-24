package repository

import (
	"database/sql"
	"log"
	"strings"

	"customer-support/internal/model"
)

const projectKindCodesMetaKey = "__meta:project_kind_codes_v1"

// applyProjectKind 마이그레이션 011. §22.1.1 ①·⑪단계 1
func applyProjectKind(db *sql.DB) {
	if db == nil {
		return
	}
	if _, err := db.Exec(`ALTER TABLE work_projects ADD COLUMN project_kind TEXT NOT NULL DEFAULT 'maintenance'`); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		log.Printf("011 work_projects.project_kind: %v", err)
	}
	if _, err := db.Exec(`
		UPDATE work_projects
		   SET project_kind = ?
		 WHERE TRIM(COALESCE(project_kind,'')) = ''`, model.ProjectKindMaintenance); err != nil {
		log.Printf("011 work_projects.project_kind backfill: %v", err)
	}
	// 기존 시드 6건은 1회만 유지보수로 고정한다. 이후 유형을 바꿔도 재기동 때 덮어쓰지 않는다.
	const wpseedKindMeta = "__meta:project_kind_wpseed_v1"
	if !metaDone(db, wpseedKindMeta) {
		if _, err := db.Exec(`
			UPDATE work_projects SET project_kind = ?
			 WHERE project_id LIKE 'WPSEED%'`, model.ProjectKindMaintenance); err != nil {
			log.Printf("011 work_projects WPSEED kind: %v", err)
		} else {
			markMetaDone(db, wpseedKindMeta)
		}
	}
	seedProjectKindCodes(db)
}

func seedProjectKindCodes(db *sql.DB) {
	if metaDone(db, projectKindCodesMetaKey) {
		return
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order) VALUES
		('PK001','project_kind','maintenance','유지보수',1),
		('PK002','project_kind','build','신규구축',2),
		('PK003','project_kind','supply','장비납품',3),
		('PK004','project_kind','consumable','소모품납품',4),
		('PK005','project_kind','other','기타',5),
		('WPK001','weekly_ref_project_kind','build','정보시스템_구축',1),
		('WPK002','weekly_ref_project_kind','build','1_CCTV_설치',2),
		('WPK003','weekly_ref_project_kind','build','2_통신_공사',3),
		('WPK004','weekly_ref_project_kind','build','3_무선_인프라_구축',4),
		('WPK005','weekly_ref_project_kind','maintenance','4_인프라_유지_관리',5),
		('WPK006','weekly_ref_project_kind','other','기타_활동',6),
		('WPK007','weekly_ref_project_kind','supply','기타_활동',7),
		('WPK008','weekly_ref_project_kind','consumable','기타_활동',8)`); err != nil {
		log.Printf("011 project_kind codes: %v", err)
		return
	}
	markMetaDone(db, projectKindCodesMetaKey)
}

// statsProjectKindSQL §4 지표에서 영업 사업을 뺀다. project_id 가 비어 있으면(미귀속 AS·점검) 포함한다.
func statsProjectKindSQL(projectIDExpr string) string {
	return ` AND (
		TRIM(COALESCE(` + projectIDExpr + `,'')) = ''
		OR EXISTS (
			SELECT 1 FROM work_projects _pk
			WHERE _pk.project_id = ` + projectIDExpr + `
			  AND COALESCE(_pk.project_kind,'maintenance') = 'maintenance'
		)
	)`
}
