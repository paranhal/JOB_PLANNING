package model

// MetricsPolicy app_settings 에서 읽은 지표 하한 (§4.5 · §68.3).
type MetricsPolicy struct {
	BaseDate     string // 옛 키. 셋의 초기값·되돌릴 근거
	BaseReceipt  string
	BaseVisit    string
	BaseComplete string
}
