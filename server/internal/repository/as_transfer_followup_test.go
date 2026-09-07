package repository

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestListReopensExcludesTransferFollowup(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "tf.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('C1','가나도서관','가나도서관',1)`); err != nil {
		t.Fatal(err)
	}
	repo := NewASRepo(db)
	src := &model.ASReceipt{
		CustomerID: "C1", Symptom: "게이트", ReceiptChannel: "phone",
		Requester: "홍길동", ReceiptDatetime: time.Now(),
	}
	if err := repo.Create(src); err != nil {
		t.Fatal(err)
	}
	child := model.NewTransferFollowupReceipt(src, "추가 확인", time.Now())
	if err := repo.Create(child); err != nil {
		t.Fatal(err)
	}
	if reopens, _ := repo.ListReopens(src.ASID); len(reopens) != 0 {
		t.Fatalf("이관후속이 재접수 목록에 있다: %+v", reopens)
	}
	follow, err := repo.ListTransferFollowups(src.ASID)
	if err != nil || len(follow) != 1 || follow[0].ASID != child.ASID {
		t.Fatalf("이관후속 목록: %+v err=%v", follow, err)
	}
	got, _ := repo.GetByID(child.ASID)
	if got == nil || got.FollowupNote != "추가 확인" || got.IsReopen {
		t.Fatalf("GetByID followup: %+v", got)
	}
	items, _, err := repo.ListFiltered("", "", "", nil, "", "", 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	var listed *model.ASListItem
	for i := range items {
		if items[i].ASID == child.ASID {
			listed = &items[i]
			break
		}
	}
	if listed == nil || !listed.IsTransferFollowup() || listed.IsReopen {
		t.Fatalf("목록 뱃지: %+v", listed)
	}
}
