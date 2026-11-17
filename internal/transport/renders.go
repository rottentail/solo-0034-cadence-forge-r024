package transport

import (
	"cadenceforge/internal/domain"
	"cadenceforge/internal/service"
	"net/http"
	"os"
	"path/filepath"
)

func (a API) submit(w http.ResponseWriter, r *http.Request) {
	var input service.Request
	if err := decode(w, r, &input); err != nil {
		failure(w, err)
		return
	}
	v, err := a.Service.Submit(r.Header.Get("Idempotency-Key"), input)
	if err != nil {
		failure(w, err)
		return
	}
	write(w, 202, v)
}
func (a API) render(w http.ResponseWriter, r *http.Request) {
	st, err := a.Service.Store.Snapshot()
	if err != nil {
		failure(w, err)
		return
	}
	v, ok := st.Renders[r.PathValue("id")]
	if !ok {
		failure(w, domain.Missing("render", r.PathValue("id")))
		return
	}
	write(w, 200, v)
}
func (a API) cancel(w http.ResponseWriter, r *http.Request) {
	v, err := a.Service.Cancel(r.PathValue("id"))
	if err != nil {
		failure(w, err)
		return
	}
	write(w, 200, v)
}
func (a API) retry(w http.ResponseWriter, r *http.Request) {
	v, err := a.Service.Retry(r.PathValue("id"))
	if err != nil {
		failure(w, err)
		return
	}
	write(w, 202, v)
}
func (a API) verify(w http.ResponseWriter, r *http.Request) {
	v, err := a.Service.Verify(r.Context(), r.PathValue("id"))
	if err != nil {
		failure(w, err)
		return
	}
	write(w, 200, v)
}
func (a API) artifact(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := a.Service.Verify(r.Context(), id); err != nil {
		failure(w, err)
		return
	}
	render, dir, err := a.Service.Artifact(id)
	if err != nil {
		failure(w, err)
		return
	}
	name := render.Artifact.Filename
	if r.URL.Query().Get("captions") == "1" {
		name = render.Artifact.CaptionFile
		w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	}
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	http.ServeFile(w, r, filepath.Join(dir, name))
}
func (a API) bundle(w http.ResponseWriter, r *http.Request) {
	f, err := os.CreateTemp("", "cadence-bundle-*.zip")
	if err != nil {
		failure(w, domain.Env("bundle", "bundle workspace failed", err))
		return
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err = a.Service.Bundle(r.Context(), r.PathValue("id"), f); err != nil {
		failure(w, err)
		return
	}
	if _, err = f.Seek(0, 0); err != nil {
		failure(w, domain.Env("bundle", "bundle rewind failed", err))
		return
	}
	info, err := f.Stat()
	if err != nil {
		failure(w, domain.Env("bundle", "bundle stat failed", err))
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="composition.zip"`)
	http.ServeContent(w, r, "composition.zip", info.ModTime(), f)
}
