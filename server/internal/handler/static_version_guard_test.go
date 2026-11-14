package handler

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// 95-198 재발 방지: /static/ 아래 js·mjs·css 참조에는 반드시 ?v= 가 붙어야 한다.
// /static/ 응답은 하루(max-age=86400) 캐시되므로, 쿼리가 없으면 배포 뒤에도 브라우저가 옛 스크립트를 쓴다
// (새 HTML + 옛 JS 불일치). 부분 템플릿(dict 로 호출)에는 .AssetQuery 가 없으므로 {{assetQ}} 를 쓴다.
// 예외 허용 목록은 비워 둔다. 꼭 필요하면 아래 map 에 "파일:줄 → 사유" 로 적는다.
var staticVersionExceptions = map[string]string{}

var staticRefRe = regexp.MustCompile(`/static/[A-Za-z0-9_/.\-]*\.(js|mjs|css)(\?[^"'\s>]*)?`)

func TestStaticAssetsAlwaysVersioned(t *testing.T) {
	root := findTemplateRoot(t)
	var bad []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".html") {
			return err
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		rel, _ := filepath.Rel(root, p)
		for i, line := range strings.Split(string(b), "\n") {
			for _, m := range staticRefRe.FindAllString(line, -1) {
				if strings.Contains(m, "?v=") {
					continue
				}
				key := rel + ":" + itoa95198(i+1)
				if _, ok := staticVersionExceptions[key]; ok {
					continue
				}
				bad = append(bad, key+" "+m)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(bad) > 0 {
		t.Fatalf("?v= 없는 정적 파일 참조 %d곳 (배포 뒤 옛 JS 캐시 위험):\n  %s", len(bad), strings.Join(bad, "\n  "))
	}
}

func itoa95198(n int) string {
	if n == 0 {
		return "0"
	}
	var d []byte
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	return string(d)
}
