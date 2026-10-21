package repository

// orgScopeAllowlist 는 92단계 TestOrgScopeStaticAudit 예외. 함수명 → 사유 한 줄.
var orgScopeAllowlist = map[string]string{
	"SearchWorkTaskCustomers": "95-9A 작업 대상 사이트는 조직별 관리가 아니다. 활성 고객 전체 검색, 응답은 기관명·상위기관·지역만.",
	"LookupForRegister":       "AS 등록 고객 조회. 호출부가 현재 조직 고객만 넘긴다.",
	"searchASFTS":             "AS 전문검색. 상위 MatchIDs가 조직 범위를 적용한다.",
	"searchASLike":            "AS LIKE 검색. 상위 MatchIDs가 조직 범위를 적용한다.",
	"activitySearchLike":      "영업활동 검색 LIKE. 상위 쿼리가 조직 범위를 적용한다.",
}
