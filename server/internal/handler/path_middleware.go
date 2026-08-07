package handler

import (
	"strings"

	"github.com/labstack/echo/v4"
)

// DecodePathMiddleware 라우팅 전에 경로를 디코딩된 형태로 고정한다.
//
// 자산번호처럼 한글이 들어간 ID는 URL에서 퍼센트 인코딩된다. Go는 인코딩이
// 표준형(대문자 16진수)과 다를 때만 URL.RawPath를 채우는데, Echo는 RawPath가
// 있으면 그 값으로 라우팅하고 경로 파라미터도 인코딩된 상태로 넘긴다.
// 그 결과 소문자 인코딩(%ec…)으로 들어온 요청만 조회에 실패한다.
// RawPath를 비우면 Echo가 항상 디코딩된 URL.Path를 쓰게 된다.
//
// 세그먼트 안에 인코딩된 슬래시(%2F)가 있으면 경로가 나뉘지만, 이 시스템의
// ID 체계에는 슬래시가 없다.
func DecodePathMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		r := c.Request()
		if r.URL.RawPath != "" && !strings.Contains(r.URL.RawPath, "%2F") && !strings.Contains(r.URL.RawPath, "%2f") {
			r.URL.RawPath = ""
		}
		return next(c)
	}
}
