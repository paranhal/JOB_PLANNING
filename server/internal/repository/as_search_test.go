package repository

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func setupASSearch(t *testing.T) (*ASRepo, *ASProcessRepo) {
	t.Helper()
	db, err := InitDB(filepath.Join(t.TempDir(), "as_search.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES (?,?,?,1)`,
		"c1", "가나도서관", "가나도서관"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES (?,?,?,1)`,
		"c2", "다라도서관", "다라도서관"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO assets (asset_id, customer_id, product_name) VALUES ('A1','c1','무인예약기')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO assets (asset_id, customer_id, product_name) VALUES ('A2','c1','KLAS')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO assets (asset_id, customer_id, product_name) VALUES ('B1','c2','무인예약기')`); err != nil {
		t.Fatal(err)
	}
	return NewASRepo(db), NewASProcessRepo(db)
}

func createAS(t *testing.T, repo *ASRepo, customerID, assetID, symptom string, at time.Time) *model.ASReceipt {
	t.Helper()
	as := &model.ASReceipt{
		CustomerID: customerID, AssetID: assetID, Symptom: symptom,
		ReceiptDatetime: at, Urgency: "normal", Priority: "normal",
	}
	if err := repo.Create(as); err != nil {
		t.Fatal(err)
	}
	return as
}

func TestASSearchParticleAndSnippets(t *testing.T) {
	repo, proc := setupASSearch(t)
	if !repo.SearchUsesFTS() {
		t.Fatal("InitDB 후 FTS5+trigram 색인이 있어야 한다")
	}

	old := createAS(t, repo, "c1", "A1", "무인예약이 안 됩니다", time.Date(2020, 3, 1, 10, 0, 0, 0, time.Local))
	if err := proc.Create(&model.ASProcess{ASID: old.ASID, WorkContent: "단말기 재시작 후 정상", TimeSpent: 30}); err != nil {
		t.Fatal(err)
	}
	none := createAS(t, repo, "c1", "A2", "홈페이지 팝업 오류", time.Date(2026, 8, 20, 10, 0, 0, 0, time.Local))

	hits, total, err := repo.SearchAS(model.ASSearchFilter{Query: "무인예약", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if total < 1 {
		t.Fatal("무인예약 검색이 「무인예약이 안 됩니다」에 걸려야 한다")
	}
	var found *model.ASSearchHit
	for i := range hits {
		if hits[i].ASID == old.ASID {
			found = &hits[i]
			break
		}
	}
	if found == nil {
		t.Fatal("기준일 이전(2020) 건이 검색되어야 한다")
	}
	if !strings.Contains(string(found.SymptomHTML), "<mark>무인예약</mark>") {
		t.Fatalf("증상 강조 없음: %s", found.SymptomHTML)
	}
	if !found.HasAction || found.Action == "" || found.Action == model.ASActionMissing {
		t.Fatalf("조치가 함께 보여야 한다: %+v", found)
	}
	if !strings.Contains(found.Action, "재시작") {
		t.Fatalf("조치 발췌: %s", found.Action)
	}

	hits2, _, err := repo.SearchAS(model.ASSearchFilter{Query: "팝업", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	var noneHit *model.ASSearchHit
	for i := range hits2 {
		if hits2[i].ASID == none.ASID {
			noneHit = &hits2[i]
			break
		}
	}
	if noneHit == nil {
		t.Fatal("조치 없는 건도 증상으로 찾아져야 한다")
	}
	if noneHit.HasAction || noneHit.Action != model.ASActionMissing {
		t.Fatalf("조치 기록 없음이어야 함: has=%v action=%s", noneHit.HasAction, noneHit.Action)
	}
	if !strings.Contains(string(noneHit.ActionHTML), model.ASActionMissing) {
		t.Fatalf("ActionHTML=%s", noneHit.ActionHTML)
	}
}

func completeWithWork(t *testing.T, repo *ASRepo, proc *ASProcessRepo, as *model.ASReceipt, work string, done time.Time) {
	t.Helper()
	if err := proc.Create(&model.ASProcess{ASID: as.ASID, WorkContent: work, TimeSpent: 20}); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetByID(as.ASID)
	if err != nil {
		t.Fatal(err)
	}
	got.Status = "completed"
	got.CompleteDatetime = &done
	if err := repo.Update(got); err != nil {
		t.Fatal(err)
	}
}

func TestASSimilarSameAssetFirstAndReopen(t *testing.T) {
	repo, proc := setupASSearch(t)
	work := "단말기 전원 재인가 후 정상동작"
	same := createAS(t, repo, "c1", "A1", "무인예약이 안 됩니다", time.Date(2026, 1, 10, 10, 0, 0, 0, time.Local))
	completeWithWork(t, repo, proc, same, work, time.Date(2026, 1, 14, 16, 0, 0, 0, time.Local))

	sameSite := createAS(t, repo, "c1", "A2", "무인예약 오류입니다", time.Date(2026, 2, 10, 10, 0, 0, 0, time.Local))
	completeWithWork(t, repo, proc, sameSite, "설정 확인 후 재시작했습니다", time.Date(2026, 2, 11, 16, 0, 0, 0, time.Local))

	otherOrg := createAS(t, repo, "c2", "B1", "무인예약이 느립니다", time.Date(2026, 3, 10, 10, 0, 0, 0, time.Local))
	completeWithWork(t, repo, proc, otherOrg, "네트워크 점검 후 재기동완료", time.Date(2026, 3, 12, 16, 0, 0, 0, time.Local))

	noWork := createAS(t, repo, "c1", "A1", "무인예약이 또 멈춤", time.Date(2026, 4, 1, 10, 0, 0, 0, time.Local))
	short := createAS(t, repo, "c1", "A1", "무인예약 재부팅", time.Date(2026, 4, 2, 10, 0, 0, 0, time.Local))
	if err := proc.Create(&model.ASProcess{ASID: short.ASID, WorkContent: "재부팅", TimeSpent: 5}); err != nil {
		t.Fatal(err)
	}
	self := createAS(t, repo, "c1", "A1", "무인예약이 또 안 됩니다", time.Date(2026, 4, 10, 10, 0, 0, 0, time.Local))

	items, err := repo.SimilarCases(model.ASSimilarFilter{
		Query: "무인예약이 또 안 됩니다", CustomerID: "c1", AssetID: "A1", ExcludeID: self.ASID, Limit: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 {
		t.Fatal("비슷한 사례가 나와야 한다")
	}
	if !items[0].SameCustomer || items[0].WeightLabel != "같은 사이트" {
		t.Fatalf("같은 사이트가 위: %+v", items[0])
	}
	seenSelf, seenNo, seenShort := false, false, false
	var sameHit *model.ASSimilarCase
	lastSame := true
	for i, it := range items {
		if it.ASID == self.ASID {
			seenSelf = true
		}
		if it.ASID == noWork.ASID {
			seenNo = true
		}
		if it.ASID == short.ASID {
			seenShort = true
		}
		if it.ASID == same.ASID {
			cp := it
			sameHit = &cp
		}
		if !it.HasAction || strings.TrimSpace(it.ActionSummary) == "" || it.ActionSummary == model.ASActionMissing {
			t.Fatalf("조치 없는 건: %+v", it)
		}
		if i > 0 && it.SameCustomer && !lastSame {
			t.Fatal("같은 사이트 건이 다른 사이트보다 뒤에 있다")
		}
		lastSame = it.SameCustomer
	}
	if seenSelf {
		t.Fatal("자기 자신은 빼야 한다")
	}
	if seenNo || seenShort {
		t.Fatal("조치 없거나 10자 미만은 빼야 한다")
	}
	if sameHit == nil {
		t.Fatal("같은 자산 완료 건이 있어야 한다")
	}
	if !sameHit.CanReopen {
		t.Fatal("완료+같은 자산이면 재접수를 제안해야 한다")
	}
}

func TestASSimilarHidesWhenNoMatch(t *testing.T) {
	repo, _ := setupASSearch(t)
	_ = createAS(t, repo, "c1", "A1", "게이트 오작동", time.Now())
	items, err := repo.SimilarCases(model.ASSimilarFilter{Query: "전혀다른증상XYZ", CustomerID: "c1", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("없으면 빈 목록: n=%d", len(items))
	}
}
