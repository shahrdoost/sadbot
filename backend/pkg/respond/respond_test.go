package respond

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJSON_SetsStatusAndContentType(t *testing.T) {
	rec := httptest.NewRecorder()
	JSON(rec, http.StatusCreated, map[string]string{"hello": "دنبال"})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Fatalf("content-type = %q", ct)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if body["hello"] != "دنبال" {
		t.Fatalf("body = %v", body)
	}
}

func TestJSON_NilBody(t *testing.T) {
	rec := httptest.NewRecorder()
	JSON(rec, http.StatusNoContent, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("expected empty body, got %q", rec.Body.String())
	}
}

func TestError_Shape(t *testing.T) {
	rec := httptest.NewRecorder()
	Error(rec, http.StatusForbidden, "دسترسی ندارید")

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if body["error"] != "دسترسی ندارید" {
		t.Fatalf("error = %q", body["error"])
	}
}
