package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
)

// writeExcelDownload xlsx 바이트를 다운로드 응답으로 보냄.
// 한글 파일명은 Chrome에서 .crdownload로 남는 경우가 있어 ASCII 파일명만 사용한다.
func writeExcelDownload(c echo.Context, data []byte, namePrefix string) error {
	if namePrefix == "" {
		namePrefix = "export"
	}
	filename := fmt.Sprintf("%s_%s.xlsx", time.Now().Format("20060102"), namePrefix)
	ct := "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

	res := c.Response()
	res.Header().Set(echo.HeaderContentType, ct)
	res.Header().Set(echo.HeaderContentDisposition, fmt.Sprintf(`attachment; filename="%s"`, filename))
	res.Header().Set(echo.HeaderContentLength, strconv.Itoa(len(data)))
	res.Header().Set("X-Content-Type-Options", "nosniff")
	res.Header().Set("Cache-Control", "no-store")
	res.WriteHeader(http.StatusOK)
	_, err := res.Write(data)
	return err
}
