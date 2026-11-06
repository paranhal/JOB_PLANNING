package handler

import (
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/dbprof"
)

const mntProfCtxKey = "mnt_profile"

type mntSpan struct {
	name string
	dur  time.Duration
}

type mntProf struct {
	on      bool
	start   time.Time
	last    time.Time
	queries int64
	spans   []mntSpan
}

func mntProfFrom(c echo.Context) *mntProf {
	if c == nil {
		return &mntProf{}
	}
	if v, ok := c.Get(mntProfCtxKey).(*mntProf); ok && v != nil {
		return v
	}
	return &mntProf{}
}

func (p *mntProf) mark(name string) {
	if p == nil || !p.on {
		return
	}
	now := time.Now()
	p.spans = append(p.spans, mntSpan{name: name, dur: now.Sub(p.last)})
	p.last = now
}

func (p *mntProf) headerValue() string {
	if p == nil || !p.on {
		return ""
	}
	total := time.Since(p.start)
	parts := []string{
		fmt.Sprintf("total;dur=%.1f", ms(total)),
		fmt.Sprintf("queries;dur=0;desc=\"n=%d\"", atomic.LoadInt64(&p.queries)),
	}
	for _, s := range p.spans {
		parts = append(parts, fmt.Sprintf("%s;dur=%.1f", s.name, ms(s.dur)))
	}
	return strings.Join(parts, ", ")
}

func ms(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}

type mntProfWriter struct {
	http.ResponseWriter
	p     *mntProf
	wrote bool
}

func (w *mntProfWriter) WriteHeader(code int) {
	if !w.wrote {
		if hv := w.p.headerValue(); hv != "" {
			w.Header().Set("Server-Timing", hv)
		}
		w.wrote = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *mntProfWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

func (w *mntProfWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *mntProfWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// MntProfileMiddleware MNT_PROFILE=1 일 때만 Server-Timing 과 요청당 쿼리 수를 남긴다.
func MntProfileMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if !dbprof.Enabled() {
			return next(c)
		}
		path := c.Request().URL.Path
		if !strings.HasPrefix(path, "/maintenance") {
			return next(c)
		}
		p := &mntProf{on: true, start: time.Now(), last: time.Now()}
		dbprof.Begin(&p.queries)
		c.Set(mntProfCtxKey, p)
		c.Response().Writer = &mntProfWriter{ResponseWriter: c.Response().Writer, p: p}
		defer dbprof.End()
		return next(c)
	}
}
