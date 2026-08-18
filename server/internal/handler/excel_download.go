package handler

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
)

// writeExcelDownload xlsx 바이트를 다운로드 응답으로 보냄.
func writeExcelDownload(c echo.Context, data []byte, namePrefix string) error {
	if namePrefix == "" {
		namePrefix = "export"
	}
	filename := fmt.Sprintf("%s_%s.xlsx", time.Now().Format("20060102"), namePrefix)
	return writeExcelFile(c, data, filename, "")
}

// writeExcelFile ASCII 파일명과 UTF-8 한글 파일명(filename*)을 함께 넣는다.
func writeExcelFile(c echo.Context, data []byte, asciiName, utf8Name string) error {
	if asciiName == "" {
		asciiName = "export.xlsx"
	}
	return writeDownloadBytes(c, data, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", asciiName, utf8Name)
}

func writeDownloadBytes(c echo.Context, data []byte, contentType, asciiName, utf8Name string) error {
	if asciiName == "" {
		asciiName = "download"
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	disp := fmt.Sprintf(`attachment; filename="%s"`, asciiName)
	if utf8Name != "" && utf8Name != asciiName {
		disp += `; filename*=UTF-8''` + url.PathEscape(utf8Name)
	}

	res := c.Response()
	res.Header().Set(echo.HeaderContentType, contentType)
	res.Header().Set(echo.HeaderContentDisposition, disp)
	res.Header().Set(echo.HeaderContentLength, strconv.Itoa(len(data)))
	res.Header().Set("X-Content-Type-Options", "nosniff")
	res.Header().Set("Cache-Control", "no-store")
	res.WriteHeader(http.StatusOK)
	_, err := res.Write(data)
	return err
}
