// syncsites 유상 유지보수 계약 자산 기준으로 정기점검 사이트 설정을 일괄 생성하는 일회성 도구.
// 화면의 "자산 기준 일괄 추가" 버튼과 같은 로직을 사용한다.
package main

import (
	"flag"
	"fmt"
	"log"

	"customer-support/internal/repository"
)

func main() {
	dbPath := flag.String("db", "data/app.db", "SQLite 파일 경로")
	apply := flag.Bool("apply", false, "실제로 저장 (미지정 시 미리보기)")
	flag.Parse()

	db, err := repository.InitDB(*dbPath)
	if err != nil {
		log.Fatalf("DB 열기 실패: %v", err)
	}
	defer db.Close()

	repo := repository.NewMaintenanceRepo(db)

	if !*apply {
		targets, err := repo.PreviewSiteConfigsFromAssets()
		if err != nil {
			log.Fatalf("미리보기 실패: %v", err)
		}
		for _, t := range targets {
			fmt.Printf("%-28s | %-4s | %-14s | KLAS=%v RFID=%v\n",
				t.ShortName, t.Region, t.InspectionCycle, t.HasKlas, t.HasRfid)
		}
		fmt.Printf("추가 대상 %d개 기관 (미리보기, 저장하지 않음)\n", len(targets))
		return
	}

	res, err := repo.SyncSiteConfigsFromAssets()
	if err != nil {
		log.Fatalf("일괄 생성 실패: %v", err)
	}
	for _, n := range res.Names {
		fmt.Println("추가:", n)
	}
	fmt.Printf("추가 %d개 / 기존 유지 %d개\n", res.Added, res.Skipped)
}
