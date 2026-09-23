package model

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

var salesLogFieldLabels = map[string]string{
	"stage": "단계", "bid_status": "입찰 상태", "close_reason": "종료 사유",
	"win_prob": "수주 확도", "probability": "확도", "probability_final": "확정 확도",
	"rfp_received_at": "RFP 접수일", "name": "사업명", "expected_amount": "금액",
	"expected_ym": "사업 시기", "sales_owner": "담당", "status": "상태",
	"lost_reason": "실주 사유", "notes": "메모", "won_at": "수주일",
	"contracted_at": "계약일", "contract_amount": "계약금액", "awarded_amount": "낙찰금액",
	"bid_eval_method": "평가방법", "biz_type": "사업 유형", "budget_year": "예산 연도",
	"budget_status": "예산 상태", "dormant_until": "휴면 해제일", "dormant_reason": "휴면 사유",
	"bid_ym": "입찰 시기", "revenue_ym": "매출 시기", "revenue_from": "대금 시작",
	"revenue_to": "대금 종료", "billing_cycle": "대금 주기", "amount_vat_included": "VAT 포함",
	"activity_type": "활동 유형", "activity_date": "활동일", "title": "제목",
	"content": "내용", "quote_no": "견적번호", "rev": "개정", "rev_reason": "개정 사유",
	"vat_mode": "VAT", "total": "합계", "subtotal": "공급가",
	"job_name": "직무", "monthly": "월임금", "year": "연도",
}

var salesLogCodeLabels = map[string]string{
	"discover": "발굴", "propose": "제안", "bid": "입찰", "closed": "사업 종료",
	"pending": "결과 대기", "won": "수주", "negotiating": "협상 중",
	"contracted": "계약", "lost": "실주", "dropped": "포기",
	"active": "진행", "dormant": "휴면",
	"included": "VAT 포함", "excluded": "VAT 별도",
	"research": "정보 입수", "call": "전화", "visit": "방문", "online": "온라인",
	"mail": "메일", "requirement": "요구사항 파악", "proposal": "제안",
	"quote": "견적", "rfp_received": "RFP 접수", "rfp": "제안서 제출",
	"once": "일시", "month": "월", "quarter": "분기", "half": "반기", "year": "연",
}

type SalesLogDiffLine struct {
	Field string
	From  string
	To    string
}

func SalesLogFieldLabel(key string) string {
	if s, ok := salesLogFieldLabels[key]; ok {
		return s
	}
	return key
}

func SalesLogValueLabel(v string) string {
	v = strings.TrimSpace(v)
	if s, ok := salesLogCodeLabels[v]; ok {
		return s
	}
	return v
}

func DiffSalesLogJSON(before, after string) []SalesLogDiffLine {
	bm := parseJSONMap(before)
	am := parseJSONMap(after)
	keys := map[string]struct{}{}
	for k := range bm {
		keys[k] = struct{}{}
	}
	for k := range am {
		keys[k] = struct{}{}
	}
	var names []string
	for k := range keys {
		if k == "updated_at" || k == "created_at" {
			continue
		}
		names = append(names, k)
	}
	sort.Strings(names)
	var out []SalesLogDiffLine
	for _, k := range names {
		fv, tv := fmtJSONVal(bm[k]), fmtJSONVal(am[k])
		if fv == tv {
			continue
		}
		out = append(out, SalesLogDiffLine{
			Field: SalesLogFieldLabel(k),
			From:  SalesLogValueLabel(fv),
			To:    SalesLogValueLabel(tv),
		})
	}
	return out
}

func parseJSONMap(s string) map[string]any {
	s = strings.TrimSpace(s)
	if s == "" || s == "{}" {
		return map[string]any{}
	}
	var m map[string]any
	if json.Unmarshal([]byte(s), &m) != nil {
		return map[string]any{}
	}
	return m
}

func fmtJSONVal(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%g", t)
	case json.Number:
		return t.String()
	case bool:
		if t {
			return "1"
		}
		return "0"
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return fmt.Sprint(t)
		}
		return string(b)
	}
}
