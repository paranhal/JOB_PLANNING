package handler

import (
	"net/http"
	"strings"
	"testing"
)

func TestSalesShowMissingRedirectsNot500(t *testing.T) {
	e := newSalesServer(t)
	rec := doGet(t, e, "/sales/NO-SUCH-ID")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "err=sales_missing") {
		t.Fatalf("loc=%s", loc)
	}
}
