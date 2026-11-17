package transport

import (
	"cadenceforge/internal/domain"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
)

func write(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Warn("response write", "error", err)
	}
}
func failure(w http.ResponseWriter, err error) {
	status, code, message := domain.Classify(err)
	if status >= 500 {
		if f := domain.FailureOf(err); f != nil {
			slog.Error("request failed", "kind", string(f.Kind), "stage", f.Stage, "error", err)
		} else {
			slog.Error("request failed", "error", err)
		}
	}
	write(w, status, map[string]string{"error": code, "message": message})
}
func decode(w http.ResponseWriter, r *http.Request, dest any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(dest); err != nil {
		return domain.Invalid("invalid JSON: %s", err)
	}
	if err := d.Decode(&struct{}{}); err != io.EOF {
		return domain.Invalid("request requires exactly one JSON object")
	}
	return nil
}
