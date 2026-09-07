package model

import "testing"

func TestProcessTypesForWorkPlace(t *testing.T) {
	office := ProcessTypesForWorkPlace(WorkPlaceOffice)
	if len(office) != 2 || office[0] != ProcessTypeRemote || office[1] != ProcessTypeInquiry {
		t.Fatalf("내근: %v", office)
	}
	field := ProcessTypesForWorkPlace(WorkPlaceField)
	if len(field) != 2 || field[0] != ProcessTypeVisit || field[1] != ProcessTypeInquiry {
		t.Fatalf("외근: %v", field)
	}
	if ProcessTypesForWorkPlace("") != nil {
		t.Fatal("미선택은 비어야 한다")
	}
	if ProcessTypeAllowed(WorkPlaceOffice, ProcessTypeVisit) {
		t.Fatal("내근+현장방문은 안 된다")
	}
	if !ProcessTypeAllowed(WorkPlaceOffice, ProcessTypeRemote) || !ProcessTypeAllowed(WorkPlaceField, ProcessTypeVisit) {
		t.Fatal("허용 조합이 거절됐다")
	}
	if ProcessTypePlaces(ProcessTypeRemote) != WorkPlaceOffice {
		t.Fatal(ProcessTypePlaces(ProcessTypeRemote))
	}
	if ProcessTypePlaces(ProcessTypeInquiry) != WorkPlaceOffice+" "+WorkPlaceField {
		t.Fatal(ProcessTypePlaces(ProcessTypeInquiry))
	}
}
