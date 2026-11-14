package handler

import (
	"net/http"
	"strings"
	"testing"

	"customer-support/internal/model"
)

// seed193Weekly 2026-10-19 주(전주 완료 구간 10-19~25)에 사업 미연결 업무를 만든다.
func seed193Weekly(t *testing.T, v env190) {
	t.Helper()
	db := v.h.Stats.repo.DB()
	for _, q := range []string{
		`INSERT INTO customers(customer_id,org_name,official_name,is_active) VALUES ('C193','제품학교','제품학교',1)`,
		`INSERT INTO assets(asset_id,customer_id,product_name,product_type,product_category,model_name) VALUES
			('A193-1','C193','전자도서관','SW','rfid','LX-1'),('A193-2','C193','무인대출반납기','HW','rfid','')`,
		`INSERT INTO maintenance_plans(plan_id,plan_year,title,status,org_id) VALUES ('MP193',2026,'t','approved','O03')`,
		`INSERT INTO as_receipts(as_id,as_number,customer_id,receipt_datetime,complete_datetime,status,org_id,asset_id) VALUES
			('AS193-A','R193A','C193','2026-10-20 10:00:00','2026-10-20 14:00:00','completed','O03','A193-1'),
			('AS193-N','R193N','C193','2026-10-20 10:00:00','2026-10-21 15:00:00','completed','O03',NULL)`,
		`INSERT INTO maintenance_visits(visit_id,plan_id,visit_date,customer_id,product_type,completed,completed_date,assignee) VALUES
			('V193-K','MP193','2026-10-20','C193','KLAS',1,'2026-10-20','담당')`,
		`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id,complete_date,work_date,source_type) VALUES
			('T193-M','admin','자산 있는 행정','complete',NULL,'O03','2026-10-21','2026-10-21',''),
			('T193-N','admin','자산 없는 행정','complete',NULL,'O03','2026-10-22','2026-10-22','')`,
		`INSERT INTO work_task_assets(task_id,asset_id) VALUES ('T193-M','A193-1'),('T193-M','A193-2')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
}

// block 「사업이 연결되지 않은 업무」 구역의 HTML.
func unassignedNoProjectBlock(t *testing.T, body string) string {
	t.Helper()
	i := strings.Index(body, "사업이 연결되지 않은 업무")
	if i < 0 {
		t.Fatalf("미연결 구역이 없다")
	}
	return body[i:]
}

func TestHTTP95_193_UnassignedShowsProductLabel(t *testing.T) {
	v := setup190(t, false)
	seed193Weekly(t, v)
	oa := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
	r := v.do(t, "GET", w190+"&edit=0", nil, oa)
	if r.Code != http.StatusOK {
		t.Fatalf("주간 보고서 %d", r.Code)
	}
	blk := unassignedNoProjectBlock(t, r.Body.String())
	for _, want := range []string{`data-product-label`, ">KLAS<", "전자도서관 · LX-1"} {
		if !strings.Contains(blk, want) {
			t.Fatalf("미연결 구역에 %q 없음", want)
		}
	}
	if !strings.Contains(blk, "외 1") {
		t.Fatal("자산이 여러 개인 행정 업무에 「외 1」이 없다")
	}
	// 제품을 알 수 없는 건(AS 자산 없음, 자산 없는 행정)은 라벨을 달지 않는다: 그 줄에 data-product-label 이 없다
	for _, title := range []string{"자산 없는 행정", "R193N"} {
		if i := strings.Index(blk, title); i >= 0 {
			line := blk[max(0, i-400):i]
			if k := strings.LastIndex(line, "<li"); k >= 0 && strings.Contains(line[k:], "data-product-label") {
				t.Fatalf("%s 에 제품 라벨이 붙었다", title)
			}
		}
	}
	// 연결 저장 경로는 그대로(사업 연결 링크·data-kind/data-id)
	if !strings.Contains(blk, `data-as-project-link="1"`) || !strings.Contains(blk, `data-kind="work_task" data-id="T193-M"`) {
		t.Fatal("연결 링크·데이터 속성이 바뀌었다")
	}
}
