// verifyassets 설치자산 엑셀(관리유형·계약구분·점검주기)과 DB 를 비교해 불일치 CSV 를 쓴다.
//
//	go run ./cmd/verifyassets -db data/app.db -xlsx 파일.xlsx -out output/asset_verify.csv
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func main() {
	dbPath := flag.String("db", "data/app.db", "SQLite")
	xlsx := flag.String("xlsx", "", "설치자산 엑셀")
	out := flag.String("out", "output/asset_verify.csv", "불일치 CSV")
	flag.Parse()
	if strings.TrimSpace(*xlsx) == "" {
		log.Fatal("-xlsx 가 필요합니다")
	}
	db, err := repository.InitDB(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	f, err := excelize.OpenFile(*xlsx)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	sheet := f.GetSheetName(0)
	rows, err := f.GetRows(sheet)
	if err != nil || len(rows) < 2 {
		log.Fatalf("시트 읽기: %v", err)
	}
	col := mapHeader(rows[0])
	idCol, ok := col["자산번호"]
	if !ok {
		idCol, ok = col["asset_id"]
	}
	if !ok {
		log.Fatal("자산번호 열이 없습니다")
	}

	outRows := [][]string{{"asset_id", "항목", "엑셀", "DB"}}
	for _, row := range rows[1:] {
		if idCol >= len(row) {
			continue
		}
		id := strings.TrimSpace(row[idCol])
		if id == "" {
			continue
		}
		a, err := repository.NewAssetRepo(db).GetByID(id)
		if err != nil || a == nil {
			outRows = append(outRows, []string{id, "존재", "엑셀", "DB없음"})
			continue
		}
		compareField(&outRows, id, "관리유형", cell(row, col, "관리유형"), a.ManagementType)
		compareField(&outRows, id, "계약구분", cell(row, col, "계약구분"), a.MaintContractType)
		wantCycle := cell(row, col, "점검주기")
		if model.MaintCycleDisplayLabel(a.MaintCycle) != strings.TrimSpace(wantCycle) &&
			a.MaintCycle != strings.TrimSpace(wantCycle) {
			outRows = append(outRows, []string{id, "점검주기", wantCycle, a.MaintCycle})
		}
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0755); err != nil {
		log.Fatal(err)
	}
	wf, err := os.Create(*out)
	if err != nil {
		log.Fatal(err)
	}
	w := csv.NewWriter(wf)
	_ = w.WriteAll(outRows)
	w.Flush()
	wf.Close()
	fmt.Printf("불일치 %d건 → %s\n", len(outRows)-1, *out)
}

func mapHeader(h []string) map[string]int {
	m := map[string]int{}
	for i, s := range h {
		m[strings.TrimSpace(s)] = i
	}
	return m
}

func cell(row []string, col map[string]int, name string) string {
	i, ok := col[name]
	if !ok || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

func compareField(out *[][]string, id, field, excel, db string) int {
	if strings.TrimSpace(excel) == strings.TrimSpace(db) {
		return 0
	}
	if model.ManagementTypeLabel(db) == excel {
		return 0
	}
	*out = append(*out, []string{id, field, excel, db})
	return 1
}
