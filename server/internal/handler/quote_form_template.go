package handler

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"
)

func EnsureQuoteFormPD() error {
	if _, err := os.Stat(quoteFormPPath); err != nil {
		if err := BuildQuoteFormTemplates("", ""); err != nil {
			return err
		}
	}
	if _, err := os.Stat(quoteFormDPath); err != nil {
		return BuildQuoteFormTemplates("", "")
	}
	return nil
}

func BuildQuoteFormTemplates(docsDir, outDir string) error {
	if docsDir == "" {
		docsDir = quoteDocsDir()
	}
	if outDir == "" {
		outDir = filepath.Join("web", "templates", "quote")
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return err
	}
	pSrc, err := findQuoteSource(docsDir, "20260915", "앤로보틱스")
	if err != nil {
		return fmt.Errorf("P형 원본: %w", err)
	}
	dSrc := filepath.Join(docsDir, "개발_템플릿.xlsx")
	if _, err := os.Stat(dSrc); err != nil {
		return fmt.Errorf("D형 원본: %w", err)
	}
	pOut := filepath.Join(outDir, "form_p.xlsx")
	dOut := filepath.Join(outDir, "form_d.xlsx")
	if err := blankAndSave(pSrc, pOut, blankQuoteFormP); err != nil {
		return err
	}
	if err := blankAndSave(dSrc, dOut, blankQuoteFormD); err != nil {
		return err
	}
	if err := assertMediaKept(pSrc, pOut); err != nil {
		return fmt.Errorf("P형 그림: %w", err)
	}
	if err := assertMediaKept(dSrc, dOut); err != nil {
		return fmt.Errorf("D형 그림: %w", err)
	}
	return nil
}

func findQuoteSource(dir, mustContain, alt string) (string, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range ents {
		n := e.Name()
		if !strings.HasSuffix(strings.ToLower(n), ".xlsx") {
			continue
		}
		if strings.Contains(n, mustContain) || strings.Contains(n, alt) {
			return filepath.Join(dir, n), nil
		}
	}
	return "", fmt.Errorf("%s 에서 %s 파일을 찾지 못했습니다", dir, mustContain)
}

func blankAndSave(src, dst string, blank func(*excelize.File) error) error {
	f, err := excelize.OpenFile(src)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := blank(f); err != nil {
		return err
	}
	return f.SaveAs(dst)
}

func clearCell(f *excelize.File, sheet, addr string) {
	_ = f.SetCellValue(sheet, addr, nil)
	_ = f.SetCellValue(sheet, addr, "")
}

func blankQuoteFormP(f *excelize.File) error {
	sheet := quoteA1Sheet(f)
	b6, _ := f.GetCellValue(sheet, "B6")
	a6, _ := f.GetCellValue(sheet, "A6")
	if strings.TrimSpace(a6) == "" && strings.TrimSpace(b6) != "" {
		_ = f.SetCellValue(sheet, "A6", strings.TrimSpace(b6))
		clearCell(f, sheet, "B6")
	}
	for _, cell := range []string{
		"A4", "E4", "A5", "A6", "A7",
		"F9", "I9",
		"B12", "F12", "I12", "B13", "C13", "F13", "I13",
		"A16", "B16", "F16", "G16", "H16", "I16",
		"I17", "I18", "H19", "A27",
	} {
		clearCell(f, sheet, cell)
	}
	_ = f.SetCellValue(sheet, "A5", "▷ 수     신 :")
	_ = f.SetCellValue(sheet, "A6", "▷ 참     조 :")
	_ = f.SetCellValue(sheet, "A7", "▷ 견 적 일 :")
	_ = f.SetCellValue(sheet, "E4", "견적번호 :")
	setPrintArea(f, sheet, "$A$1:$I$30")
	setFitToWidth1(f, sheet)
	return nil
}

func blankQuoteFormD(f *excelize.File) error {
	sheet := quoteA1Sheet(f)
	for _, cell := range []string{
		"B4", "F4", "C5", "C6", "C7",
		"D12", "J12", "D13", "J13",
		"I16", "J16", "I20", "J20", "I24", "J24", "I25",
		"B33",
	} {
		clearCell(f, sheet, cell)
	}
	for r := 17; r <= 19; r++ {
		clearDLine(f, sheet, r)
	}
	for r := 21; r <= 23; r++ {
		clearDLine(f, sheet, r)
	}
	_ = f.SetCellValue(sheet, "C5", " 수     신  :")
	_ = f.SetCellValue(sheet, "C6", " 참     조 :")
	_ = f.SetCellValue(sheet, "C7", " 견 적 일 :")
	_ = f.SetCellValue(sheet, "F4", "견적번호 :")
	setPrintArea(f, sheet, "$B$1:$J$36")
	setFitToWidth1(f, sheet)
	return nil
}

func clearDLine(f *excelize.File, sheet string, row int) {
	for _, col := range []string{"C", "D", "E", "F", "G", "H", "I", "J"} {
		clearCell(f, sheet, fmt.Sprintf("%s%d", col, row))
	}
}

func assertMediaKept(src, dst string) error {
	a, err := zipMediaCountFile(src)
	if err != nil {
		return err
	}
	b, err := zipMediaCountFile(dst)
	if err != nil {
		return err
	}
	if a != b {
		return fmt.Errorf("xl/media 개수 원본 %d → 저장 %d (로고·직인이 빠졌습니다)", a, b)
	}
	return nil
}
