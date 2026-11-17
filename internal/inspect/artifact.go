package inspect

import (
	"cadenceforge/internal/domain"
	"cadenceforge/internal/media"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"os"
)

func Digest(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", n, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}
func Artifact(ctx context.Context, prober media.Prober, path string, r domain.Render) (domain.Inspection, error) {
	m, err := prober.Probe(ctx, path)
	if err != nil {
		return domain.Inspection{}, domain.Env("inspect", "artifact probe failed", err)
	}
	delta := m.DurationMS - r.Plan.DurationMS
	tolerance := int64(100)
	if r.Recipe.Format == "mp4" {
		tolerance = max(tolerance, int64(2000/r.Recipe.FPS))
	}
	if math.Abs(float64(delta)) > float64(tolerance) {
		return domain.Inspection{}, domain.Env("inspect", fmt.Sprintf("output duration delta %dms exceeds %dms", delta, tolerance), nil)
	}
	if !m.Audio || m.SampleRate != r.Recipe.SampleRate {
		return domain.Inspection{}, domain.Env("inspect", "output audio contract mismatch", nil)
	}
	if r.Recipe.Format == "mp4" && (!m.Video || m.Width != r.Recipe.Width || m.Height != r.Recipe.Height) {
		return domain.Inspection{}, domain.Env("inspect", "output video contract mismatch", nil)
	}
	digest, n, err := Digest(path)
	if err != nil {
		return domain.Inspection{}, domain.Env("inspect", "artifact digest failed", err)
	}
	return domain.Inspection{Media: m, Digest: digest, Bytes: n, DurationDeltaMS: delta}, nil
}
