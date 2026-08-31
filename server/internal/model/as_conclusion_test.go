package model

import (
	"strings"
	"testing"
)

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

func TestBuildASConclusionDraftForResultPartialPrefix(t *testing.T) {
	base := BuildASConclusionDraft("설정 오류", "로그인 실패", "설정 복구")
	got := BuildASConclusionDraftForResult(ResultPartial, "설정 오류", "로그인 실패", "설정 복구")
	want := PartialConclusionPrefix + base
	if got != want {
		t.Fatalf("partial 초안=%q want %q", got, want)
	}
	if !strings.HasPrefix(got, "(부분 조치)") {
		t.Fatalf("머리말 없음: %q", got)
	}
	done := BuildASConclusionDraftForResult(ResultDone, "설정 오류", "로그인 실패", "설정 복구")
	if done != base {
		t.Fatalf("완료 초안이 바뀜: %q", done)
	}
	if BuildASConclusionDraftForResult(ResultPartial, "", "", "") != "" {
		t.Fatal("빈 초안에 머리말만 붙으면 안 된다")
	}
	if BuildASConclusionDraftForResult(ResultRevisit, "설정 오류", "로그인 실패", "설정 복구") != base {
		t.Fatal("재방문은 초안 문장만 두고 머리말은 붙이지 않는다")
	}
}

func TestASConclusionDraftFromUsesActionThenProcess(t *testing.T) {
	as := &ASReceipt{Symptom: "부팅 불가", CauseDetail: "전원 불량", ActionTaken: "파워 교체", ResultCode: ResultDone}
	got := ASConclusionDraftFrom(as, nil)
	want := BuildASConclusionDraft("전원 불량", "부팅 불가", "파워 교체")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	as.ActionTaken = ""
	procs := []ASProcess{{WorkContent: "1차"}, {WorkContent: "파워 교체"}}
	got = ASConclusionDraftFrom(as, procs)
	if got != want {
		t.Fatalf("이력 마지막 작업: %q", got)
	}
	if ASConclusionDraftFrom(nil, nil) != "" {
		t.Fatal("nil 접수는 빈 초안")
	}
}

func TestShowsASCauseReport(t *testing.T) {
	if !ShowsASCauseReport(ResultDone) || !ShowsASCauseReport(ResultPartial) {
		t.Fatal("완료·추가조치 필요는 열려야 한다")
	}
	for _, code := range []string{ResultRevisit, ResultTransfer, ResultHold, ""} {
		if ShowsASCauseReport(code) {
			t.Fatalf("%q 는 닫혀야 한다", code)
		}
	}
}
