package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// §39.3 · §39.8 5~7: 조치 화면 첨부는 한 번, 라이트박스 id 하나, click·dblclick 확대.
func TestASActionAttachIncludedOnceAndZoomHooks(t *testing.T) {
	root := findTemplateRoot(t)
	src, err := os.ReadFile(filepath.Join(root, "as", "action.html"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(src), `{{template "as_action_attach" .}}`) != 1 {
		t.Fatalf("action.html 의 as_action_attach 호출 %d회 — 한 번이어야 한다",
			strings.Count(string(src), `{{template "as_action_attach" .}}`))
	}
	if strings.Count(string(src), `{{template "photo_zoom_script"`) != 1 {
		t.Fatal("조치 화면 바닥에 photo_zoom_script 가 한 번 있어야 한다")
	}

	for _, rel := range []string{"as/_action_attach.html", "as/_photo_gallery.html"} {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "<script") {
			t.Errorf("%s 부분 템플릿에 <script> 가 있다", rel)
		}
	}

	e, _, _, _, asID := newASActionFixture(t)
	png := makePNG(t, 40, 20)
	if rec := postReceiptPhoto(t, e, asID, "증상.png", png); rec.Code != http.StatusSeeOther {
		t.Fatalf("접수 사진 업로드: status=%d", rec.Code)
	}
	if rec := postActionPhotoEcho(t, e, asID, "조치.png", "교체 후", png); rec.Code != http.StatusSeeOther {
		t.Fatalf("조치 사진 업로드: status=%d", rec.Code)
	}

	page := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/"+asID+"/action", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(page, req)
	if page.Code != http.StatusOK {
		t.Fatalf("조치 화면: status=%d", page.Code)
	}
	body := page.Body.String()
	if strings.Count(body, `data-action-attach="`) != 1 {
		t.Fatalf("data-action-attach %d개", strings.Count(body, `data-action-attach="`))
	}
	if got := strings.Count(body, `id="action-attach-lightbox"`); got != 1 {
		t.Fatalf("#action-attach-lightbox %d개 want 1", got)
	}
	if !strings.Contains(body, `data-photo-gallery="receipt"`) {
		t.Error("접수 사진 영역이 없다")
	}
	if strings.Count(body, `data-photo-zoom>`) < 2 {
		t.Fatal("접수·조치 사진 확대 루트가 둘 다 있어야 한다")
	}
	if !strings.Contains(body, `data-photo-lightbox id="action-attach-lightbox"`) {
		t.Fatal("조치 사진 라이트박스가 없다")
	}
	if !strings.Contains(body, `data-photo-lightbox class=`) {
		t.Fatal("접수 사진 라이트박스가 없다")
	}
	if !strings.Contains(body, `addEventListener('dblclick'`) && !strings.Contains(body, `addEventListener("dblclick"`) {
		t.Error("dblclick 리스너가 없다")
	}
	if !strings.Contains(body, `addEventListener('click'`) && !strings.Contains(body, `addEventListener("click"`) {
		t.Error("click 리스너가 없다")
	}
	if !strings.Contains(body, `closest('[data-photo-zoom]')`) && !strings.Contains(body, `closest("[data-photo-zoom]")`) {
		t.Error("closest([data-photo-zoom]) 가 없다")
	}
	if strings.Count(body, "window.__photoZoomBound") < 1 {
		t.Error("확대 스크립트 중복 바인딩 가드가 없다")
	}
}

func TestPartialScriptTemplatesIncludedAtMostOnce(t *testing.T) {
	root := findTemplateRoot(t)
	defineRe := regexp.MustCompile(`\{\{define "([^"]+)"\}\}`)
	callRe := regexp.MustCompile(`\{\{template "([^"]+)"`)

	scriptDefines := map[string]string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasPrefix(info.Name(), "_") || !strings.HasSuffix(path, ".html") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		src := string(b)
		if !strings.Contains(src, "<script") {
			return nil
		}
		for _, m := range defineRe.FindAllStringSubmatch(src, -1) {
			name := m[1]
			chunkStart := strings.Index(src, m[0])
			rest := src[chunkStart:]
			next := defineRe.FindStringIndex(rest[len(m[0]):])
			chunk := rest
			if next != nil {
				chunk = rest[:len(m[0])+next[0]]
			}
			if strings.Contains(chunk, "<script") {
				scriptDefines[name] = path
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(scriptDefines) == 0 {
		t.Fatal("스크립트를 담은 부분 템플릿을 못 찾았다")
	}

	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || strings.HasPrefix(info.Name(), "_") || !strings.HasSuffix(path, ".html") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		src := string(b)
		counts := map[string]int{}
		for _, m := range callRe.FindAllStringSubmatch(src, -1) {
			counts[m[1]]++
		}
		rel, _ := filepath.Rel(root, path)
		for name, defPath := range scriptDefines {
			if counts[name] > 1 {
				t.Errorf("%s 가 {{template %q}} 를 %d번 넣는다 (%s 안에 <script>)",
					filepath.ToSlash(rel), name, counts[name], filepath.ToSlash(defPath))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
