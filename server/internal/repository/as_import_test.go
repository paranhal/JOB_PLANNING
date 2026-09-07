package repository

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestASImportGateAndCancel(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "imp.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES (?,?,?,1)`,
		"c1", "가나도서관", "가나도서관"); err != nil {
		t.Fatal(err)
	}
	repo := NewASImportRepo(db)

	csv := "거래처명,접수일자,내용,답변 및 처리\n" +
		"가나도서관,2024-06-01,팝업 오류입니다,재시작했습니다\n" +
		"없는기관,2024-06-02,접속 안 됨,확인\n" +
		"가나도서관,1999-01-01,옛날 증상,조치\n"
	drafts, err := ParseASImportFile("as.csv", []byte(csv))
	if err != nil {
		t.Fatal(err)
	}
	b, err := repo.CreateBatch("as.csv", "tester", drafts)
	if err != nil {
		t.Fatal(err)
	}
	var nAS int
	_ = db.QueryRow(`SELECT COUNT(*) FROM as_receipts`).Scan(&nAS)
	if nAS != 0 {
		t.Fatalf("업로드만으로 반영되면 안 됨 n=%d", nAS)
	}
	if b.Status != "reported" || b.ErrorRows < 2 {
		t.Fatalf("검증 리포트: status=%s err=%d", b.Status, b.ErrorRows)
	}

	if _, err := repo.ApplyBatch(b.BatchID); err == nil {
		t.Fatal("오류가 남은 채 반영되면 안 된다")
	}

	fake := "IMP999"
	if _, err := db.Exec(`INSERT INTO as_import_batches(batch_id, filename, status, created_at) VALUES (?,?,?,?)`,
		fake, "x.csv", "uploaded", "2026-01-01 00:00:00"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ApplyBatch(fake); err == nil || !strings.Contains(err.Error(), "검증 리포트") {
		t.Fatalf("검증 없이 반영: %v", err)
	}

	rows, _ := repo.ListRows(b.BatchID)
	for _, row := range rows {
		if row.HasBlocker() {
			if err := repo.SetExcluded(b.BatchID, row.RowNo, true); err != nil {
				t.Fatal(err)
			}
		}
	}
	n, err := repo.ApplyBatch(b.BatchID)
	if err != nil || n != 1 {
		t.Fatalf("반영 n=%d err=%v", n, err)
	}
	var origin, batchCol string
	var confirmed int
	var visit, reason string
	if err := db.QueryRow(`SELECT data_origin, COALESCE(import_batch_id,''), schedule_confirmed,
		COALESCE(visit_scheduled_date,''), COALESCE(schedule_no_date_reason,'')
		FROM as_receipts WHERE import_batch_id=?`, b.BatchID).
		Scan(&origin, &batchCol, &confirmed, &visit, &reason); err != nil {
		t.Fatal(err)
	}
	if origin != "import" || batchCol != b.BatchID {
		t.Fatalf("origin=%s batch=%s", origin, batchCol)
	}
	if confirmed == 1 && visit == "" {
		t.Fatal("예정일 없이 확정하면 안 된다")
	}
	if visit == "" && reason == "" {
		t.Fatal("예정일 없으면 사유가 있어야 한다")
	}
	var links int
	_ = db.QueryRow(`SELECT COUNT(*) FROM as_keyword_links l
		JOIN as_receipts a ON a.as_id=l.as_id WHERE a.import_batch_id=?`, b.BatchID).Scan(&links)
	if links != 0 {
		t.Fatalf("키워드를 확정하면 안 된다 n=%d", links)
	}
	sug, err := NewASKeywordRepo(db).Suggest("팝업 오류입니다")
	if err != nil {
		t.Fatal(err)
	}
	got := false
	for _, k := range sug {
		if k.Keyword == "팝업" {
			got = true
		}
	}
	if !got {
		t.Fatal("반입 건에도 후보(팝업)는 나와야 한다")
	}

	gone, err := repo.CancelBatch(b.BatchID)
	if err != nil || gone != 1 {
		t.Fatalf("일괄 취소 n=%d err=%v", gone, err)
	}
	_ = db.QueryRow(`SELECT COUNT(*) FROM as_receipts WHERE import_batch_id=?`, b.BatchID).Scan(&nAS)
	if nAS != 0 {
		t.Fatalf("취소 후 접수가 남음 n=%d", nAS)
	}
}

func TestASImportParseOmitsApply(t *testing.T) {
	rows, err := ParseASImportFile("x.csv", []byte("거래처명,접수일자,내용\n도서관,2024-05-01,무인예약이 안 됩니다\n"))
	if err != nil || len(rows) != 1 {
		t.Fatalf("parse n=%d err=%v", len(rows), err)
	}
	if rows[0].CustomerRaw != "도서관" || rows[0].ReceiptDate != "2024-05-01" {
		t.Fatalf("%+v", rows[0])
	}
}
