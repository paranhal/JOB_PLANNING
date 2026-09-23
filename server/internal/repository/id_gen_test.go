package repository

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExtractAreaCode(t *testing.T) {
	cases := map[string]string{
		"02-1234-5678": "02",
		"041-550-1621": "041",
		"044-301-7061": "044",
		"":             "000",
		"1234":         "000",
	}
	for in, want := range cases {
		if got := ExtractAreaCode(in); got != want {
			t.Fatalf("ExtractAreaCode(%q)=%q want %q", in, got, want)
		}
	}
}

func TestIDFormats(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	cid, err := NextCustomerID(db, "041-550-1621")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(cid, "C041-") || !reCustomerIDV2.MatchString(cid) {
		t.Fatalf("customer id: %s", cid)
	}

	aid, err := NextAssetID(db, "EZ-200E2", "", "hw", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(aid, "AEZ-200E23-") {
		t.Fatalf("asset id: %s", aid)
	}

	at := time.Date(2026, 7, 15, 10, 0, 0, 0, time.Local)
	asNum, err := NextASNumber(db, at)
	if err != nil {
		t.Fatal(err)
	}
	if asNum != "R2607-001" {
		t.Fatalf("as number: %s", asNum)
	}

	s1, err := NextSalesNo(db, at)
	if err != nil {
		t.Fatal(err)
	}
	s2, err := NextSalesNo(db, at)
	if err != nil {
		t.Fatal(err)
	}
	s3, err := NextSalesNo(db, time.Date(2026, 8, 1, 0, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	if s1 != "S2607-001" || s2 != "S2607-002" || s3 != "S2608-001" {
		t.Fatalf("sales no: %s %s %s", s1, s2, s3)
	}

	p1, err := NextProcessNumber(db, asNum, at)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := NextProcessNumber(db, asNum, at)
	if err != nil {
		t.Fatal(err)
	}
	if p1 != "R2607-001-P01" || p2 != "R2607-001-P02" {
		t.Fatalf("process: %s %s", p1, p2)
	}
}
