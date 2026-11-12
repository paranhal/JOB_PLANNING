package repository

import (
	"testing"

	"customer-support/internal/model"
)

// 하부 업무 배지: 사이트에서 그 하부 업무가 완료되면 배지가 붙고, 여러 하부 업무가 끝나면 배지가 여러 개 쌓인다. 95-179D.
func TestSiteBadgesByCustomerAndSummary(t *testing.T) {
	fx := sitesFixture(t)
	repo, db := fx.repo, fx.repo.db
	mk := func(name, badge string) string {
		t.Helper()
		id, err := repo.CreatePackage(fx.phase, name, model.TrackModeSite)
		if err != nil {
			t.Fatal(err)
		}
		if badge != "" {
			if err := repo.UpdatePackageMeta(fx.pid, id, PackageMeta{Badge: badge}, DateChangeCtx{}); err != nil {
				t.Fatal(err)
			}
		}
		return id
	}
	gb := mk("GBIC 수량 산정2", "사전GB")
	fw := mk("방화벽/L2 SW 수량 산정", "사전FW")
	plain := mk("배지 없는 업무", "")
	gb2 := mk("GBIC 보조", "사전GB") // 같은 배지 이름을 다른 하부 업무가 가져도 된다
	siteHS, siteCA, siteYE := fx.ids["홍성고등학교"], fx.ids["천안고등학교"], fx.ids["예산중학교"]
	if _, err := repo.AddPhaseSites(fx.pid, fx.phase, []string{siteHS, siteCA, siteYE}, false, "", "", "u1"); err != nil {
		t.Fatal(err)
	}
	for _, pk := range []string{gb, fw, plain, gb2} {
		if _, err := repo.AddPackageSites(fx.pid, pk, []string{siteHS, siteCA, siteYE}, false, "", "", "u1"); err != nil {
			t.Fatal(err)
		}
	}
	done := func(pkg, site, date string) {
		t.Helper()
		if _, err := db.Exec(`UPDATE package_sites SET status='done', updated_at=? WHERE package_id=? AND project_site_id=?`, date+" 10:00:00", pkg, site); err != nil {
			t.Fatal(err)
		}
	}
	// 홍성고: GB 먼저(11-03), 며칠 뒤 FW(11-08), 배지 없는 업무와 같은 배지의 보조 업무도 완료
	done(gb, siteHS, "2026-11-03")
	done(fw, siteHS, "2026-11-08")
	done(plain, siteHS, "2026-11-09")
	done(gb2, siteHS, "2026-11-10")
	// 천안고: FW 만. 예산중: 아직 아무것도 완료 안 됨(진행 중만)
	done(fw, siteCA, "2026-11-08")
	if _, err := db.Exec(`UPDATE package_sites SET status='in_progress' WHERE package_id=? AND project_site_id=?`, gb, siteYE); err != nil {
		t.Fatal(err)
	}
	// 완료 이벤트가 있으면 그 일자를 쓴다(updated_at 보다 우선)
	if _, err := db.Exec(`INSERT INTO package_events(event_id, package_id, package_site_id, plan_id, event_date, event_type, qty_delta, user_id, work_task_id, source, note, created_at)
		SELECT 'EV-BADGE1', package_id, package_site_id, '', '2026-11-02', 'complete', 0, 'u1', '', 'work', '', '2026-11-02 09:00:00' FROM package_sites WHERE package_id=? AND project_site_id=?`, gb, siteHS); err != nil {
		t.Fatal(err)
	}

	m, err := repo.SiteBadgesByCustomer(fx.pid)
	if err != nil {
		t.Fatal(err)
	}
	hs := m["C-hsh"]
	if len(hs) != 2 || hs[0].Label != "사전GB" || hs[1].Label != "사전FW" {
		t.Fatalf("홍성고 배지(완료 순, 같은 이름 한 번, 배지 없는 업무 제외): %+v", hs)
	}
	if hs[0].Date != "2026-11-02" || hs[0].Tone != model.BadgeTone("사전GB") || hs[1].Date != "2026-11-08" || hs[1].Package != "방화벽/L2 SW 수량 산정" {
		t.Fatalf("홍성고 배지 상세: %+v", hs)
	}
	if ca := m["C-cah"]; len(ca) != 1 || ca[0].Label != "사전FW" {
		t.Fatalf("천안고 배지: %+v", ca)
	}
	if len(m["C-yem"]) != 0 {
		t.Fatalf("완료 안 된 사이트에 배지가 붙었다: %+v", m["C-yem"])
	}
	// 완료를 되돌리면(진행 중) 배지가 사라진다
	if _, err := db.Exec(`UPDATE package_sites SET status='in_progress' WHERE package_id=? AND project_site_id=?`, fw, siteCA); err != nil {
		t.Fatal(err)
	}
	if m, _ = repo.SiteBadgesByCustomer(fx.pid); len(m["C-cah"]) != 0 {
		t.Fatal("완료가 아니면 배지가 없어야 한다")
	}

	// 사업별 요약: 배지가 붙은 사이트 수, 많은 순
	if _, err := db.Exec(`UPDATE package_sites SET status='done', updated_at='2026-11-11 10:00:00' WHERE package_id=? AND project_site_id=?`, fw, siteYE); err != nil {
		t.Fatal(err)
	}
	sum, err := repo.BadgeSummary([]string{fx.pid, "WPT"})
	if err != nil {
		t.Fatal(err)
	}
	got := sum[fx.pid]
	if len(got) != 2 || got[0].Label != "사전FW" || got[0].Count != 2 || got[1].Label != "사전GB" || got[1].Count != 1 {
		t.Fatalf("배지 요약: %+v", got)
	}
	if len(sum["WPT"]) != 0 {
		t.Fatal("다른 사업")
	}
}
