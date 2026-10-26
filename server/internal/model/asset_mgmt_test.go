package model

import "testing"

func TestMapBulkManagementTypeTable(t *testing.T) {
	contracts := []string{"paid", "free", "none", ""}
	for _, c := range contracts {
		if got := MapBulkManagementType("direct", c); got != ManagementTypePeriodic {
			t.Errorf("direct × %q → %q", c, got)
		}
		if got := MapBulkManagementType("on_demand", c); got != ManagementTypePeriodic {
			t.Errorf("on_demand × %q → %q", c, got)
		}
		if got := MapBulkManagementType("reference", c); got != ManagementTypePeriodic {
			t.Errorf("reference × %q → %q", c, got)
		}
	}
	if got := MapBulkManagementType("", "paid"); got != ManagementTypePeriodic {
		t.Fatalf("미입력×paid %q", got)
	}
	if got := MapBulkManagementType("", "free"); got != ManagementTypePeriodic {
		t.Fatalf("미입력×free %q", got)
	}
	if got := MapBulkManagementType("", "none"); got != ManagementTypeFault {
		t.Fatalf("미입력×none %q", got)
	}
	if got := MapBulkManagementType("", ""); got != "" {
		t.Fatalf("미입력×빈계약 %q", got)
	}
	if got := MapBulkManagementType("fault", "paid"); got != "fault" {
		t.Fatalf("모순 조합은 유지 %q", got)
	}
	if got := MapBulkManagementType("periodic", "none"); got != "periodic" {
		t.Fatalf("periodic+none 유지 %q", got)
	}
	if got := MapBulkManagementType("third_party", ""); got != "third_party" {
		t.Fatalf("타사장비 유지 %q", got)
	}
}

func TestMapBulkMaintCycleMatrix(t *testing.T) {
	cycles := []string{"Call ", "call", "수시", "", "monthly"}
	contracts := []string{"none", "paid", "free", "", "call"}
	for _, c := range contracts {
		for _, cy := range cycles {
			got := MapBulkMaintCycle(c, cy)
			if c == "none" {
				if cy == "" {
					if got != "" {
						t.Errorf("none×빈값 → %q", got)
					}
				} else if got != MaintCycleCall {
					t.Errorf("none×%q → %q", cy, got)
				}
			} else if got != cy {
				t.Errorf("%s×%q 는 그대로여야 함 → %q", c, cy, got)
			}
		}
		if got := MapBulkMaintCycle(c, ""); c == "none" {
			if got != "" {
				t.Errorf("none NULL/빈 %q", got)
			}
		}
	}
}

func TestCanonicalAndDisplayCall(t *testing.T) {
	if CanonicalMaintCycleCode("CALL") != "call" || CanonicalMaintCycleCode(" Call ") != "call" {
		t.Fatal("canonical call")
	}
	if MaintCycleDisplayLabel("Call ") != "CALL" || MaintCycleDisplayLabel("call") != "CALL" {
		t.Fatal("display CALL")
	}
	if MaintCycleDisplayLabel("monthly") != "월" {
		t.Fatal("월")
	}
}

func TestApplyNoneContractCycleDefault(t *testing.T) {
	a := &Asset{MaintContractType: "none"}
	ApplyNoneContractCycleDefault(a)
	if a.MaintCycle != "call" {
		t.Fatalf("빈 주기 채움 %q", a.MaintCycle)
	}
	b := &Asset{MaintContractType: "none", MaintCycle: "monthly"}
	ApplyNoneContractCycleDefault(b)
	if b.MaintCycle != "monthly" {
		t.Fatalf("직접 선택 유지 %q", b.MaintCycle)
	}
	c := &Asset{MaintContractType: "paid"}
	ApplyNoneContractCycleDefault(c)
	if c.MaintCycle != "" {
		t.Fatalf("유상은 채우지 않음 %q", c.MaintCycle)
	}
}

func TestIsSelectableManagementType(t *testing.T) {
	if !IsSelectableManagementType("fault") || IsSelectableManagementType("direct") || IsSelectableManagementType("") {
		t.Fatal("selectable")
	}
}
