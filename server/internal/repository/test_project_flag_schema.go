package repository

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
)

type TestProjectFlagResult struct {
	ProjectsMarked int
	TasksMarked    int
}

func applyTestProjectNameFlagV84(db *sql.DB) {
	if db == nil {
		return
	}
	if _, err := ApplyTestProjectNameFlag(db); err != nil {
		log.Printf("084 test project flag: %v", err)
	}
}

func ApplyTestProjectNameFlag(db *sql.DB) (TestProjectFlagResult, error) {
	var out TestProjectFlagResult
	if db == nil {
		return out, nil
	}
	up, err := testProjectFlagMigrationSQL()
	if err != nil {
		return out, err
	}
	beforeP, beforeT := countTestFlag(db)
	if err := execSQLIgnoreDupCol(db, up); err != nil {
		return out, err
	}
	afterP, afterT := countTestFlag(db)
	out.ProjectsMarked = afterP - beforeP
	out.TasksMarked = afterT - beforeT
	if out.ProjectsMarked < 0 {
		out.ProjectsMarked = 0
	}
	if out.TasksMarked < 0 {
		out.TasksMarked = 0
	}
	return out, nil
}

func countTestFlag(db *sql.DB) (projects, tasks int) {
	_ = db.QueryRow(`SELECT COUNT(*) FROM work_projects WHERE COALESCE(is_test,0)=1`).Scan(&projects)
	_ = db.QueryRow(`SELECT COUNT(*) FROM work_tasks WHERE COALESCE(is_test,0)=1`).Scan(&tasks)
	return
}

func testProjectFlagMigrationSQL() (string, error) {
	for _, p := range workGroupSQLPaths("084_test_project_name_flag.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("084_test_project_name_flag.sql 없음")
}

func testProjectFlagDownSQL() (string, error) {
	for _, p := range workGroupSQLPaths("084_test_project_name_flag.down.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("084_test_project_name_flag.down.sql 없음")
}

func testProjectFlagHasSeedInsert(up string) bool {
	for _, line := range strings.Split(up, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "--") {
			continue
		}
		if strings.Contains(strings.ToUpper(trim), "INSERT") {
			return true
		}
	}
	return false
}
