package repository

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLogSlowSQLRecordsOver200ms(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	logSlowSQL("work.queryAS", time.Now().Add(-50*time.Millisecond))
	if strings.Contains(buf.String(), "slow sql") {
		t.Fatal("50ms 조회가 slow sql 로 남았다")
	}

	logSlowSQL("work.queryAS", time.Now().Add(-250*time.Millisecond))
	got := buf.String()
	if !strings.Contains(got, "slow sql") || !strings.Contains(got, "work.queryAS") {
		t.Fatalf("200ms 초과 로그 없음: %s", got)
	}
}
