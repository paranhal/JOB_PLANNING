package model

import "testing"

func TestWorkTaskActClass(t *testing.T) {
	if WorkTaskActClass(WBWorkAS) != ActFilterAS || WorkTaskActClass("replace") != ActFilterAS {
		t.Fatal("as")
	}
	if WorkTaskActClass(WBWorkMaintenance) != ActFilterMnt || WorkTaskActClass("visit") != ActFilterMnt {
		t.Fatal("mnt")
	}
	if WorkTaskActClass(WBWorkAdmin) != ActFilterTask || WorkTaskActClass("unknown") != ActFilterTask {
		t.Fatal("기타")
	}
	if WorkTaskActLabel("nope") != "기타 업무" {
		t.Fatal("label")
	}
}
