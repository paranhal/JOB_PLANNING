package handler

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func Test95_167A_ImportPreviewNotice(t *testing.T) {
	e, _, _, db := newProjectWorkHarness(t)
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.POST("/projects/:id/sites/schools-import-preview", h.Project.SiteSchoolImportPreview)
	raw, err := repository.BuildSiteSchoolDataWorkbook([][]string{
		{"상위기관명", "기관코드", "기관명 *", "상태", "설치일", "검수일", "비고"},
		{model.SchoolParentOfficialName, "", "없는초등학교167A", "", "", "", ""},
	})
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	if err := w.WriteField("mode", "add"); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteField("create_missing", "1"); err != nil {
		t.Fatal(err)
	}
	part, err := w.CreateFormFile("file", "schools.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/projects/WP-O01/sites/schools-import-preview", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("미리보기 %d %s", rec.Code, rec.Body.String())
	}
	page := rec.Body.String()
	if !strings.Contains(page, "없는 학교만 새 고객으로 만듭니다") || !strings.Contains(page, "새로 만들 고객") || !strings.Contains(page, "없는초등학교167A") {
		t.Fatalf("안내 문구 없음")
	}
}
