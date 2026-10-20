package repository

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestASSearchAndKnowledgeOrgScoped(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "as_org.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mediaOrg, err := NewOrgRepo(db).Create("미디어사업팀", "미디어", "", "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c1','가나도서관','가나도서관',1,?), ('c2','미디어고객','미디어고객',1,?)`, model.OrgIDLibrary, mediaOrg.OrgID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO assets (asset_id, customer_id, product_name) VALUES
		('A1','c1','무인예약기'), ('B1','c2','무인예약기')`); err != nil {
		t.Fatal(err)
	}

	repo := NewASRepo(db)
	proc := NewASProcessRepo(db)
	at := time.Date(2026, 3, 1, 10, 0, 0, 0, time.Local)
	libAS := &model.ASReceipt{
		CustomerID: "c1", AssetID: "A1", Symptom: "무인예약이 안 됩니다",
		ReceiptDatetime: at, Urgency: "normal", Priority: "normal", OrgID: model.OrgIDLibrary,
	}
	if err := repo.Create(libAS); err != nil {
		t.Fatal(err)
	}
	if err := proc.Create(&model.ASProcess{ASID: libAS.ASID, WorkContent: "단말기 재시작 후 정상 동작함", TimeSpent: 30}); err != nil {
		t.Fatal(err)
	}
	mediaAS := &model.ASReceipt{
		CustomerID: "c2", AssetID: "B1", Symptom: "무인예약이 안 됩니다",
		ReceiptDatetime: at, Urgency: "normal", Priority: "normal", OrgID: mediaOrg.OrgID,
	}
	if err := repo.Create(mediaAS); err != nil {
		t.Fatal(err)
	}
	if err := proc.Create(&model.ASProcess{ASID: mediaAS.ASID, WorkContent: "미디어 현장 재기동 후 정상", TimeSpent: 20}); err != nil {
		t.Fatal(err)
	}

	hits, total, err := repo.SearchAS(model.ASSearchFilter{Query: "무인예약", OrgID: model.OrgIDLibrary, Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(hits) != 1 || hits[0].ASID != libAS.ASID {
		t.Fatalf("library search total=%d hits=%v", total, hits)
	}

	sim, err := repo.SimilarCases(model.ASSimilarFilter{Query: "무인예약", OrgID: mediaOrg.OrgID, ExcludeID: "x", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(sim) != 1 || sim[0].ASID != mediaAS.ASID {
		t.Fatalf("media similar %+v", sim)
	}

	names, err := repo.SearchProductNamesInOrg(model.OrgIDLibrary)
	if err != nil || len(names) != 1 || names[0] != "무인예약기" {
		t.Fatalf("products %v err=%v", names, err)
	}
	sites, err := repo.KnowledgeSitesInOrg(mediaOrg.OrgID)
	if err != nil || len(sites) != 1 || sites[0].CustomerID != "c2" {
		t.Fatalf("sites %+v err=%v", sites, err)
	}

	_, err = repo.RebuildKeywordLinksInOrg(nil, model.OrgIDLibrary)
	if err != nil {
		t.Fatal(err)
	}
	kw := NewASKeywordRepo(db)
	list, err := kw.ListForOrg(true, model.OrgIDLibrary)
	if err != nil {
		t.Fatal(err)
	}
	var libN, mediaN int
	for _, k := range list {
		if k.Keyword == "무인예약" || k.Keyword == "재기동" {
			libN += k.LinkCount
		}
	}
	mlist, _ := kw.ListForOrg(true, mediaOrg.OrgID)
	for _, k := range mlist {
		mediaN += k.LinkCount
	}
	if repo.ReceiptCountInOrg(model.OrgIDLibrary) != 1 || repo.ReceiptCountInOrg(mediaOrg.OrgID) != 1 {
		t.Fatalf("receipt counts lib=%d media=%d", repo.ReceiptCountInOrg(model.OrgIDLibrary), repo.ReceiptCountInOrg(mediaOrg.OrgID))
	}
	if libN == 0 && mediaN == 0 {
		t.Fatal("org-scoped keyword link counts should see rebuilt links")
	}
}
