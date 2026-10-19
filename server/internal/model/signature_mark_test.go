package model

import (
	"bytes"
	"image"
	"image/png"
	"testing"
)

func TestSignatureOrNameFallsBack(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	good := SignatureOrName(buf.Bytes(), "홍길동", "hong", "U1")
	if len(good.PNG) == 0 || good.Name != "홍길동" {
		t.Fatalf("%+v", good)
	}
	if got := SignatureOrName(nil, "홍길동", "hong", "U1"); len(got.PNG) != 0 || got.Name != "홍길동" {
		t.Fatalf("미등록 %+v", got)
	}
	if got := SignatureOrName([]byte("not-an-image"), "홍길동", "hong", "U1"); len(got.PNG) != 0 || got.Name != "홍길동" {
		t.Fatalf("손상 %+v", got)
	}
	if got := SignatureOrName(nil, "  ", "hong", "U1"); got.Name != "hong" {
		t.Fatalf("아이디 대체 %q", got.Name)
	}
	if got := SignatureOrName(nil, "", "", "U1"); got.Name != "U1" {
		t.Fatalf("user id %q", got.Name)
	}
	if StampNameClass("Alexandria") != "text-ui-xs" || StampNameClass("남궁민수") != "text-ui-sm" || StampNameClass("김가") != "text-ui" {
		t.Fatalf("class %s %s %s", StampNameClass("Alexandria"), StampNameClass("남궁민수"), StampNameClass("김가"))
	}
}
