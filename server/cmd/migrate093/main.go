// migrate093 설치자산 관리유형·미계약 CALL 일괄 정리. 적용 전 DB 를 백업하고 CSV 리포트를 쓴다.
//
//	go run ./cmd/migrate093 -db data/app.db -out output
//	go run ./cmd/migrate093 -db data/app.db -out output -apply
//	go run ./cmd/migrate093 -revert -from output/migration_093_changes.csv -db data/app.db
package main

import (
	"flag"
	"fmt"
	"log"
	"sort"

	"customer-support/internal/repository"
)

func main() {
	dbPath := flag.String("db", "data/app.db", "SQLite 파일")
	outDir := flag.String("out", "output", "리포트 디렉터리")
	apply := flag.Bool("apply", false, "백업 후 DB에 적용 (없으면 미리보기)")
	revert := flag.Bool("revert", false, "변경 CSV의 이전값으로 복구")
	from := flag.String("from", "output/migration_093_changes.csv", "-revert 시 읽을 CSV")
	flag.Parse()

	db, err := repository.InitDB(*dbPath)
	if err != nil {
		log.Fatalf("DB 열기: %v", err)
	}
	defer db.Close()

	if *revert {
		n, err := repository.RevertAssetMgmtFromCSV(db, *from)
		if err != nil {
			log.Fatalf("되돌리기: %v", err)
		}
		fmt.Printf("되돌림 %d건 (%s)\n", n, *from)
		return
	}

	var bak string
	if *apply {
		bak, err = repository.BackupAppDB(*dbPath)
		if err != nil {
			log.Fatalf("백업 실패: %v", err)
		}
		fmt.Println("백업:", bak)
	} else {
		fmt.Println("미리보기입니다. 코드를 고치고 CSV 를 쓰려면 -apply 를 붙이세요. (지금도 계산·리포트는 동일 함수)")
	}

	res, err := repository.ApplyAssetMgmtTypesV93(db, *apply)
	if err != nil {
		log.Fatalf("적용: %v", err)
	}
	res.BackupPath = bak
	if err := repository.WriteMigration093Reports(*outDir, res); err != nil {
		log.Fatalf("리포트: %v", err)
	}
	printCounts("적용 전 관리유형", res.MgmtBefore)
	printCounts("적용 후 관리유형", res.MgmtAfter)
	printCounts("미계약 점검주기(후)", res.NoneCycleAfter)
	fmt.Printf("변경 %d건 → %s/migration_093_changes.csv\n", len(res.Changes), *outDir)
	fmt.Printf("미입력 유지 %d건 → %s/migration_093_unresolved.csv\n", len(res.Unresolved), *outDir)
	printCounts("모순 조합(참고, 미변경)", res.Contradict)
	if bak != "" {
		fmt.Println("되돌리기: 백업 파일을 app.db 로 복사하거나 -revert")
	}
}

func printCounts(title string, m map[string]int) {
	fmt.Println(title)
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("  %s\t%d\n", k, m[k])
	}
}
