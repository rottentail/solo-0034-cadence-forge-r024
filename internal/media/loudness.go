package media

import (
	"cadenceforge/internal/domain"
	"context"
	"encoding/json"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type Loudness struct {
	IntegratedLUFS *float64 `json:"integrated_lufs"`
	TruePeakDBFS   *float64 `json:"true_peak_dbfs"`
	RangeLU        float64  `json:"range_lu"`
	Silent         bool     `json:"silent"`
}

func finite(s string) *float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || strings.Contains(strings.ToLower(s), "inf") || strings.Contains(strings.ToLower(s), "nan") {
		return nil
	}
	return &v
}
func Measure(ctx context.Context, binary, path string) (Loudness, error) {
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "-nostdin", "-hide_banner", "-protocol_whitelist", "file,pipe", "-format_whitelist", Formats, "-i", path, "-vn", "-af", "loudnorm=I=-16:TP=-1.5:LRA=11:print_format=json", "-f", "null", "-")
	var log LimitedBuffer
	log.Limit = 32768
	cmd.Stderr = &log
	if err := cmd.Run(); err != nil {
		return Loudness{}, domain.Env("measure", "loudness scan failed", &ProcessError{Err: err, Stderr: log.String(), Ctx: ctx.Err()})
	}
	text := log.String()
	start := strings.LastIndex(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end < start {
		return Loudness{}, domain.Env("measure", "loudness output missing", nil)
	}
	var raw struct {
		I   string `json:"input_i"`
		TP  string `json:"input_tp"`
		LRA string `json:"input_lra"`
	}
	if err := json.Unmarshal([]byte(text[start:end+1]), &raw); err != nil {
		return Loudness{}, domain.Env("measure", "loudness output is not valid JSON", err)
	}
	l := Loudness{IntegratedLUFS: finite(raw.I), TruePeakDBFS: finite(raw.TP)}
	l.Silent = l.IntegratedLUFS == nil
	if v := finite(raw.LRA); v != nil {
		l.RangeLU = *v
	}
	return l, nil
}
