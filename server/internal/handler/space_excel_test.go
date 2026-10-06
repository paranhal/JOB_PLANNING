package handler

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func newSpaceExcelApp(t *testing.T) *echo.Echo {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "space-xlsx.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	org := model.OrgIDLibrary
	must := func(q string, args ...interface{}) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	must(`INSERT INTO customers (customer_id, org_name, official_name, org_id, is_active)
		VALUES ('C1','가도서관','가도서관',?,1), ('C2','나도서관','나도서관',?,1)`, org, org)
	must(`INSERT INTO customer_buildings (building_id,customer_id,building_name,is_active)
		VALUES ('B1','C1','본관',1)`)
	must(`INSERT INTO customer_floors (floor_id,building_id,floor_name,sort_order) VALUES ('F1','B1','1층',1)`)
	for i := 1; i <= 55; i++ {
		must(`INSERT INTO customer_rooms (room_id,floor_id,room_name) VALUES (?,?,?)`,
			fmt.Sprintf("R%02d", i), "F1", fmt.Sprintf("%d호", i))
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/spaces/export.xlsx", h.Space.ExportExcel, h.Auth.RequireMasterView)
	return e
}

func getSpaceXlsx(t *testing.T, e *echo.Echo, path string) *excelize.File {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	f, err := excelize.OpenReader(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestSpaceExcelTwoSheetsAndHeaders(t *testing.T) {
	e := newSpaceExcelApp(t)
	f := getSpaceXlsx(t, e, "/spaces/export.xlsx")
	defer f.Close()
	names := f.GetSheetList()
	if len(names) != 2 || names[0] != "공간" || names[1] != "기관별 집계" {
		t.Fatalf("sheets=%v", names)
	}
	h1, err := f.GetRows("공간")
	if err != nil {
		t.Fatal(err)
	}
	if len(h1) < 1 {
		t.Fatal("공간 시트 비었다")
	}
	want1 := []string{"기관명", "건물명", "층", "호/실명"}
	if strings.Join(h1[0], ",") != strings.Join(want1, ",") {
		t.Fatalf("공간 머리글=%v", h1[0])
	}
	h2, err := f.GetRows("기관별 집계")
	if err != nil {
		t.Fatal(err)
	}
	want2 := []string{"기관명", "건물수", "층수", "호/실 개수"}
	if strings.Join(h2[0], ",") != strings.Join(want2, ",") {
		t.Fatalf("집계 머리글=%v", h2[0])
	}
}

func TestSpaceExcelSearchAndNoPageLimit(t *testing.T) {
	e := newSpaceExcelApp(t)
	f := getSpaceXlsx(t, e, "/spaces/export.xlsx")
	defer f.Close()
	rows, err := f.GetRows("공간")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows)-1 < 55 {
		t.Fatalf("50줄 제한이 있다 rows=%d", len(rows)-1)
	}

	f2 := getSpaceXlsx(t, e, "/spaces/export.xlsx?q=55호")
	defer f2.Close()
	got, err := f2.GetRows("공간")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("검색 결과 줄(머리글 포함)=%d want 2, rows=%v", len(got), got)
	}
	if !strings.Contains(got[1][3], "55호") {
		t.Fatalf("검색 줄=%v", got[1])
	}
}

func TestSpaceExcelStatCellsAreNumbers(t *testing.T) {
	e := newSpaceExcelApp(t)
	f := getSpaceXlsx(t, e, "/spaces/export.xlsx")
	defer f.Close()
	for _, cell := range []string{"B2", "C2", "D2"} {
		ct, err := f.GetCellType("기관별 집계", cell)
		if err != nil {
			t.Fatal(err)
		}
		if ct != excelize.CellTypeNumber && ct != excelize.CellTypeUnset {
			t.Fatalf("%s type=%v want number", cell, ct)
		}
		v, err := f.GetCellValue("기관별 집계", cell)
		if err != nil {
			t.Fatal(err)
		}
		if v == "" {
			t.Fatalf("%s 비었다", cell)
		}
	}
}
