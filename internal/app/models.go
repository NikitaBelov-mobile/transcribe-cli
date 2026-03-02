package app

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// PresetModel defines a known downloadable whisper model.
type PresetModel struct {
	Alias string `json:"alias"`
	Name  string `json:"name"`
	URL   string `json:"url"`
}

var presetModels = []PresetModel{
	{Alias: "tiny", Name: "ggml-tiny", URL: "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-tiny.bin"},
	{Alias: "base", Name: "ggml-base", URL: "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.bin"},
	{Alias: "small", Name: "ggml-small", URL: "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.bin"},
	{Alias: "medium", Name: "ggml-medium", URL: "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-medium.bin"},
	{Alias: "large", Name: "ggml-large-v3", URL: "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-large-v3.bin"},
}

var presetIndex = buildPresetIndex()

func buildPresetIndex() map[string]PresetModel {
	index := make(map[string]PresetModel, len(presetModels)*2)
	for _, preset := range presetModels {
		index[strings.ToLower(preset.Alias)] = preset
		index[strings.ToLower(preset.Name)] = preset
	}
	index["large-v3"] = presetIndexValue("large")
	return index
}

func presetIndexValue(key string) PresetModel {
	for _, preset := range presetModels {
		if preset.Alias == key {
			return preset
		}
	}
	return PresetModel{}
}

func ListPresetModels() []PresetModel {
	out := append([]PresetModel(nil), presetModels...)
	sort.Slice(out, func(i, j int) bool {
		return out[i].Alias < out[j].Alias
	})
	return out
}

func LookupPresetModel(name string) (PresetModel, bool) {
	name = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(name, ".bin")))
	preset, ok := presetIndex[name]
	return preset, ok
}

// CanonicalModelName normalizes aliases to ggml-* model names.
func CanonicalModelName(name string) string {
	name = strings.TrimSpace(strings.TrimSuffix(name, ".bin"))
	if name == "" {
		return ""
	}
	if preset, ok := LookupPresetModel(name); ok {
		return preset.Name
	}
	return name
}

// ModelInfo describes a downloaded local model.
type ModelInfo struct {
	Name       string
	Path       string
	SizeBytes  int64
	ModifiedAt time.Time
}

func ListModels(cfg Config) ([]ModelInfo, error) {
	if err := EnsureStateDirs(cfg); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(cfg.ModelsDir)
	if err != nil {
		return nil, err
	}
	models := make([]ModelInfo, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) != ".bin" {
			continue
		}
		path := filepath.Join(cfg.ModelsDir, name)
		info, err := entry.Info()
		if err != nil {
			continue
		}
		models = append(models, ModelInfo{
			Name:       strings.TrimSuffix(name, filepath.Ext(name)),
			Path:       path,
			SizeBytes:  info.Size(),
			ModifiedAt: info.ModTime(),
		})
	}
	sort.Slice(models, func(i, j int) bool {
		return models[i].Name < models[j].Name
	})
	return models, nil
}

func InstallModel(cfg Config, name, url string, out io.Writer) (string, error) {
	if err := EnsureStateDirs(cfg); err != nil {
		return "", err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("model name is required")
	}

	canonical := CanonicalModelName(name)
	if canonical == "" {
		return "", fmt.Errorf("model name is required")
	}

	if strings.TrimSpace(url) == "" {
		if preset, ok := LookupPresetModel(canonical); ok {
			url = preset.URL
			canonical = preset.Name
		} else {
			return "", fmt.Errorf("unknown preset model %q; use `transcribe model presets` or provide --url", name)
		}
	}

	finalPath := filepath.Join(cfg.ModelsDir, canonical+".bin")
	tmpPath := finalPath + ".part"

	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	file, err := os.Create(tmpPath)
	if err != nil {
		return "", err
	}

	progress := &progressWriter{out: out, total: resp.ContentLength}
	if _, err := io.Copy(file, io.TeeReader(resp.Body, progress)); err != nil {
		file.Close()
		_ = os.Remove(tmpPath)
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return "", err
	}
	if out != nil {
		_, _ = io.WriteString(out, "\n")
	}
	return finalPath, nil
}

func RemoveModel(cfg Config, name string) error {
	name = CanonicalModelName(name)
	if name == "" {
		return fmt.Errorf("model name is required")
	}
	path := filepath.Join(cfg.ModelsDir, name+".bin")
	if err := os.Remove(path); err != nil {
		return err
	}
	return nil
}

func ResolveModelPath(cfg Config, model string) (string, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		model = cfg.DefaultModel
	}
	if model == "" {
		model = "ggml-base"
	}

	if filepath.IsAbs(model) || strings.ContainsRune(model, os.PathSeparator) {
		if _, err := os.Stat(model); err != nil {
			return "", fmt.Errorf("model not found: %s", model)
		}
		return model, nil
	}

	canonical := CanonicalModelName(model)
	path := filepath.Join(cfg.ModelsDir, canonical+".bin")
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("model not found: %s (install with: transcribe model install --name %s)", path, canonical)
	}
	return path, nil
}

type progressWriter struct {
	out        io.Writer
	total      int64
	received   int64
	lastRender time.Time
}

func (w *progressWriter) Write(p []byte) (int, error) {
	n := len(p)
	w.received += int64(n)
	if w.out != nil {
		now := time.Now()
		if w.lastRender.IsZero() || now.Sub(w.lastRender) > 500*time.Millisecond {
			w.lastRender = now
			if w.total > 0 {
				percent := float64(w.received) / float64(w.total) * 100
				_, _ = fmt.Fprintf(w.out, "\rDownloading model... %.1f%%", percent)
			} else {
				_, _ = fmt.Fprintf(w.out, "\rDownloading model... %.1f MB", float64(w.received)/1024.0/1024.0)
			}
		}
	}
	return n, nil
}
