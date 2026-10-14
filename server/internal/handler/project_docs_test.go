package handler

import (
	"net/http"
	"strings"
	"testing"
)

func TestProjectDocsFigmaLayout(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("PROJECT_EXAMPLE_DATA", "1")
	e, _, _ := newProjectServer(t)
	page := doGet(t, e, "/projects/docs")
	if page.Code != http.StatusOK {
		t.Fatalf("status=%d", page.Code)
	}
	body := page.Body.String()
	for _, want := range []string{
		"서류 작성·관리",
		"임시저장",
		"검토 요청",
		"제출",
		"D-2026-104",
		"맑음 · 18~24℃",
		"작성 김현우",
		"검토 박준석 · 대기",
		"승인 이정민 · 대기",
		"사업 선택",
		"A동 골조공사",
		"데이터센터 네트워크 구축",
		"ERP 연동 플랫폼 개발",
		"충남세종 RFID 자동화 장비 유지보수",
		"AS 조치 보고서",
		"사업 유형 / 단계 선택",
		"이어 쓰기",
		"단계별 서류 목록",
		"공정 착수 승인서",
		"공정 착수 확인서",
		"공정 진행 보고서",
		"상세 확인",
		"선택된 서류 작성 폼",
		"편집 가능",
		"파일 내보내기",
		"상태 변경",
		"연관 단계 일정",
		"담당자 및 검토",
		"문서 본문",
		"A동 8층 슬래브 타설",
		"화면 설계를 위한 예시",
		"견적서",
	} {
		if want == "견적서" {
			if strings.Contains(body, want) {
				t.Fatal("이전 바로가기 카드가 남아 있다")
			}
			continue
		}
		if !strings.Contains(body, want) {
			t.Fatalf("화면에 %q 없음", want)
		}
	}
	if strings.Contains(body, "data-node-id") {
		t.Fatal("Figma 원본 흔적이 렌더에 남았다")
	}
}
