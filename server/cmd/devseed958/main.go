package main

import (
	"fmt"
	"os"
	"time"

	"customer-support/internal/handler"
	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func main() {
	path := "/tmp/jobp-95-8-test.db"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	_ = os.Remove(path)
	db, err := repository.InitDB(path)
	if err != nil {
		panic(err)
	}
	defer db.Close()
	if err := repository.NewUserRepo(db).EnsureAdmin(handler.HashPassword("admin")); err != nil {
		panic(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c-958','테스트도서관','테스트도서관',1,'O01')`); err != nil {
		panic(err)
	}
	as := &model.ASReceipt{
		CustomerID:      "c-958",
		ReceiptChannel:  "phone",
		Requester:       "담당",
		Symptom:         "게이트 오작동 테스트",
		Urgency:         "normal",
		Priority:        "normal",
		AssignedTo:      "관리자",
		ReceiptDatetime: time.Now(),
		Status:          "in_progress",
		OrgID:           "O01",
	}
	if err := repository.NewASRepo(db).Create(as); err != nil {
		panic(err)
	}
	fmt.Println(as.ASID)
}
