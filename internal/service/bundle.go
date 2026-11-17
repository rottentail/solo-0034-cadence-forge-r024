package service

import (
	"archive/zip"
	"cadenceforge/internal/domain"
	"cadenceforge/internal/inspect"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
)

func (s *Service) Artifact(id string) (domain.Render, string, error) {
	st, err := s.Store.Snapshot()
	if err != nil {
		return domain.Render{}, "", err
	}
	r, ok := st.Renders[id]
	if !ok {
		return r, "", domain.Missing("render", id)
	}
	if r.State != "succeeded" || r.Artifact == nil {
		return r, "", domain.Conflict("render has no committed artifact")
	}
	dir, err := s.Blobs.RenderDir(id, len(r.Attempts))
	return r, dir, err
}
func (s *Service) Verify(ctx context.Context, id string) (domain.Inspection, error) {
	r, dir, err := s.Artifact(id)
	if err != nil {
		return domain.Inspection{}, err
	}
	in, err := inspect.Artifact(ctx, s.Probe, filepath.Join(dir, r.Artifact.Filename), r)
	if err != nil {
		return in, err
	}
	if in.Digest != r.Artifact.Inspection.Digest || in.Bytes != r.Artifact.Inspection.Bytes {
		return in, domain.Env("verify", "committed artifact integrity failed", nil)
	}
	captions, err := os.ReadFile(filepath.Join(dir, r.Artifact.CaptionFile))
	if err != nil {
		return in, domain.Env("verify", "committed captions unreadable", err)
	}
	if string(captions) != r.Plan.VTT {
		return in, domain.Env("verify", "caption integrity failed", nil)
	}
	return in, nil
}
func (s *Service) Bundle(ctx context.Context, id string, dest io.Writer) error {
	if _, err := s.Verify(ctx, id); err != nil {
		return err
	}
	r, dir, err := s.Artifact(id)
	if err != nil {
		return err
	}
	z := zip.NewWriter(dest)
	for _, name := range []string{r.Artifact.Filename, r.Artifact.CaptionFile} {
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			z.Close()
			return domain.Env("bundle", "artifact open failed", err)
		}
		w, err := z.Create(name)
		if err == nil {
			_, err = io.Copy(w, f)
		}
		f.Close()
		if err != nil {
			z.Close()
			return domain.Env("bundle", "artifact packing failed", err)
		}
	}
	manifest, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		z.Close()
		return domain.Env("bundle", "manifest encoding failed", err)
	}
	w, err := z.Create("composition.json")
	if err != nil {
		z.Close()
		return domain.Env("bundle", "manifest packing failed", err)
	}
	if _, err = w.Write(manifest); err != nil {
		z.Close()
		return domain.Env("bundle", "manifest packing failed", err)
	}
	if err = z.Close(); err != nil {
		return domain.Env("bundle", "bundle finalize failed", err)
	}
	return nil
}
