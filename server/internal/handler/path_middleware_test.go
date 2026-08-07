package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

// 한글이 포함된 자산번호(예: A자료검색대3-001)는 소문자 퍼센트 인코딩으로
// 들어와도 디코딩된 값으로 핸들러에 전달돼야 한다.
func TestDecodePathMiddlewareKoreanID(t *testing.T) {
	const want = "A자료검색대3-001"

	e := echo.New()
	e.Pre(DecodePathMiddleware)

	var got string
	e.GET("/assets/:id", func(c echo.Context) error {
		got = c.Param("id")
		return c.NoContent(http.StatusOK)
	})

	cases := map[string]string{
		"디코딩된 경로": "/assets/A자료검색대3-001",
		"대문자 인코딩":  "/assets/A%EC%9E%90%EB%A3%8C%EA%B2%80%EC%83%89%EB%8C%803-001",
		"소문자 인코딩":  "/assets/A%ec%9e%90%eb%a3%8c%ea%b2%80%ec%83%89%eb%8c%803-001",
	}

	for name, path := range cases {
		got = ""
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

		if rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200", name, rec.Code)
		}
		if got != want {
			t.Errorf("%s: param = %q, want %q", name, got, want)
		}
	}
}
