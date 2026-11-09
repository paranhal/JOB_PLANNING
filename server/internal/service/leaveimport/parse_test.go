package leaveimport

import (
	"strings"
	"testing"
)

func Test95_161D_ParseUsageAndSheet(t *testing.T) {
	us, errs := ParseUsageCell("3/5, 3/12(반차)", 2026)
	if len(errs) != 0 || len(us) != 2 {
		t.Fatalf("%v %v", us, errs)
	}
	if us[0].On != "2026-03-05" || us[0].Minutes != 480 || !us[0].Deduct {
		t.Fatalf("%+v", us[0])
	}
	if us[1].On != "2026-03-12" || us[1].Minutes != 240 {
		t.Fatalf("%+v", us[1])
	}
	us, errs = ParseUsageCell("4/1 예비군", 2026)
	if len(errs) != 0 || len(us) != 1 || us[0].Deduct || us[0].On != "2026-04-01" {
		t.Fatalf("%v %v", us, errs)
	}
	_, errs = ParseUsageCell("오전반차", 2026)
	if len(errs) == 0 {
		t.Fatal("want 확인 필요")
	}

	raw, err := BuildFixture(2026)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := Parse(raw, 2026)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]StaffRow{}
	for _, r := range rows {
		by[r.Name] = r
	}
	a := by["가상갑"]
	if a.GrantDays != 15 || a.MilitaryDays != 1 || a.RemainDays != 14.5 || len(a.Usages) != 2 {
		t.Fatalf("%+v", a)
	}
	b := by["가상을"]
	if b.GrantDays != 10 || len(b.Usages) != 1 || b.Usages[0].Deduct {
		t.Fatalf("%+v", b)
	}
	if !by["미등록자"].GrantEmpty {
		t.Fatal("empty grant")
	}
	if len(by["표기오류"].Errors) == 0 || !strings.Contains(strings.Join(by["표기오류"].Errors, ","), "확인 필요") {
		t.Fatalf("%v", by["표기오류"].Errors)
	}
}

func Test95_161D_WrongSheetRejected(t *testing.T) {
	fRaw, err := TemplateXLSX()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(fRaw, 2026); err != nil {
		t.Fatal(err)
	}
}
