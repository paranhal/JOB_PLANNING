package model

import "testing"

func TestNextChildSeqSkipsDeleted(t *testing.T) {
	parent := "WT-113"
	if n := NextChildSeq(parent, nil); n != 1 {
		t.Fatalf("empty=%d", n)
	}
	if n := NextChildSeq(parent, []string{"WT-113-1", "WT-113-2"}); n != 3 {
		t.Fatalf("two siblings=%d", n)
	}
	// 1번이 삭제돼도 2가 있으면 다음은 3
	if n := NextChildSeq(parent, []string{"WT-113-2"}); n != 3 {
		t.Fatalf("gap=%d", n)
	}
	if n := ChildSeqOf(parent, "WT-113-1-1"); n != 0 {
		t.Fatalf("grandchild counted as sibling: %d", n)
	}
	if id := ChildTaskID(parent, 1); id != "WT-113-1" {
		t.Fatalf("id=%s", id)
	}
}

func TestCanAddSubtaskUnderDepth(t *testing.T) {
	p := WorkTask{TaskID: "WT-001"}
	if err := CanAddSubtaskUnder(p, 1); err != nil {
		t.Fatal(err)
	}
	if err := CanAddSubtaskUnder(p, 2); err != nil {
		t.Fatal(err)
	}
	if err := CanAddSubtaskUnder(p, 3); err != ErrSubtaskDepth {
		t.Fatalf("depth 3: %v", err)
	}
	p.RecurrenceRole = RecurrenceRoleOccurrence
	if err := CanAddSubtaskUnder(p, 1); err != ErrSubtaskOccur {
		t.Fatalf("occurrence: %v", err)
	}
}
