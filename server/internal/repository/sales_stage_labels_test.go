package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestSalesStageLabelsRenameKeepsCodesAndProbability(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "sales_labels.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	reopenSalesLegacyMigrations(db)
	applySalesStagesV227(db)
	applySalesStageLabels(db)
	LoadLookupCache(db)

	repo := NewSalesRepo(db)
	p := &model.SalesProject{Name: "이름만 바뀌는 건", IsTentativeName: true}
	if err := repo.Create(p); err != nil {
		t.Fatal(err)
	}
	before, err := repo.Get(p.SalesID)
	if err != nil {
		t.Fatal(err)
	}
	if before.Stage != model.SalesStage4Discover || before.Probability != 10 {
		t.Fatalf("저장 후 stage=%s prob=%d", before.Stage, before.Probability)
	}

	applySalesStageLabels(db)

	after, err := repo.Get(p.SalesID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Stage != before.Stage || after.Probability != before.Probability {
		t.Fatalf("코드/확도가 바뀌었다 stage %s→%s prob %d→%d", before.Stage, after.Stage, before.Probability, after.Probability)
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sales_projects WHERE sales_id=? AND stage=?`, p.SalesID, model.SalesStage4Discover).Scan(&n); err != nil || n != 1 {
		t.Fatalf("stage 값이 바뀌었다 n=%d err=%v", n, err)
	}

	wantName := map[string]string{
		model.SalesStage4Discover: "발굴",
		model.SalesStage4Propose:  "제안",
		model.SalesStage4Bid:      "입찰",
		model.SalesStage4Closed:   "사업 종료",
	}
	stages, err := repo.Stages()
	if err != nil {
		t.Fatal(err)
	}
	if len(stages) != 4 {
		t.Fatalf("단계 수=%d", len(stages))
	}
	for _, st := range stages {
		if st.Label != wantName[st.Code] {
			t.Fatalf("%s 이름=%q want %q", st.Code, st.Label, wantName[st.Code])
		}
	}
}
