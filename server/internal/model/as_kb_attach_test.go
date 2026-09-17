package model

import "testing"

func TestLooksLikeVideoOnlyThreeExt(t *testing.T) {
	if !LooksLikeVideo("a.MP4", "application/octet-stream") {
		t.Fatal("mp4")
	}
	if !LooksLikeVideo("clip.mov", "") || !LooksLikeVideo("x.webm", "") {
		t.Fatal("mov/webm")
	}
	if LooksLikeVideo("clip.avi", "video/x-msvideo") {
		t.Fatal("avi 는 문서")
	}
	if LooksLikeVideo("shot.jpg", "video/mp4") {
		t.Fatal("확장자가 우선")
	}
}

func TestSizeLimitMessage(t *testing.T) {
	got := SizeLimitMessage(MaxKBVideoBytes, 82<<20)
	if got != "50MB 까지 올릴 수 있습니다 (지금 82MB)" {
		t.Fatalf("%q", got)
	}
	got = SizeLimitMessage(MaxKBPhotoBytes, 12<<20)
	if got != "10MB 까지 올릴 수 있습니다 (지금 12MB)" {
		t.Fatalf("%q", got)
	}
}

func TestKBAttachReject(t *testing.T) {
	if msg := KBAttachReject("a.mp4", "", 82<<20, 0); msg != SizeLimitMessage(MaxKBVideoBytes, 82<<20) {
		t.Fatalf("video over: %q", msg)
	}
	if msg := KBAttachReject("a.mp4", "", 1024, 2); msg != "동영상은 한 건에 2개까지입니다" {
		t.Fatalf("count: %q", msg)
	}
	if msg := KBAttachReject("a.jpg", "", 11<<20, 0); msg != SizeLimitMessage(MaxKBPhotoBytes, 11<<20) {
		t.Fatalf("photo over: %q", msg)
	}
	if msg := KBAttachReject("a.mp4", "", 1024, 1); msg != "" {
		t.Fatalf("ok: %q", msg)
	}
}
