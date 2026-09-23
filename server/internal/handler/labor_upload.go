package handler

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/labstack/echo/v4"
	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
)

type laborUploadRow struct {
	Row        int
	Name       string
	Monthly    int
	Daily      int
	Hourly     int
	JobCode    string
	Verdict    string
	OldMonthly int
	Unmatched  bool
}

func (h *QuotesHandler) SaveLaborRows(c echo.Context) error {
	if !canWriteSales(c) && !isAdminRole(c) {
		return echo.ErrForbidden
	}
	year := atoiQuiet(c.FormValue("year"))
	if year <= 0 {
		year = time.Now().Year()
	}
	_ = c.Request().ParseForm()
	codes := c.Request().PostForm["job_code"]
	names := c.Request().PostForm["job_name"]
	monthlies := c.Request().PostForm["monthly"]
	dailies := c.Request().PostForm["daily"]
	hourlies := c.Request().PostForm["hourly"]
	var items []model.LaborRate
	n := len(codes)
	if len(names) > n {
		n = len(names)
	}
	for i := 0; i < n; i++ {
		code, name := "", ""
		if i < len(codes) {
			code = strings.TrimSpace(codes[i])
		}
		if i < len(names) {
			name = strings.TrimSpace(names[i])
		}
		if code == "" && name == "" {
			continue
		}
		if code == "" {
			code = fmt.Sprintf("%02d", i+1)
		}
		it := model.LaborRate{Year: year, JobCode: code, JobName: name}
		if i < len(monthlies) {
			it.Monthly = atoiQuiet(strings.ReplaceAll(monthlies[i], ",", ""))
		}
		if i < len(dailies) {
			it.Daily = atoiQuiet(strings.ReplaceAll(dailies[i], ",", ""))
		}
		if i < len(hourlies) {
			it.Hourly = atoiQuiet(strings.ReplaceAll(hourlies[i], ",", ""))
		}
		items = append(items, it)
	}
	added, changed, err := h.repo.SaveLaborRates(year, items)
	if err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, fmt.Sprintf("/labor-rates?year=%d&ok=rows&added=%d&changed=%d", year, added, changed))
}

func (h *QuotesHandler) CopyLaborRates(c echo.Context) error {
	if !canWriteSales(c) && !isAdminRole(c) {
		return echo.ErrForbidden
	}
	from := atoiQuiet(c.FormValue("from"))
	to := atoiQuiet(c.FormValue("to"))
	if from <= 0 || to <= 0 {
		return c.Redirect(http.StatusSeeOther, "/labor-rates?err=year")
	}
	n, err := h.repo.CopyLaborRates(from, to)
	if err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, fmt.Sprintf("/labor-rates?year=%d&ok=copy&copied=%d", to, n))
}

func (h *QuotesHandler) LaborUploadForm(c echo.Context) error {
	if !canViewSales(c) {
		return echo.ErrForbidden
	}
	year := atoiQuiet(c.QueryParam("year"))
	if year <= 0 {
		year = time.Now().Year()
	}
	return c.Render(http.StatusOK, "quotes/labor_upload.html", map[string]interface{}{
		"Title": "노임단가 올리기", "Active": NavQuotes, "Year": year, "CanWrite": canWriteSales(c) || isAdminRole(c),
	})
}

func (h *QuotesHandler) LaborUploadPreview(c echo.Context) error {
	if !canWriteSales(c) && !isAdminRole(c) {
		return echo.ErrForbidden
	}
	year := atoiQuiet(c.FormValue("year"))
	if year <= 0 {
		year = time.Now().Year()
	}
	file, err := c.FormFile("file")
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/labor-rates/upload?err=file")
	}
	if file.Size > 5<<20 {
		return c.Redirect(http.StatusSeeOther, "/labor-rates/upload?err=size")
	}
	if !strings.HasSuffix(strings.ToLower(file.Filename), ".xlsx") {
		return c.Redirect(http.StatusSeeOther, "/labor-rates/upload?err=ext")
	}
	src, err := file.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	raw, err := io.ReadAll(src)
	if err != nil {
		return err
	}
	xf, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		return err
	}
	defer xf.Close()
	sheets := xf.GetSheetList()
	sheet := strings.TrimSpace(c.FormValue("sheet"))
	if sheet == "" && len(sheets) > 0 {
		sheet = sheets[0]
	}
	existing, _ := h.repo.ListLaborRates(year)
	rows := parseLaborExcel(xf, sheet, existing)
	return c.Render(http.StatusOK, "quotes/labor_upload.html", map[string]interface{}{
		"Title": "노임단가 올리기", "Active": NavQuotes, "Year": year,
		"FileName": file.Filename, "Sheets": sheets, "Sheet": sheet, "Rows": rows,
		"CanWrite": true, "Preview": true, "Existing": existing,
	})
}

func (h *QuotesHandler) LaborUploadApply(c echo.Context) error {
	if !canWriteSales(c) && !isAdminRole(c) {
		return echo.ErrForbidden
	}
	_ = c.Request().ParseForm()
	year := atoiQuiet(c.FormValue("year"))
	fileName := strings.TrimSpace(c.FormValue("file_name"))
	note := fmt.Sprintf("%s · %s · %s", fileName, time.Now().Format("2006-01-02"), ctxString(c, "user_name"))
	codes := c.Request().PostForm["job_code"]
	names := c.Request().PostForm["job_name"]
	monthlies := c.Request().PostForm["monthly"]
	dailies := c.Request().PostForm["daily"]
	hourlies := c.Request().PostForm["hourly"]
	skips := map[int]bool{}
	for _, s := range c.Request().PostForm["skip"] {
		skips[atoiQuiet(s)] = true
	}
	var items []model.LaborRate
	for i := range names {
		if skips[i] {
			continue
		}
		name := strings.TrimSpace(names[i])
		if name == "" {
			continue
		}
		code := ""
		if i < len(codes) {
			code = strings.TrimSpace(codes[i])
		}
		if code == "" {
			code = fmt.Sprintf("%02d", i+1)
		}
		it := model.LaborRate{Year: year, JobCode: code, JobName: name, SourceNote: note}
		if i < len(monthlies) {
			it.Monthly = atoiQuiet(strings.ReplaceAll(monthlies[i], ",", ""))
		}
		if i < len(dailies) {
			it.Daily = atoiQuiet(strings.ReplaceAll(dailies[i], ",", ""))
		}
		if i < len(hourlies) {
			it.Hourly = atoiQuiet(strings.ReplaceAll(hourlies[i], ",", ""))
		}
		items = append(items, it)
	}
	added, changed, err := h.repo.SaveLaborRates(year, items)
	if err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, fmt.Sprintf("/labor-rates?year=%d&ok=upload&added=%d&changed=%d", year, added, changed))
}

func parseLaborExcel(xf *excelize.File, sheet string, existing []model.LaborRate) []laborUploadRow {
	rows, err := xf.GetRows(sheet)
	if err != nil {
		return nil
	}
	head := -1
	colJob, colM, colD, colH := -1, -1, -1, -1
	for i, row := range rows {
		for j, cell := range row {
			s := strings.TrimSpace(cell)
			if strings.Contains(s, "직무") || strings.Contains(s, "직종") || strings.Contains(s, "구분") {
				if head < 0 {
					head = i
					colJob = j
				}
			}
			if strings.Contains(s, "월평균") || strings.Contains(s, "월임금") {
				colM = j
				if head < 0 {
					head = i
				}
			}
			if strings.Contains(s, "일평균") || strings.Contains(s, "일임금") {
				colD = j
			}
			if strings.Contains(s, "시간평균") || strings.Contains(s, "시간당") {
				colH = j
			}
		}
		if head >= 0 && colJob >= 0 && colM >= 0 {
			break
		}
	}
	if head < 0 || colJob < 0 {
		return []laborUploadRow{{Row: 1, Name: "(머리글을 찾지 못함)", Unmatched: true, Verdict: "못 읽음"}}
	}
	byNorm := map[string]model.LaborRate{}
	for _, it := range existing {
		byNorm[normLaborName(it.JobName)] = it
	}
	var out []laborUploadRow
	for i := head + 1; i < len(rows); i++ {
		row := rows[i]
		name := cellAt(row, colJob)
		if strings.TrimSpace(name) == "" {
			continue
		}
		ur := laborUploadRow{Row: i + 1, Name: name, Monthly: parseWonInt(cellAt(row, colM)), Daily: parseWonInt(cellAt(row, colD)), Hourly: parseWonInt(cellAt(row, colH))}
		if hit, ok := byNorm[normLaborName(name)]; ok {
			ur.JobCode = hit.JobCode
			ur.OldMonthly = hit.Monthly
			if hit.Monthly == ur.Monthly {
				ur.Verdict = "같음"
			} else {
				ur.Verdict = fmt.Sprintf("바뀜 %s → %s", formatIntComma(hit.Monthly), formatIntComma(ur.Monthly))
			}
		} else {
			ur.Unmatched = true
			ur.Verdict = "못 읽음"
		}
		out = append(out, ur)
	}
	return out
}

func cellAt(row []string, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

func parseWonInt(s string) int {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, "원", "")
	s = strings.ReplaceAll(s, " ", "")
	n, _ := strconv.Atoi(s)
	return n
}

func normLaborName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsSpace(r) || r == '(' || r == ')' || r == '/' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func formatIntComma(n int) string {
	s := strconv.Itoa(n)
	if n < 0 {
		return "-" + formatIntComma(-n)
	}
	var out []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, byte(c))
	}
	return string(out)
}
