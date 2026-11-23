package repository

import (
	"strings"

	"customer-support/internal/model"
)

// IndustryContainsSQL 95-219: 업종 열 col 이 업종 key 를 「포함」하는가의 SQL 조각과 인자.
// 저장값은 고정 순서 쉼표(예: 「도서관,학교」)지만 옛 값(별칭·슬래시)도 읽도록 구분자를 쉼표로 맞춘 뒤 쉼표 경계로 비교한다
// (「공공」이 「공공기관」에 걸리지 않는다). key 가 비었거나 「업종 미지정」이면 업종이 비어 있는 조건이다.
// 고객 목록 필터·후보 조회·정기점검 일괄·월 대상 등 업종 조회는 모두 이 함수를 쓴다.
func IndustryContainsSQL(col, key string) (string, []any) {
	k := model.IndustryKey(key)
	if k == model.IndustryUnspecified {
		return `TRIM(COALESCE(` + col + `,''))=''`, nil
	}
	raws := model.IndustryRawValues(k)
	if len(raws) == 0 {
		return `1=0`, nil
	}
	norm := `(',' || REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(COALESCE(` + col + `,''),' ',''),'/',','),'，',','),'、',','),';',',') || ',')`
	parts := make([]string, 0, len(raws))
	args := make([]any, 0, len(raws))
	for _, v := range raws {
		if strings.TrimSpace(v) == "" {
			continue
		}
		parts = append(parts, norm+` LIKE ?`)
		args = append(args, "%,"+strings.ReplaceAll(v, " ", "")+",%")
	}
	if len(parts) == 0 {
		return `1=0`, nil
	}
	return `(` + strings.Join(parts, ` OR `) + `)`, args
}

// splitIndustryCounts 업종별 건수 집계: 여러 업종 고객은 각 업종에 1번씩 센다. 업종이 비면 model.IndustryUnspecified.
func addIndustryCounts(counts map[string]int, order *[]string, raw string, n int) {
	list := model.ParseIndustries(raw)
	if len(list) == 0 {
		list = []string{model.IndustryUnspecified}
	}
	for _, k := range list {
		if _, ok := counts[k]; !ok {
			*order = append(*order, k)
		}
		counts[k] += n
	}
}
