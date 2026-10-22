package repository

// orgScopeAllowlist 는 92단계 TestOrgScopeStaticAudit 예외. 함수명 → 사유 한 줄.
var orgScopeAllowlist = map[string]string{
	"SearchWorkTaskCustomers":      "95-9A 작업 대상 사이트는 조직별 관리가 아니다. 활성 고객 전체 검색, 응답은 기관명·상위기관·지역만.",
	"SearchCustomersWithoutConfig": "95-9A Q1. 점검 사이트 고객 검색도 조직별 관리가 아니다. 미설정·활성 고객 전체.",
	"lookupCustomerName":           "전사 공용 마스터 — 95-2E. 사업 엑셀 업로드는 활성 고객 전체에서 이름 조회, 사업 부서 소속 1건이면 우선.",
	"List":                         "전사 공용 마스터 — 95-2E",
	"listProjects":                 "사업 전 직원 조회 — 95-2E. 쓰기는 GetVisible(write). 집계는 ListContractsInOrg.",
	"LookupForRegister":            "AS 등록 고객 조회. 호출부가 현재 조직 고객만 넘긴다.",
	"searchASFTS":                  "AS 전문검색. 상위 MatchIDs가 조직 범위를 적용한다.",
	"searchASLike":                 "AS LIKE 검색. 상위 MatchIDs가 조직 범위를 적용한다.",
	"activitySearchLike":           "영업활동 검색 LIKE. 상위 쿼리가 조직 범위를 적용한다.",
	"ListWorkPickProjects":         "95-9A/95-12 작업·일일업무·AS 사업 선택 목록. 조직과 무관하게 취소·종료가 아닌 계약 사업.",
}
