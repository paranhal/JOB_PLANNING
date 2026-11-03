package repository

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
)

func applyItemTemplatesV98(db *sql.DB) {
	if db == nil {
		return
	}
	up, err := itemTemplatesMigrationSQL()
	if err != nil {
		log.Printf("098 wb_item_templates: %v", err)
		return
	}
	if err := execWorkGroupSQLIgnoreDupCol(db, up); err != nil {
		log.Printf("098 wb_item_templates apply: %v", err)
	}
}

func itemTemplatesMigrationSQL() (string, error) {
	for _, p := range workGroupSQLPaths("098_wb_item_templates.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("098_wb_item_templates.sql 없음")
}

func itemTemplatesDownSQL() (string, error) {
	for _, p := range workGroupSQLPaths("098_wb_item_templates.down.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("098_wb_item_templates.down.sql 없음")
}

func itemTemplateSQLForbiddenProdPatch(sqlText string) error {
	for _, line := range strings.Split(sqlText, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "--") {
			continue
		}
		u := strings.ToUpper(trim)
		if strings.Contains(u, "UPDATE ") && strings.Contains(u, "WORK_TASK_ITEMS") {
			return fmt.Errorf("운영 행 UPDATE 금지")
		}
		if strings.Contains(u, "INSERT INTO WORK_TASK_ITEMS") {
			return fmt.Errorf("work_task_items INSERT 금지")
		}
	}
	return nil
}
