package model

import "testing"

func TestResolveActionContent(t *testing.T) {
	if _, code := ResolveActionContent("", false, ""); code != ActionErrRequired {
		t.Fatalf("빈 조치: %s", code)
	}
	if _, code := ResolveActionContent("  ", false, ""); code != ActionErrRequired {
		t.Fatalf("공백 조치: %s", code)
	}
	got, code := ResolveActionContent("", true, "원격만 안내")
	if code != "" || got != ActionNAPrefix+" 원격만 안내" {
		t.Fatalf("해당 없음: %q %s", got, code)
	}
	if _, code := ResolveActionContent("x", true, ""); code != ActionErrNAReason {
		t.Fatalf("해당 없음 사유 없음: %s", code)
	}
}

func TestActionContentTooShort(t *testing.T) {
	if ActionContentTooShort("1234567890") {
		t.Fatal("10자는 짧지 않다")
	}
	if !ActionContentTooShort("현장 점검") {
		t.Fatal("5자는 짧다")
	}
	if ActionContentTooShort(ActionNAPrefix + " 사유") {
		t.Fatal("해당 없음은 길이 검사를 건너뛴다")
	}
}
