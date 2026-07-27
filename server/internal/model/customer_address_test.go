package model

import (
	"strings"
	"testing"
)

func TestParseLegacyCustomerAddress(t *testing.T) {
	pc, sido, sigungu, dong, det := ParseLegacyCustomerAddress("(우)32255 충남 홍성군 홍북읍 선화로 22", "")
	if pc != "32255" || sido != "충청남도" || sigungu != "홍성군" || dong != "홍북읍" {
		t.Fatalf("got pc=%q sido=%q sigungu=%q dong=%q det=%q", pc, sido, sigungu, dong, det)
	}
	if !strings.Contains(det, "선화로") {
		t.Fatalf("detail should keep road: %q", det)
	}

	pc, sido, _, _, _ = ParseLegacyCustomerAddress("(30150)세종특별자치시 한누리대로 2023", "")
	if pc != "30150" || sido != "세종특별자치시" {
		t.Fatalf("sejong: pc=%q sido=%q", pc, sido)
	}
}
