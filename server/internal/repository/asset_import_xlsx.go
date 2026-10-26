package repository

import (
	"bytes"

	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
)

func BuildAssetImportTemplate() ([]byte, error) {
	return buildAssetImportWorkbook(nil, true)
}

func BuildAssetImportFixture(rows [][]string) ([]byte, error) {
	return buildAssetImportWorkbook(rows, false)
}

func buildAssetImportWorkbook(data [][]string, withGuide bool) ([]byte, error) {
	f := excelize.NewFile()
	base := f.GetSheetName(0)
	_ = f.SetSheetName(base, "자산입력")
	if withGuide {
		_, _ = f.NewSheet("작성안내")
		_, _ = f.NewSheet("작성예시")
		_, _ = f.NewSheet("코드표")
		guide := []string{
			"설치 자산 일괄 입력 양식",
			"‘자산입력’ 시트 머리글은 바꾸지 마세요. 2행부터 한 줄에 장비 1대입니다.",
			"필수: 기관명 · 제품명 · 설치위치(또는 건물명·층·호실).",
			"기관명은 등록 고객의 기관명·공식명·짧은이름과 정규화 일치해야 합니다. 고객은 전사 공용이라 등록 부서와 달라도 됩니다. 새 고객은 만들지 않습니다. 자산은 업로더 조직으로 저장합니다.",
			"설치위치가 건물/층/호실 슬래시 3칸이면 자동으로 나눕니다. 저장 값은 건물 층 호실을 공백으로 잇습니다.",
			"관리유형은 장애대응 · 정기점검 · 타사장비만 넣습니다. 미계약이면 점검주기 기본값은 CALL입니다.",
		}
		for i, line := range guide {
			_ = f.SetCellValue("작성안내", cellName(1, i+1), ExcelSafeText(line))
		}
		exHdr := []string{"기관명", "제품명", "설치위치", "건물명", "층", "호실", "제품구분", "관리유형", "설치주체"}
		for i, h := range exHdr {
			_ = f.SetCellValue("작성예시", cellName(i+1, 1), ExcelSafeText(h))
		}
		ex := []string{"○○초등학교", "AP", "본관/1층/교무실", "", "", "", "네트워크장비", "정기점검", "타사"}
		for i, v := range ex {
			_ = f.SetCellValue("작성예시", cellName(i+1, 2), ExcelSafeText(v))
		}
		codeHdr := []string{"제품구분", "운영상태", "설치주체", "관리유형", "계약구분", "점검주기", "제품군"}
		for i, h := range codeHdr {
			_ = f.SetCellValue("코드표", cellName(i+1, 1), ExcelSafeText(h))
		}
		codeRows := [][]string{
			{"SW", "운영중", "자사", "장애대응", "유상", "월", "홈페이지"},
			{"HW", "점검중", "타사", "정기점검", "무상", "분기", "전자도서관"},
			{"서버", "장애", "제조사", "타사장비", "미계약", "반기", "모바일"},
			{"네트워크장비", "철수", "협력사", "", "CALL", "홀수격월", "RFID자동화"},
			{"주변장비", "폐기", "미상", "", "", "짝수격월", "자료관리"},
			{"키오스크", "", "", "", "", "CALL", "기타"},
		}
		for r, row := range codeRows {
			for c, v := range row {
				_ = f.SetCellValue("코드표", cellName(c+1, r+2), ExcelSafeText(v))
			}
		}
	}
	for i, h := range assetImportDataHeaders {
		_ = f.SetCellValue("자산입력", cellName(i+1, 1), ExcelSafeText(h))
	}
	for r, row := range data {
		for c, v := range row {
			_ = f.SetCellValue("자산입력", cellName(c+1, r+2), v)
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	_ = f.Close()
	return buf.Bytes(), nil
}

func cellName(col, row int) string {
	name, _ := excelize.CoordinatesToCellName(col, row)
	return name
}

// BuildGanam27Workbook 95-62 시나리오: 가남초등학교 27건.
func BuildGanam27Workbook() ([]byte, error) {
	var rows [][]string
	hdrIdx := map[string]int{}
	for i, h := range assetImportDataHeaders {
		hdrIdx[normalizeAssetHeader(h)] = i
	}
	put := func(line []string, key, val string) {
		i, ok := hdrIdx[key]
		if !ok {
			return
		}
		for len(line) <= i {
			line = append(line, "")
		}
		line[i] = val
	}
	newLine := func() []string { return make([]string, len(assetImportDataHeaders)) }
	for i := 0; i < 27; i++ {
		line := newLine()
		put(line, "기관명", "가남초등학교")
		put(line, "제품명", "무선AP")
		put(line, "제품구분", "네트워크장비")
		if i < 19 {
			put(line, "관리유형", "정기점검")
			put(line, "설치주체", "타사")
		} else {
			put(line, "관리유형", "타사장비")
			put(line, "설치주체", "미상")
		}
		if i == 18 {
			put(line, "설치위치", "창의융합동 1층 다목적실")
		} else {
			room := "101"
			if i >= 19 {
				room = "201"
			}
			put(line, "설치위치", "본관/1층/"+room+itoaMin(i))
		}
		if i < 3 {
			put(line, "시리얼", "V2824")
			put(line, "모델명", "V2824")
		} else {
			put(line, "시리얼", "SN-GNAM-"+itoaMin(i))
			put(line, "모델명", "AP-X")
		}
		rows = append(rows, line)
	}
	_ = model.AssetImportMaxRows
	return BuildAssetImportFixture(rows)
}

func itoaMin(n int) string {
	if n < 0 {
		n = -n
	}
	s := ""
	if n == 0 {
		return "0"
	}
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
