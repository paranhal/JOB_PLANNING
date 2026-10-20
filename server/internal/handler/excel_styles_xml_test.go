package handler

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

var rgbAttrRe = regexp.MustCompile(`rgb="([^"]*)"`)

func excelizeFileBytes(t *testing.T, f *excelize.File) []byte {
	t.Helper()
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func stylesXML(t *testing.T, xlsx []byte) string {
	t.Helper()
	if len(xlsx) == 0 {
		t.Fatal("xlsx 빈 본문")
	}
	zr, err := zip.NewReader(bytes.NewReader(xlsx), int64(len(xlsx)))
	if err != nil {
		t.Fatalf("zip: %v", err)
	}
	for _, zf := range zr.File {
		if zf.Name != "xl/styles.xml" {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	t.Fatal("xl/styles.xml 없음")
	return ""
}

func assertExcelizeRGB8(t *testing.T, xlsx []byte) {
	t.Helper()
	styles := []byte(stylesXML(t, xlsx))
	ms := rgbAttrRe.FindAllSubmatch(styles, -1)
	for _, m := range ms {
		v := string(m[1])
		if len(v) != 8 {
			t.Fatalf("rgb=%q 길이=%d (8자리 FF+RRGGBB 이어야 함)", v, len(v))
		}
		up := strings.ToUpper(v)
		if len(up) != 8 || !isHex6(up[2:]) {
			t.Fatalf("rgb=%q 16진 아님", v)
		}
		for i := 0; i < 2; i++ {
			c := up[i]
			if (c < '0' || c > '9') && (c < 'A' || c > 'F') {
				t.Fatalf("rgb=%q 16진 아님", v)
			}
		}
	}
}

func dailyAssigneeColorFixture() model.DailyAssigneeReport {
	return model.DailyAssigneeReport{
		From: "2026-08-10", To: "2026-08-16", FileDay: "20260814", WorkingDays: 4,
		Days: []model.DailyAssigneeDay{
			{Date: "2026-08-10", Label: "08-10(월)"},
			{Date: "2026-08-11", Label: "08-11(화)"},
			{Date: "2026-08-12", Label: "08-12(수)", Off: true},
			{Date: "2026-08-13", Label: "08-13(목)"},
		},
		People: []model.DailyAssigneePerson{
			{
				Label:  "양기헌",
				Counts: []int{2, 1, 0, 0}, Minutes: []int{40, 20, 0, 0},
				CountTotal: 3, HasCountAvg: true, CountAvg: 0.8, CountMax: 2,
				MinuteTotal: 60, HasMinuteAvg: true, MinuteAvg: 15, MinuteMax: 40,
			},
		},
		TeamCounts: []int{2, 1, 0, 0}, TeamMinutes: []int{40, 20, 0, 0},
		TeamCountTotal: 3, HasTeamCountAvg: true, TeamCountAvg: 0.8, TeamCountMax: 2,
		TeamMinuteTotal: 60, HasTeamMinuteAvg: true, TeamMinuteAvg: 15, TeamMinuteMax: 40,
		Details: []model.DailyAssigneeDetailRow{
			{Assignee: "양기헌", Date: "2026-08-10", Weekday: "월", Kind: "AS", WorkNo: "R1", Customer: "도서관", Content: "게이트", Status: "완료", Minutes: 40, Cumulative: 40, NewPerson: true},
		},
	}
}

func weeklyColorFixture() model.WeeklyReport {
	return model.WeeklyReport{
		WeekFrom: "2026-08-10", WeekTo: "2026-08-16", Friday: "2026-08-14",
		WorkingDays: 5,
		PersonRows: []model.WeeklyPersonRow{
			{Label: model.StatsTeamLabel, IsTeam: true, Receipt: 1, Completed: 1, HasDayAvg: true, DayAvg: 1, DayMax: 1},
			{Label: "최혜경", Receipt: 1, Completed: 1, HasDayAvg: true, DayAvg: 1, DayMax: 1},
		},
		Events: []model.WeeklyEventRow{
			{Kind: model.WeeklyEventReceipt, WorkNo: "R1", OccurDate: "2026-08-11", WorkType: "AS", Customer: "충남", Assignee: "최혜경"},
		},
	}
}

func TestDailyAssigneeStoredFillColors(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	rep := dailyAssigneeColorFixture()
	if err := writeDailyMatrixSheet(f, rep); err != nil {
		t.Fatal(err)
	}
	if err := writeDailyDetailSheet(f, rep); err != nil {
		t.Fatal(err)
	}
	_ = f.DeleteSheet("Sheet1")
	raw := excelizeFileBytes(t, f)
	assertExcelizeRGB8(t, raw)
	styles := stylesXML(t, raw)
	if !strings.Contains(styles, `rgb="FF1E3A5F"`) {
		t.Fatal("헤더 남색 FF1E3A5F 없음")
	}
	if !strings.Contains(styles, `rgb="FFFF4D4F"`) {
		t.Fatal("휴일 빨강 FFFF4D4F 없음")
	}
	if strings.Contains(styles, `rgb="FFE5E7EB"`) || strings.Contains(styles, `rgb="FFFECACA"`) {
		t.Fatal("옛 회색·0건 빨강이 남음")
	}
	if cellFillRGB(t, f, dailyMatrixSheet, "D2") != "FF4D4F" {
		t.Fatalf("휴일 헤더 D2=%s", cellFillRGB(t, f, dailyMatrixSheet, "D2"))
	}
	if cellFillRGB(t, f, dailyMatrixSheet, "E3") != "" {
		t.Fatalf("평일 0건 E3 바탕=%s want 없음", cellFillRGB(t, f, dailyMatrixSheet, "E3"))
	}
}

func cellFillRGB(t *testing.T, f *excelize.File, sheet, cell string) string {
	t.Helper()
	idx, err := f.GetCellStyle(sheet, cell)
	if err != nil {
		t.Fatal(err)
	}
	st, err := f.GetStyle(idx)
	if err != nil {
		t.Fatalf("GetStyle(%d) %s: %v", idx, cell, err)
	}
	if st == nil || len(st.Fill.Color) == 0 {
		return ""
	}
	rgb := strings.ToUpper(strings.TrimPrefix(st.Fill.Color[0], "#"))
	if len(rgb) == 8 && strings.HasPrefix(rgb, "FF") {
		return rgb[2:]
	}
	return rgb
}

func TestExcelizeExportsStyleRGB8(t *testing.T) {
	t.Run("daily", func(t *testing.T) {
		f := excelize.NewFile()
		defer f.Close()
		rep := dailyAssigneeColorFixture()
		if err := writeDailyMatrixSheet(f, rep); err != nil {
			t.Fatal(err)
		}
		if err := writeDailyDetailSheet(f, rep); err != nil {
			t.Fatal(err)
		}
		_ = f.DeleteSheet("Sheet1")
		assertExcelizeRGB8(t, excelizeFileBytes(t, f))
	})
	t.Run("weekly", func(t *testing.T) {
		f := excelize.NewFile()
		defer f.Close()
		rep := weeklyColorFixture()
		if err := writeWeeklyStatsSheet(f, rep); err != nil {
			t.Fatal(err)
		}
		if err := writeWeeklyAllListSheet(f, rep); err != nil {
			t.Fatal(err)
		}
		_ = writeSimpleSheet(f, "휴가", []string{"문서"}, func(i int) []interface{} {
			if i > 0 {
				return nil
			}
			return []interface{}{"L-1"}
		})
		_ = f.DeleteSheet("Sheet1")
		assertExcelizeRGB8(t, excelizeFileBytes(t, f))
	})
	t.Run("company_weekly", func(t *testing.T) {
		period := repository.CompanyWeeklyPeriod(time.Date(2026, 8, 17, 0, 0, 0, 0, time.Local))
		raw, err := buildCompanyWeeklySingleSheet(companyWeeklyBuildInput{
			SheetName: "26.8월3주(업무)", Period: period, Rows: sampleCompanyWeeklyRows(),
		})
		if err != nil {
			t.Fatal(err)
		}
		assertExcelizeRGB8(t, raw)

		src := mustCompanyWeeklyFixture(t)
		out, err := buildCompanyWeeklyWorkbook(companyWeeklyBuildInput{
			Source: src, SheetName: "26.8월3주(업무)", Period: period, Rows: sampleCompanyWeeklyRows(),
		})
		if err != nil {
			t.Fatal(err)
		}
		assertExcelizeRGB8(t, out.Data)
	})
	t.Run("asset", func(t *testing.T) {
		f, err := buildAssetExcelWorkbook(nil)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		assertExcelizeRGB8(t, excelizeFileBytes(t, f))
	})
	t.Run("space", func(t *testing.T) {
		f, err := buildSpaceExcelWorkbook(nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		assertExcelizeRGB8(t, excelizeFileBytes(t, f))
	})
	t.Run("customer", func(t *testing.T) {
		f, err := buildCustomerExcelWorkbook(nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		assertExcelizeRGB8(t, excelizeFileBytes(t, f))
	})
	t.Run("maintenance", func(t *testing.T) {
		f, err := BuildMaintenanceExcelWorkbook(2026, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		assertExcelizeRGB8(t, excelizeFileBytes(t, f))
	})
	t.Run("site_config", func(t *testing.T) {
		f, err := BuildSiteConfigExcelWorkbook(nil)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		assertExcelizeRGB8(t, excelizeFileBytes(t, f))
	})
	t.Run("knowledge", func(t *testing.T) {
		f, err := buildKnowledgeExcel(nil)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		assertExcelizeRGB8(t, excelizeFileBytes(t, f))
	})
	t.Run("conflicts", func(t *testing.T) {
		f, err := buildActionConflictExcel(nil)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		assertExcelizeRGB8(t, excelizeFileBytes(t, f))
	})
	t.Run("stats_detail", func(t *testing.T) {
		f := excelize.NewFile()
		defer f.Close()
		if err := writeStatsSheet(f, "Sheet1", nil, nil); err != nil {
			t.Fatal(err)
		}
		assertExcelizeRGB8(t, excelizeFileBytes(t, f))
	})
	t.Run("quote_templates", func(t *testing.T) {
		for _, name := range []string{"A1", "A2", "B1", "B2"} {
			var (
				f   *excelize.File
				err error
			)
			switch name {
			case "A1":
				f, err = buildFormA1Template()
			case "A2":
				f, err = buildFormA2Template()
			case "B1":
				f, err = buildFormB1Template()
			default:
				f, err = buildFormB2Template()
			}
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			raw := excelizeFileBytes(t, f)
			_ = f.Close()
			assertExcelizeRGB8(t, raw)
		}
	})
	t.Run("pipeline", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		if err := writePipelineXLSX(c, repository.SalesListFilter{}, model.SalesForecastOpts{}, model.SalesForecast{}, nil); err != nil {
			t.Fatal(err)
		}
		assertExcelizeRGB8(t, rec.Body.Bytes())
	})
	t.Run("sales_logs", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		if err := writeSalesLogsXLSX(c, repository.SalesLogFilter{From: "2026-08-01", To: "2026-08-31"}, nil); err != nil {
			t.Fatal(err)
		}
		assertExcelizeRGB8(t, rec.Body.Bytes())
	})
}

func TestWriteExcelFileHeaders(t *testing.T) {
	body := []byte("xlsx-bytes")
	e := echo.New()
	e.GET("/dl", func(c echo.Context) error {
		return writeExcelFile(c, body, "20261020_daily.xlsx", "20261020_담당자별일일업무.xlsx")
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/dl", nil)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	got := rec.Body.Bytes()
	if !bytes.Equal(got, body) {
		t.Fatalf("본문=%q", got)
	}
	cl := rec.Header().Get(echo.HeaderContentLength)
	if cl != strconv.Itoa(len(body)) {
		t.Fatalf("Content-Length=%q want %d", cl, len(body))
	}
	disp := rec.Header().Get(echo.HeaderContentDisposition)
	if !strings.Contains(disp, `filename="20261020_daily.xlsx"`) {
		t.Fatalf("ASCII 파일명 없음: %s", disp)
	}
	if !strings.Contains(disp, "filename*=UTF-8''") {
		t.Fatalf("UTF-8 파일명 없음: %s", disp)
	}
	if !strings.Contains(disp, ".xlsx") {
		t.Fatalf("확장자 .xlsx 없음: %s", disp)
	}
	if strings.Contains(disp, ".crdownload") {
		t.Fatalf("crdownload: %s", disp)
	}
}
