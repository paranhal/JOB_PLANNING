package hwpx

import (
	"strings"
	"testing"
)

func TestFitASReportTablesShortensBodyKeepsSignatureRow(t *testing.T) {
	data := readOfficialReportTemplate(t)
	out, err := FitASReportTables(data)
	if err != nil {
		t.Fatal(err)
	}
	sec := string(mustZipFile(t, out, "Contents/section0.xml"))
	if strings.Contains(sec, `height="16172"`) || strings.Contains(sec, `height="17315"`) {
		t.Fatal("조치·장애 표가 줄어들지 않았다")
	}
	if !strings.Contains(sec, `height="10795"`) || !strings.Contains(sec, `height="6010"`) {
		t.Fatal("한글에서 줄인 칸 높이가 없다")
	}
	if strings.Count(sec, `width="18430" height="2414"`) < 1 {
		t.Fatal("사인 칸 높이를 바꿨다")
	}
	if !strings.Contains(sec, `width="48499" widthRelTo="ABSOLUTE" height="4828"`) {
		t.Fatal("보고·사인 표 높이가 칸 합과 다르다")
	}
	last := sec[strings.LastIndex(sec, "<hp:tbl"):]
	end := strings.Index(last, "</hp:tbl>")
	if end < 0 || !strings.Contains(last[:end], `vertOffset="1977"`) {
		t.Fatal("한글 저장본과 다른 사인 표 자리다")
	}
}
