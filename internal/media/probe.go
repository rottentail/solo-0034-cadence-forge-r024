package media

import (
	"cadenceforge/internal/domain"
	"context"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"time"
)

type Prober struct{ Binary string }

// Rejection marks media bytes the processing contract cannot accept: the
// unprocessable-input class, distinct from runtime environment failures.
type Rejection struct{ Reason string }

func (r *Rejection) Error() string { return r.Reason }

func (p Prober) Probe(ctx context.Context, path string) (domain.Media, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	data, err := Run(ctx, p.Binary, []string{"-v", "error", "-protocol_whitelist", "file,pipe", "-format_whitelist", Formats, "-show_format", "-show_streams", "-of", "json", path}, 1<<20)
	if err != nil {
		var pe *ProcessError
		if errors.As(err, &pe) && pe.Exited() {
			return domain.Media{}, &Rejection{Reason: pe.Error()}
		}
		return domain.Media{}, domain.Env("probe", "media probe failed", err)
	}
	var raw struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
		Streams []struct {
			Type        string `json:"codec_type"`
			Codec       string `json:"codec_name"`
			Width       int    `json:"width"`
			Height      int    `json:"height"`
			Rate        string `json:"sample_rate"`
			Channels    int    `json:"channels"`
			Duration    string `json:"duration"`
			Disposition struct {
				Attached int `json:"attached_pic"`
			} `json:"disposition"`
		} `json:"streams"`
	}
	if err = json.Unmarshal(data, &raw); err != nil {
		return domain.Media{}, domain.Env("probe", "probe output is not valid JSON", err)
	}
	duration, _ := strconv.ParseFloat(raw.Format.Duration, 64)
	m := domain.Media{}
	for _, s := range raw.Streams {
		if s.Type == "video" && s.Disposition.Attached == 0 && !m.Video {
			m.Video = true
			m.Width = s.Width
			m.Height = s.Height
			m.VideoCodec = s.Codec
		}
		if s.Type == "audio" && !m.Audio {
			m.Audio = true
			m.SampleRate, _ = strconv.Atoi(s.Rate)
			m.Channels = s.Channels
			m.AudioCodec = s.Codec
		}
		if duration == 0 {
			d, _ := strconv.ParseFloat(s.Duration, 64)
			if d > duration {
				duration = d
			}
		}
	}
	if math.IsNaN(duration) || math.IsInf(duration, 0) || duration <= 0 || duration > 121 {
		return m, &Rejection{Reason: "media duration must be >0 and <=121 seconds"}
	}
	if !m.Video && !m.Audio {
		return m, &Rejection{Reason: "no usable audio or video stream"}
	}
	if m.Video && (m.Width < 1 || m.Height < 1 || m.Width > 3840 || m.Height > 2160) {
		return m, &Rejection{Reason: "video exceeds processing dimensions"}
	}
	if m.Audio && (m.Channels < 1 || m.Channels > 8 || m.SampleRate < 8000 || m.SampleRate > 192000) {
		return m, &Rejection{Reason: "unsupported audio layout"}
	}
	m.DurationMS = int64(math.Round(duration * 1000))
	return m, nil
}
