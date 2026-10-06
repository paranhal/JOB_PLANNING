package handler

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompanyWeeklyDownloadFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	persistCompanyWeeklyFile(dir, "tok-abc", companyWeeklyDownload{
		Data: []byte("xlsx-bytes"), ASCIIName: "a.xlsx", UTF8Name: "주간.xlsx",
	})
	if _, err := os.Stat(filepath.Join(dir, "uploads", "company_weekly", "tok-abc.xlsx")); err != nil {
		t.Fatal(err)
	}
	item, ok := loadCompanyWeeklyFile(dir, "tok-abc")
	if !ok || string(item.Data) != "xlsx-bytes" || item.UTF8Name != "주간.xlsx" {
		t.Fatalf("%+v ok=%v", item, ok)
	}
	if _, ok := loadCompanyWeeklyFile(dir, "../x"); ok {
		t.Fatal("경로 탈출을 막아야 한다")
	}
}
