package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCSVFormulaSafety(t *testing.T) {
	for _, s := range []string{"=HYPERLINK(1)", "+SUM(A1)", "-1+1", "@SUM(A1)"} {
		if !strings.HasPrefix(safeCell(s), "'") {
			t.Fatal(s)
		}
	}
	if safeCell("407-123") != "407-123" {
		t.Fatal("changed safe order ID")
	}
}
func TestAuthAndOriginChecks(t *testing.T) {
	s := &Server{Password: "test-password-for-suite"}
	h := s.Handler()
	for _, tc := range []struct {
		method, path, origin string
		want                 int
	}{{"GET", "/api/results", "", 401}, {"POST", "/api/login", "https://other.example", 403}, {"POST", "/api/login", "", 401}} {
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{"password":"wrong"}`))
		r.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatal(tc, w.Code)
		}
	}
	r := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader("bad"))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
}
