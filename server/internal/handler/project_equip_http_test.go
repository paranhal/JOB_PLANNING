package handler

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

func postEquipFile(t *testing.T, e *echo.Echo, path, field, name string, data []byte, ck *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile(field, name)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write(data)
	_ = w.Close()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	if ck != nil {
		req.AddCookie(ck)
	} else {
		req.AddCookie(jwtCookie(t))
	}
	e.ServeHTTP(rec, req)
	return rec
}

func TestProjectEquipHTTPYearsSitesImportPerm(t *testing.T) {
	e, _, _ := newProjectServer(t)
	rec := doForm(t, e, "/projects", url.Values{
		"name": {"95사업"}, "status": {"active"}, "is_paid": {"1"}, "project_kind": {"build"},
		"contract_amount": {"1000"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create %d", rec.Code)
	}
	id := strings.TrimPrefix(strings.Split(rec.Header().Get("Location"), "?")[0], "/projects/")
	show := doGet(t, e, "/projects/"+id)
	if show.Code != http.StatusOK || !strings.Contains(show.Body.String(), "사이트·장비") {
		t.Fatalf("show %d", show.Code)
	}
	years := doGet(t, e, "/projects/"+id+"/years")
	if years.Code != http.StatusOK || !strings.Contains(years.Body.String(), "연차가 없습니다") {
		t.Fatalf("years empty %d %s", years.Code, clipBody(years.Body.String()))
	}
	yok := doForm(t, e, "/projects/"+id+"/years", url.Values{
		"year_no": {"1"}, "label": {"1차"}, "start_date": {"2026-01-01"}, "end_date": {"2026-12-31"},
		"amount": {"400"}, "status": {"planned"},
	})
	if yok.Code != http.StatusSeeOther {
		t.Fatalf("year save %d %s", yok.Code, yok.Header().Get("Location"))
	}
	sok := doForm(t, e, "/projects/"+id+"/sites", url.Values{
		"site_name": {"본관"}, "status": {"active"}, "region": {"서울"},
	})
	if sok.Code != http.StatusSeeOther {
		t.Fatalf("site %d", sok.Code)
	}
	sites := doGet(t, e, "/projects/"+id+"/sites")
	if sites.Code != http.StatusOK || !strings.Contains(sites.Body.String(), "본관") {
		t.Fatal("site list")
	}
	xlsx := doGet(t, e, "/projects/"+id+"/sites/template.xlsx")
	if xlsx.Code != http.StatusOK {
		t.Fatalf("xlsx %d", xlsx.Code)
	}
	if !strings.Contains(xlsx.Header().Get("Content-Disposition"), "attachment") {
		t.Fatal("download header")
	}
	assertExcelizeRGB8(t, xlsx.Body.Bytes())

	raw, err := repositoryBuildBook(
		[][]string{{"사이트명", "지역", "주소", "상태", "비고"}, {"별관", "경기", "", "예정", ""}},
		[][]string{{"사이트명", "분류코드", "제품명", "모델", "제조사", "수량", "시리얼", "설치위치", "설치일", "상태", "연차번호", "비고"},
			{"별관", "", "PC", "", "", "1", "SN-A", "", "", "예정", "", ""}},
	)
	if err != nil {
		t.Fatal(err)
	}
	prev := postEquipFile(t, e, "/projects/"+id+"/sites/import-preview", "file", "t.xlsx", raw, nil)
	if prev.Code != http.StatusOK || !strings.Contains(prev.Body.String(), "미리보기") {
		t.Fatalf("preview %d", prev.Code)
	}
	app := postEquipFile(t, e, "/projects/"+id+"/sites/import-apply", "file", "t.xlsx", raw, nil)
	if app.Code != http.StatusOK {
		t.Fatalf("apply %d", app.Code)
	}

	cl := jwtCookieRole(t, model.RoleCLevel)
	clGet := doGetWith(t, e, "/projects/"+id+"/years", cl)
	if clGet.Code != http.StatusOK {
		t.Fatalf("clevel get %d", clGet.Code)
	}
	clPost := workFormCookie(t, e, "/projects/"+id+"/years", url.Values{
		"year_no": {"3"}, "label": {"x"}, "start_date": {"2028-01-01"}, "end_date": {"2028-12-31"},
		"amount": {"1"}, "status": {"planned"},
	}, cl)
	if clPost.Code != http.StatusForbidden {
		t.Fatalf("clevel write %d", clPost.Code)
	}
	obs := jwtCookieUserClaims(t, &model.User{UserID: "USR-OBS95", Username: "obs95", Role: model.RoleObserver, IsReadOnly: true, IsActive: true})
	if doGetWith(t, e, "/projects/"+id+"/sites", obs).Code != http.StatusForbidden {
		t.Fatal("observer")
	}
	pt := jwtCookieUserClaims(t, &model.User{UserID: "USR-PT95", Username: "pt95", Role: "partner", IsActive: true})
	if code := doGetWith(t, e, "/projects/"+id+"/sites", pt).Code; code != http.StatusForbidden && code != http.StatusSeeOther {
		t.Fatalf("partner %d", code)
	}
	cats := doGet(t, e, "/projects/equipment-categories")
	if cats.Code != http.StatusOK || !strings.Contains(cats.Body.String(), "장비 분류") {
		t.Fatalf("cats %d", cats.Code)
	}
}

func repositoryBuildBook(sites, equips [][]string) ([]byte, error) {
	return buildEquipWorkbookBytes(sites, equips)
}
