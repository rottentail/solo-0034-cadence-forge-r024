package encoding

import (
	"cadenceforge/internal/blob"
	"cadenceforge/internal/domain"
	"cadenceforge/internal/media"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Runner struct {
	Binary string
	Blobs  *blob.Store
}

func (r Runner) Encode(ctx context.Context, render domain.Render, stage string) (string, error) {
	paths := map[string]string{}
	for id, s := range render.Sources {
		path, err := r.Blobs.Object(s.Digest)
		if err != nil {
			return "", err
		}
		if _, err = os.Stat(path); err != nil {
			return "", domain.Env("encode", fmt.Sprintf("source %s unavailable", id), err)
		}
		paths[id] = path
	}
	name := "composition." + render.Recipe.Format
	args, err := Arguments(render, paths, filepath.Join(stage, name))
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	if _, err = media.Run(ctx, r.Binary, args, 65536); err != nil {
		return "", domain.Env("encode", "media encode failed", err)
	}
	if err = os.WriteFile(filepath.Join(stage, "captions.vtt"), []byte(render.Plan.VTT), 0600); err != nil {
		return "", domain.Env("encode", "caption file write failed", err)
	}
	return name, nil
}
