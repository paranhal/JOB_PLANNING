package model

import "testing"

func TestInferKBOrigin(t *testing.T) {
	if InferKBOrigin("", "새로 적음") != KBOriginAdded {
		t.Fatal("조치 없으면 added")
	}
	if InferKBOrigin("재시작 후 정상", "재시작 후 정상") != KBOriginFromAction {
		t.Fatal("같으면 from_action")
	}
	if InferKBOrigin("재시작 후 정상", "방화벽 예외 등록") != KBOriginRevised {
		t.Fatal("다르면 revised")
	}
}

func TestDisplayPersonUnknown(t *testing.T) {
	if DisplayPerson("") != KBAuthorUnknown || DisplayPerson("  ") != KBAuthorUnknown {
		t.Fatal("빈 이름은 작성자 미상")
	}
	if DisplayPerson("태자운") != "태자운" {
		t.Fatal("이름은 그대로")
	}
}
