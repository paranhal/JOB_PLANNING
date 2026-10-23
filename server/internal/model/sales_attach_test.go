package model

import (
	"strings"
	"testing"
)

func TestSalesAttachRejectAndMagic(t *testing.T) {
	pdf := []byte("%PDF-1.4")
	if msg := SalesAttachReject("rfp.pdf", 100, pdf, 0, nil); msg != "" {
		t.Fatal(msg)
	}
	if msg := SalesAttachReject("note.exe", 10, []byte("MZ"), 0, nil); msg == "" || !strings.Contains(msg, "PDF") {
		t.Fatalf("exe: %s", msg)
	}
	if msg := SalesAttachReject("page.html", 10, []byte("<html>"), 0, nil); msg == "" {
		t.Fatal("html")
	}
	if msg := SalesAttachReject("fake.pdf", 20, []byte("<html>"), 0, nil); !strings.Contains(msg, "내용") {
		t.Fatalf("위장: %s", msg)
	}
	if msg := SalesAttachReject("big.pdf", SalesInitialAttachMaxBytes+1, pdf, 0, nil); !strings.Contains(msg, "20MB") {
		t.Fatalf("크기: %s", msg)
	}
	if msg := SalesAttachReject("a.pdf", 10, pdf, SalesInitialAttachMaxFiles, nil); !strings.Contains(msg, "20개") {
		t.Fatalf("개수: %s", msg)
	}
	seen := map[string]bool{"rfp.pdf": true}
	if msg := SalesAttachReject("RFP.PDF", 10, pdf, 1, seen); !strings.Contains(msg, "이미") {
		t.Fatalf("중복: %s", msg)
	}
	if msg := SalesAttachReject("../secret.pdf", 10, pdf, 0, nil); !strings.Contains(msg, "이름") {
		t.Fatalf("경로: %s", msg)
	}
	if SalesAttachMagicOK("docx", []byte("PK\x03\x04")) == false {
		t.Fatal("ooxml")
	}
	if SalesAttachMagicOK("doc", oleCompoundMagic) == false {
		t.Fatal("ole")
	}
}

func TestSalesFormOpenCards(t *testing.T) {
	if SalesFormOpen02("") || SalesFormOpen03("") {
		t.Fatal("빈 오류")
	}
	if !SalesFormOpen02("유입 경로를 확인하세요") {
		t.Fatal("02")
	}
	if !SalesFormOpen03("영업담당을 선택하세요") {
		t.Fatal("03")
	}
	if SalesFormOpen02("사업명을 입력하세요") {
		t.Fatal("01은 접힌 카드가 아님")
	}
}

func TestSalesFormCollapsedSum(t *testing.T) {
	if SalesFormCollapsedSum("", "  ") != "아직 입력하지 않음" {
		t.Fatal("empty")
	}
	if SalesFormCollapsedSum("에이펙스", "", "1.8억원") != "에이펙스 · 1.8억원" {
		t.Fatal("join")
	}
}
