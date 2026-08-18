package passwd

import (
	"strings"
	"testing"
)

func TestHashIsBcryptAndVerifies(t *testing.T) {
	h := Hash("admin")
	if !IsBcrypt(h) {
		t.Fatalf("bcrypt가 아님: %q", h)
	}
	if Hash("admin") == h {
		t.Fatal("같은 비밀번호라도 bcrypt 해시는 달라야 한다")
	}
	if !Verify(h, "admin") {
		t.Fatal("bcrypt 검증 실패")
	}
	if Verify(h, "wrong") {
		t.Fatal("틀린 비밀번호가 통과함")
	}
}

func TestVerifyLegacySHA256(t *testing.T) {
	legacy := SHA256Legacy("1234")
	if IsBcrypt(legacy) || !NeedsRehash(legacy) {
		t.Fatalf("레거시 판별 실패: %s", legacy)
	}
	if len(legacy) != 64 {
		t.Fatalf("SHA-256 hex 길이: %d", len(legacy))
	}
	if !Verify(legacy, "1234") {
		t.Fatal("구 해시 검증 실패")
	}
	if Verify(legacy, "admin") {
		t.Fatal("구 해시가 다른 비밀번호를 받음")
	}
}

func TestKnownRainbowSHA256StillVerifiesThenNeedsRehash(t *testing.T) {
	// 무염 SHA-256("admin") — 레인보우 테이블에 있는 값
	adminSHA := SHA256Legacy("admin")
	if adminSHA != "8c6976e5b5410415bde908bd4dee15dfb167a9c873fc4bb8a81f6f2ab448a918" {
		t.Fatalf("알려진 admin SHA-256과 다름: %s", adminSHA)
	}
	if !Verify(adminSHA, "admin") {
		t.Fatal("알려진 SHA-256을 검증하지 못함")
	}
	if !NeedsRehash(adminSHA) {
		t.Fatal("재해시 대상이어야 함")
	}
}

func TestEmptyRejected(t *testing.T) {
	if Verify("", "x") || Verify(Hash("x"), "") || Verify("  ", "x") {
		t.Fatal("빈 값 통과")
	}
	if IsBcrypt("") || NeedsRehash("") {
		t.Fatal("빈 해시는 bcrypt도 재해시 대상도 아님")
	}
	if !strings.HasPrefix(Hash("as-edit"), "$2") {
		t.Fatal("AS 수정 비번도 bcrypt")
	}
}
