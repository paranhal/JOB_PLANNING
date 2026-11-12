// utcshift133 과거에 UTC 로 저장된 조치 이력·메일·문자 발송함·업무 등록일·팀장 변경 이력의 시각을 한국 시각으로 +9시간 한다. 95-133.
//
//	미리보기: go run ./cmd/utcshift133 -db data/app.db -cutoff "2026-11-12 07:06"
//	반영:     go run ./cmd/utcshift133 -db data/app.db -cutoff "2026-11-12 07:06" -apply
//
// -cutoff 는 「저장 코드가 현지(KST) 시각으로 바뀐 빌드를 운영에 올려 처음 기동한 시각」(한국 시각)이다.
// 필수이며 기본값이 없다. 그 시각보다 이른 행만 +9시간 한다.
// 미리보기는 읽기 전용으로 DB를 열어 아무것도 쓰지 않는다. -apply 는 한 트랜잭션이고 한 번만 된다.
// 운영 반영은 DB 백업 뒤 사용자가 직접 실행한다.
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"

	"customer-support/internal/repository"
)

func main() {
	dbPath := flag.String("db", "data/app.db", "SQLite 경로")
	cutoff := flag.String("cutoff", "", "한국 시각 'YYYY-MM-DD HH:MM[:SS]'. 이보다 이른 행만 +9시간 (필수)")
	apply := flag.Bool("apply", false, "생략하면 미리보기만 하고 쓰지 않는다")
	detect := flag.Bool("detect", false, "cutoff 를 모를 때: 조치 이력 시각에서 +9시간 뛰는 지점(후보)을 찾는다. 읽기 전용·참고용")
	flag.Parse()

	if *detect {
		db, err := sql.Open("sqlite", *dbPath+"?mode=ro")
		if err != nil {
			log.Fatal(err)
		}
		defer db.Close()
		jumps, n, err := repository.DetectUTCShiftCutoff(db, 5)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("탐지(읽기 전용, 참고용): work_activities %d행을 입력 순서로 훑었습니다.\n", n)
		if len(jumps) == 0 {
			fmt.Println("+9시간 뛰는 지점이 없습니다. 아직 KST 저장이 시작되지 않았거나 자료가 적습니다. 배포 기록으로 정하세요.")
			return
		}
		for i, j := range jumps {
			fmt.Printf("후보 %d: %s %s → %s %s (차이 %s, 앞 %d행·뒤 %d행)\n  cutoff 후보: %q\n", i+1, j.PrevID, j.PrevAt, j.NextID, j.NextAt, j.Gap, j.Before, j.After, j.NextAt)
		}
		fmt.Println("퇴근~출근처럼 자연스럽게 9시간쯤 비는 구간도 후보로 나옵니다. 확정하지 말고 운영 /version 의 started(기동 시각)·배포 기록과 맞춰 보세요.")
		return
	}

	if *cutoff == "" {
		fmt.Fprintln(os.Stderr, "-cutoff 가 필요합니다. 저장 코드가 현지 시각으로 바뀐 빌드를 운영에 올려 처음 기동한 시각(한국 시각)입니다.")
		os.Exit(2)
	}
	dsn := *dbPath
	if !*apply {
		dsn += "?mode=ro"
	} else {
		fmt.Fprintln(os.Stderr, "적용 전에 DB 백업을 확인하세요")
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	rep, err := repository.ShiftUTCToKST(db, *cutoff, *apply)
	if err != nil {
		log.Fatal(err)
	}
	mode := "미리보기"
	if rep.Applied {
		mode = "반영"
	}
	fmt.Printf("%s: cutoff(한국 시각)=%s\n", mode, rep.Cutoff)
	if rep.FirstBootRef != "" {
		fmt.Printf("참고: app_settings.95_133_local_now_since=%s (95-161A 이후 첫 기동 시각. cutoff 와 다르면 cutoff 를 다시 확인)\n", rep.FirstBootRef)
	}
	for _, t := range rep.Tables {
		fmt.Printf("- %-16s %-9s 대상 %d건", t.Table, t.Column, t.Target)
		if t.Target > 0 {
			fmt.Printf(" (%s ~ %s), 그중 날짜가 바뀌는 건 %d건", t.Oldest, t.Newest, t.DateChanged)
		}
		fmt.Printf(", cutoff 이후 %d건은 그대로", t.After)
		if rep.Applied {
			fmt.Printf(", 이번에 +9시간 %d건", t.Shifted)
		}
		fmt.Println()
	}
	if !rep.Applied {
		fmt.Println("쓰지 않았습니다. 반영하려면 DB 백업 후 -apply 를 붙이세요.")
	}
}
