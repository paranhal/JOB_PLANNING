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

	repo := NewSalesRepo(db)
	p := &model.SalesProject{Name: "이름만 바뀌는 건", IsTentativeName: true, Stage: model.SalesStageContact}
	if err := repo.Create(p); err != nil {
		t.Fatal(err)
	}
	before, err := repo.Get(p.SalesID)
	if err != nil {
		t.Fatal(err)
	}
	if before.Stage != model.SalesStageContact || before.Probability != 10 {
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
	if err := db.QueryRow(`SELECT COUNT(*) FROM sales_projects WHERE sales_id=? AND stage=?`, p.SalesID, model.SalesStageContact).Scan(&n); err != nil || n != 1 {
		t.Fatalf("stage 값이 바뀌었다 n=%d err=%v", n, err)
	}

	wantName := map[string]string{
		model.SalesStageContact:     "발굴",
		model.SalesStageLead:        "검토",
		model.SalesStageProposal:    "견적",
		model.SalesStageNegotiation: "협상",
		model.SalesStageWon:         "수주",
		model.SalesStageLost:        "실주",
	}
	wantProb := map[string]string{
		model.SalesStageContact: "10", model.SalesStageLead: "25", model.SalesStageProposal: "40",
		model.SalesStageNegotiation: "70", model.SalesStageWon: "100", model.SalesStageLost: "0",
	}
	stages, err := repo.Stages()
	if err != nil {
		t.Fatal(err)
	}
	if len(stages) != 6 {
		t.Fatalf("단계 수=%d", len(stages))
	}
	for _, st := range stages {
		if st.Label != wantName[st.Code] {
			t.Fatalf("%s 이름=%q want %q", st.Code, st.Label, wantName[st.Code])
		}
		var probName string
		if err := db.QueryRow(
			`SELECT code_name FROM codes WHERE code_group='sales_stage_prob' AND code_value=? AND is_active=1`,
			st.Code,
		).Scan(&probName); err != nil {
			t.Fatal(err)
		}
		if probName != wantProb[st.Code] {
			t.Fatalf("%s 확도 코드=%s want %s", st.Code, probName, wantProb[st.Code])
		}
		if st.Probability != atoiOr(wantProb[st.Code]) {
			t.Fatalf("%s 확도=%d", st.Code, st.Probability)
		}
	}
}

func atoiOr(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}
