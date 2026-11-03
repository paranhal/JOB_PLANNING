package repository

import (
	"os"
	"strings"
	"testing"
)

func TestAppDBPerfTestsHaveBuildTag(t *testing.T) {
	b, err := os.ReadFile("work_today_perf_test.go")
	if err != nil {
		t.Fatal(err)
	}
	head := string(b)
	if i := strings.Index(head, "package "); i > 0 {
		head = head[:i]
	}
	if !strings.Contains(head, "go:build appdb") && !strings.Contains(head, "+build appdb") {
		t.Fatal("운영 DB 복사본 테스트는 //go:build appdb 여야 한다")
	}
}
