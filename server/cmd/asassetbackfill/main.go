// asassetbackfill 이미 만들어진 AS 업무에 접수 자산 연결이 없으면 잇는다.
//
//	미리보기: go run ./cmd/asassetbackfill -db data/app.db
//	반영:     go run ./cmd/asassetbackfill -db data/app.db -apply
//
// 기본은 쓰기 없이 건수만 보여 준다. 업무가 없는 접수는 만들지 않고 삭제하지도 않는다.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"customer-support/internal/repository"
)

func main() {
	dbPath := flag.String("db", "data/app.db", "SQLite 경로")
	apply := flag.Bool("apply", false, "생략하면 미리보기만 하고 쓰지 않는다")
	flag.Parse()

	fmt.Fprintln(os.Stderr, "적용 전에 DB 백업을 확인하세요")
	db, err := repository.InitDB(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	rep, err := repository.NewWBRepo(db).BackfillASTaskAssets(!*apply)
	if err != nil {
		log.Fatal(err)
	}
	mode := "미리보기"
	if *apply {
		mode = "반영"
	}
	fmt.Printf("%s: 연결할 업무 %d건, 이번에 연결 %d건, 이미 연결 %d건\n", mode, rep.Missing, rep.Linked, rep.AlreadyLinked)
	if rep.AlreadyLinked > 0 {
		fmt.Printf("건너뜀: 이미 접수 자산이 연결된 업무 %d건\n", rep.AlreadyLinked)
	}
	if len(rep.NoTask) == 0 {
		fmt.Println("자산은 있는데 업무가 없는 접수: 0건")
		return
	}
	fmt.Printf("자산은 있는데 업무가 없는 접수: %d건 (만들지 않음)\n", len(rep.NoTask))
	for _, row := range rep.NoTask {
		item := "작업항목 없음"
		if row.HasWorkItem {
			item = "작업항목 있음"
		}
		fmt.Printf("- %s 상태=%s 담당=%s %s 등록=%s\n", row.Number, row.Status, row.Assignee, item, row.ReceiptAt)
	}
}
