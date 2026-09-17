package model

// MetricsPolicy app_settings 에서 읽은 지표 하한 (§4.5).
type MetricsPolicy struct {
	BaseDate string // YYYY-MM-DD. 비면 하한 없음
}
