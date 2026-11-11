// asworkbackfill 이미 만들어진 AS 하위업무(재방문·확인·지원)에 일정 업무가 없으면 만든다.
//
//	미리보기: go run ./cmd/asworkbackfill -db data/app.db
//	반영:     go run ./cmd/asworkbackfill -db data/app.db -apply
//
// 기본은 쓰기 없이 건수만 보여 준다. -apply 를 적어야 한 트랜잭션으로 넣는다.
// 이미 일정 업무가 있는 건은 건너뛴다. 운영 반영은 이 도구를 배포한 뒤 직접 실행한다.
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

	rep, err := repository.NewWBRepo(db).BackfillASWorkTasks(!*apply)
	if err != nil {
		log.Fatal(err)
	}
	mode := "미리보기"
	if *apply {
		mode = "반영"
	}
	fmt.Printf("%s: 누락 %d건, 이번에 생성 %d건, 이미 있음 %d건, 건너뜀 %d건\n",
		mode, rep.Missing, rep.Created, rep.Already, rep.Skipped)
	for _, reason := range rep.Reasons {
		fmt.Printf("- %s\n", reason)
	}
}
