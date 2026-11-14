package hwpx

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
)

// 95-190: 주간업무보고 발표 요약 자료(HWPX). 템플릿 없이 한글 12가 여는 기본 서식으로 만든다.
// 서식 원본이 따로 생기면 Replace 방식으로 바꾼다(지금은 제목·팀·사업별 추진내역/계획을 문단으로 적는다).

// SummaryItem 사업(또는 활동) 한 건의 발표 요약.
type SummaryItem struct {
	Name, Assignee, Customer string
	Done, Plan               []string
	Need, Decision           string
}

// SummaryTeam 팀 한 곳의 발표 요약.
type SummaryTeam struct {
	Name  string
	Items []SummaryItem
}

// BuildWeeklySummary 발표 요약 HWPX 를 만든다. 내용이 없으면 오류.
func BuildWeeklySummary(title, period string, teams []SummaryTeam) ([]byte, error) {
	var lines []string
	add := func(s string) { lines = append(lines, s) }
	add(title)
	if strings.TrimSpace(period) != "" {
		add(period)
	}
	add("")
	n := 0
	for _, t := range teams {
		if len(t.Items) == 0 {
			continue
		}
		add("■ " + t.Name)
		for _, it := range t.Items {
			head := "▶ " + it.Name
			if a := strings.TrimSpace(it.Assignee); a != "" {
				head += " (" + a + ")"
			}
			add(head)
			if c := strings.TrimSpace(it.Customer); c != "" {
				add("   고객: " + c)
			}
			if len(it.Done) > 0 {
				add("   금주 추진내역")
				for _, l := range it.Done {
					add("    - " + l)
				}
			}
			if len(it.Plan) > 0 {
				add("   차주 추진계획")
				for _, l := range it.Plan {
					add("    - " + l)
				}
			}
			if s := strings.TrimSpace(it.Need); s != "" {
				add("   지원 필요사항: " + oneLine(s))
			}
			if s := strings.TrimSpace(it.Decision); s != "" {
				add("   주요 의사결정: " + oneLine(s))
			}
			add("")
			n++
		}
	}
	if n == 0 {
		return nil, fmt.Errorf("발표 요약에 넣을 줄이 없습니다")
	}
	var sec bytes.Buffer
	sec.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes" ?>`)
	sec.WriteString(sectionOpen)
	sec.WriteString(`<hp:p id="0" paraPrIDRef="0" styleIDRef="0" pageBreak="0" columnBreak="0" merged="0"><hp:run charPrIDRef="0">`)
	sec.WriteString(secPrXML)
	sec.WriteString(`<hp:ctrl><hp:colPr id="" type="NEWSPAPER" layout="LEFT" colCount="1" sameSz="1" sameGap="0"/></hp:ctrl></hp:run></hp:p>`)
	for _, l := range lines {
		sec.WriteString(`<hp:p id="0" paraPrIDRef="0" styleIDRef="0" pageBreak="0" columnBreak="0" merged="0"><hp:run charPrIDRef="0"><hp:t>`)
		var esc bytes.Buffer
		if err := xml.EscapeText(&esc, []byte(l)); err != nil {
			return nil, err
		}
		sec.Write(esc.Bytes())
		sec.WriteString(`</hp:t></hp:run></hp:p>`)
	}
	sec.WriteString(`</hs:sec>`)
	hpf := strings.Replace(contentHPF, "<opf:title>조치완료보고서</opf:title>", "<opf:title>"+xmlEsc(title)+"</opf:title>", 1)
	return packHWPX([]hwpxEntry{
		{Name: "mimetype", Body: []byte("application/hwp+zip")},
		{Name: "version.xml", Body: []byte(versionXML)},
		{Name: "settings.xml", Body: []byte(settingsXML)},
		{Name: "META-INF/container.xml", Body: []byte(containerXML)},
		{Name: "META-INF/container.rdf", Body: []byte(containerRDF)},
		{Name: "META-INF/manifest.xml", Body: []byte(manifestXML)},
		{Name: "Contents/content.hpf", Body: []byte(hpf)},
		{Name: "Contents/header.xml", Body: hangulHeaderXML},
		{Name: "Contents/section0.xml", Body: sec.Bytes()},
		{Name: "Preview/PrvText.txt", Body: encodeUTF16LE(strings.Join(lines, "\n"))},
		{Name: "Preview/PrvImage.png", Body: previewPNG()},
	})
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "\n", " ")), " ")
}

func xmlEsc(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

const sectionOpen = `<hs:sec xmlns:ha="http://www.hancom.co.kr/hwpml/2011/app" xmlns:hp="http://www.hancom.co.kr/hwpml/2011/paragraph" xmlns:hp10="http://www.hancom.co.kr/hwpml/2016/paragraph" xmlns:hs="http://www.hancom.co.kr/hwpml/2011/section" xmlns:hc="http://www.hancom.co.kr/hwpml/2011/core" xmlns:hh="http://www.hancom.co.kr/hwpml/2011/head" xmlns:hhs="http://www.hancom.co.kr/hwpml/2011/history" xmlns:hm="http://www.hancom.co.kr/hwpml/2011/master-page" xmlns:hpf="http://www.hancom.co.kr/schema/2011/hpf" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf/" xmlns:ooxmlchart="http://www.hancom.co.kr/hwpml/2016/ooxmlchart" xmlns:hwpunitchar="http://www.hancom.co.kr/hwpml/2016/HwpUnitChar" xmlns:epub="http://www.idpf.org/2007/ops" xmlns:config="urn:oasis:names:tc:opendocument:xmlns:config:1.0">`
