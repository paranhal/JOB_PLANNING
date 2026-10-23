package model

import (
	"bytes"
	"path/filepath"
	"strings"
)

const (
	SalesInitialAttachMaxBytes   = 20 << 20
	SalesInitialAttachMaxFiles   = 20
	SalesInitialAttachMaxRequest = 21 * SalesInitialAttachMaxBytes
	SalesInitialAttachField      = "initial_files"
)

var oleCompoundMagic = []byte{0xD0, 0xCF, 0x11, 0xE0}

func SalesInitialAttachExts() []string {
	return []string{"pdf", "doc", "docx", "xls", "xlsx", "hwp", "hwpx", "ppt", "pptx"}
}

func SalesAttachExt(name string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(strings.TrimSpace(name)), "."))
	return ext
}

func SalesAttachExtAllowed(name string) bool {
	ext := SalesAttachExt(name)
	for _, ok := range SalesInitialAttachExts() {
		if ext == ok {
			return true
		}
	}
	return false
}

func SalesAttachBadge(name string) string {
	switch SalesAttachExt(name) {
	case "docx":
		return "DOC"
	case "xlsx":
		return "XLS"
	case "pptx":
		return "PPT"
	case "hwpx":
		return "HWP"
	default:
		return strings.ToUpper(SalesAttachExt(name))
	}
}

func SalesAttachReject(name string, size int64, head []byte, existing int, seenNames map[string]bool) string {
	raw := strings.TrimSpace(name)
	if strings.Contains(raw, "..") || strings.ContainsAny(raw, `/\`) {
		return "파일 이름이 올바르지 않습니다."
	}
	base := filepath.Base(strings.ReplaceAll(raw, "\\", "/"))
	if base == "" || base == "." || base == ".." {
		return "파일 이름이 올바르지 않습니다."
	}
	if !SalesAttachExtAllowed(base) {
		return base + " — PDF, DOC, XLS, PPT, HWP 파일만 올릴 수 있습니다."
	}
	if size > SalesInitialAttachMaxBytes {
		return base + " — 20MB를 넘습니다 (" + FormatByteSize(size) + ")."
	}
	if existing >= SalesInitialAttachMaxFiles {
		return "사업당 파일은 최대 20개입니다."
	}
	key := strings.ToLower(base)
	if seenNames != nil && seenNames[key] {
		return base + " — 이미 첨부된 파일입니다."
	}
	if !SalesAttachMagicOK(SalesAttachExt(base), head) {
		return base + " — 파일 내용이 확장자와 다릅니다."
	}
	return ""
}

func SalesAttachMagicOK(ext string, head []byte) bool {
	ext = strings.ToLower(strings.TrimSpace(ext))
	if len(head) < 4 {
		return false
	}
	switch ext {
	case "pdf":
		return bytes.HasPrefix(head, []byte("%PDF"))
	case "docx", "xlsx", "pptx", "hwpx":
		return bytes.HasPrefix(head, []byte("PK"))
	case "doc", "xls", "ppt", "hwp":
		return bytes.HasPrefix(head, oleCompoundMagic)
	default:
		return false
	}
}

func SalesFormOpen02(formErr string) bool {
	s := strings.TrimSpace(formErr)
	if s == "" {
		return false
	}
	keys := []string{"고객", "기관명", "예상 금액", "견적 금액", "유입", "조달", "계약 대상", "계약 방법", "평가", "쇼핑몰", "예산"}
	for _, k := range keys {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

func SalesFormOpen03(formErr string) bool {
	s := strings.TrimSpace(formErr)
	if s == "" {
		return false
	}
	keys := []string{"영업담당", "영업 담당", "시기", "정밀", "입찰 시기", "매출 시기", "대금", "경쟁", "사업 그룹", "사업 대분류"}
	for _, k := range keys {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

type SalesFormStageHint struct {
	Desc string
	Need string
	Warn string
}

func SalesFormStageHintOf(code string) SalesFormStageHint {
	switch strings.TrimSpace(code) {
	case SalesStage4Discover:
		return SalesFormStageHint{
			Desc: "고객 니즈를 파악하고 사업 기회를 식별하는 단계입니다.",
			Need: "필수: 사업명 · 고객사 · 예산 · 유입 경로",
			Warn: "등록된 사업은 영업 파이프라인의 ‘발굴’ 단계에 추가됩니다. 수주 확정 전까지 수행 프로젝트는 생성되지 않습니다.",
		}
	case SalesStage4Propose:
		return SalesFormStageHint{
			Desc: "제안서를 준비·제출하고 고객 검토를 받는 단계입니다.",
			Need: "필수: RFP 접수일 · 제안 마감일 · 제안 담당",
			Warn: "‘제안’ 단계입니다. RFP 접수일을 넣으면 수주 가능성이 올라갑니다.",
		}
	case SalesStage4Bid:
		return SalesFormStageHint{
			Desc: "입찰·협상을 진행하며 수주 여부를 확정합니다.",
			Need: "필수: 입찰 상태 · 입찰 마감일 · 최종 수주 확률",
			Warn: "‘입찰’ 단계입니다. 입찰 상태와 마감일을 확인하세요.",
		}
	case SalesStage4Closed:
		return SalesFormStageHint{
			Desc: "계약 체결, 실주, 중단 중 하나로 영업을 마칩니다.",
			Need: "필수: 종료 사유 · 종료일",
			Warn: "이미 종료된 사업입니다. 계약으로 종료하면 사업관리로 전환할 수 있습니다.",
		}
	default:
		return SalesFormStageHint{}
	}
}

func SalesBidStatusLabel(code string) string {
	switch strings.TrimSpace(code) {
	case SalesBidPending:
		return "입찰 중"
	case SalesBidNegotiating:
		return "협상 중"
	case SalesBidWon:
		return "수주"
	default:
		return ""
	}
}

func SalesCloseReasonLabel(code string) string {
	switch strings.TrimSpace(code) {
	case SalesCloseContracted:
		return "계약"
	case SalesCloseLost:
		return "실주"
	case SalesCloseDropped:
		return "중단"
	default:
		return ""
	}
}

func SalesFormCollapsedSum(parts ...string) string {
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return "아직 입력하지 않음"
	}
	return strings.Join(out, " · ")
}
