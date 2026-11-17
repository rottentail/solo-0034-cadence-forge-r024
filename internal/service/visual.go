package service

import (
	"cadenceforge/internal/domain"
	"cadenceforge/internal/inspect"
	"cadenceforge/internal/media"
	"context"
)

func (s *Service) Visual(ctx context.Context, id, mode string) ([]byte, error) {
	state, err := s.Store.Snapshot()
	if err != nil {
		return nil, err
	}
	source, ok := state.Sources[id]
	if !ok {
		return nil, domain.Missing("source", id)
	}
	path, err := s.Blobs.Object(source.Digest)
	if err != nil {
		return nil, err
	}
	digest, n, err := inspect.Digest(path)
	if err != nil {
		return nil, domain.Env("verify", "source digest read failed", err)
	}
	if digest != source.Digest || n != source.Bytes {
		return nil, domain.Env("verify", "source content integrity failed", nil)
	}
	return media.Visual(ctx, s.Config.FFmpeg, path, mode, source.Media)
}
