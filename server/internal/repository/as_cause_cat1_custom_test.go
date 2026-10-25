package repository

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestASCauseCat1CustomMigrationIdempotentAndCount(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "c44.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ApplyASCauseCat1CustomV88ForTest(db)
	ApplyASCauseCat1CustomV88ForTest(db)
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('c1','가','가',1)`); err != nil {
		t.Fatal(err)
	}
	as := NewASRepo(db)
	now := time.Now()
	r1 := &model.ASReceipt{CustomerID: "c1", Symptom: "s1", ReceiptDatetime: now, Status: "in_progress", OrgID: model.OrgIDLibrary}
	if err := as.Create(r1); err != nil {
		t.Fatal(err)
	}
	got, _ := as.GetByID(OrgAll, r1.ASID)
	got.CauseCat1 = model.CauseCatCustom
	got.CauseCat1Custom = "CCTV"
	got.ActionTaken = "점검"
	if err := as.Update(got); err != nil {
		t.Fatal(err)
	}
	r2 := &model.ASReceipt{CustomerID: "c1", Symptom: "s2", ReceiptDatetime: now, Status: "in_progress", OrgID: model.OrgIDLibrary}
	if err := as.Create(r2); err != nil {
		t.Fatal(err)
	}
	got2, _ := as.GetByID(OrgAll, r2.ASID)
	got2.CauseCat1 = model.CauseCatCustom
	got2.CauseCat1Custom = "cctv"
	got2.ActionTaken = "조치"
	_ = as.Update(got2)
	r3 := &model.ASReceipt{CustomerID: "c1", Symptom: "s3", ReceiptDatetime: now, Status: "in_progress", OrgID: model.OrgIDLibrary}
	if err := as.Create(r3); err != nil {
		t.Fatal(err)
	}
	got3, _ := as.GetByID(OrgAll, r3.ASID)
	got3.CauseCat1 = "rfid"
	got3.CauseCat2 = "rfid.hw"
	_ = as.Update(got3)

	recent, err := as.ListCauseCat1CustomRecent(model.OrgIDLibrary, 20)
	if err != nil || len(recent) < 1 {
		t.Fatalf("recent %v %v", recent, err)
	}
	counts, err := as.CountCauseCat1(model.OrgIDLibrary)
	if err != nil {
		t.Fatal(err)
	}
	var cctv, rfid int
	for _, c := range counts {
		if strings.EqualFold(c.Label, "CCTV") || strings.EqualFold(c.Label, "cctv") {
			cctv += c.N
		}
		if c.Key == "rfid" {
			rfid = c.N
		}
	}
	if cctv != 2 {
		t.Fatalf("cctv n=%d counts=%+v", cctv, counts)
	}
	if rfid != 1 {
		t.Fatalf("rfid n=%d", rfid)
	}
	hits, _, err := as.SearchAS(model.ASSearchFilter{Query: "s", CauseCat: model.CauseCatCustom, OrgID: model.OrgIDLibrary, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) < 2 {
		t.Fatalf("search custom n=%d", len(hits))
	}
	for _, h := range hits {
		if !strings.EqualFold(h.CauseName, "CCTV") && !strings.EqualFold(h.CauseName, "cctv") {
			t.Fatalf("label %q", h.CauseName)
		}
	}
}
