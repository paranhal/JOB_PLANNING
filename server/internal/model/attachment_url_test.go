package model

import (
	"testing"
)

func TestAttachmentIsImageAndURLs(t *testing.T) {
	a := Attachment{
		FileName: "오류화면.HEIC",
		FilePath: "data/uploads/as/R1/receipt/123_0.jpg",
		MIMEType: "image/jpeg",
	}
	if !a.IsImage() {
		t.Fatal("jpeg mime")
	}
	if a.PublicURL() != "/uploads/as/R1/receipt/123_0.jpg" {
		t.Fatalf("public: %s", a.PublicURL())
	}
	if a.ThumbURL() != "/uploads/as/R1/receipt/123_0_thumb.jpg" {
		t.Fatalf("thumb: %s", a.ThumbURL())
	}
	pdf := Attachment{FileName: "접수서.pdf", FilePath: "data/uploads/as/R1/receipt/1_scan.pdf", MIMEType: "application/pdf"}
	if pdf.IsImage() {
		t.Fatal("pdf")
	}
	if pdf.ThumbURL() != "" {
		t.Fatal("pdf thumb")
	}
	heicName := Attachment{FileName: "a.heic", FilePath: "x", MIMEType: ""}
	if !heicName.IsImage() {
		t.Fatal("heic 확장자")
	}
}
