package model

import "testing"

func TestBuildASConclusionDraft(t *testing.T) {
	got := BuildASConclusionDraft("전원 불량", "부팅 불가", "파워 교체")
	want := "전원 불량으로 부팅 불가 문제가 발생했고, 파워 교체하여 정상작동하는 것으로 확인됨."
	if got != want {
		t.Fatalf("got %q", got)
	}
	ro := BuildASConclusionDraft("케이블 단선", "통신 끊김", "케이블 재접속")
	if ro != "케이블 단선으로 통신 끊김 문제가 발생했고, 케이블 재접속하여 정상작동하는 것으로 확인됨." {
		t.Fatalf("받침 으로: %q", ro)
	}
	noJong := BuildASConclusionDraft("설정 오류", "로그인 실패", "설정 복구")
	if noJong != "설정 오류로 로그인 실패 문제가 발생했고, 설정 복구하여 정상작동하는 것으로 확인됨." {
		t.Fatalf("받침 없음 로: %q", noJong)
	}
	if BuildASConclusionDraft("", "", "") != "" {
		t.Fatal("전부 빈 초안은 없어야 한다")
	}
}
