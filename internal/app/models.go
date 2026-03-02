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

// PresetModels maps shortcut names to public ggml model URLs.
var PresetModels = map[string]string{
	"tiny":   "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-tiny.bin",
	"base":   "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.bin",
	"small":  "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.bin",
	"medium": "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-medium.bin",
	"large":  "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-large-v3.bin",
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
	name = normalizeModelName(name)
	if name == "" {
		return "", fmt.Errorf("model name is required")
	}
	if strings.TrimSpace(url) == "" {
		if presetURL, ok := PresetModels[name]; ok {
			url = presetURL
		} else {
			return "", fmt.Errorf("URL is required for non-preset model")
		}
	}

	finalPath := filepath.Join(cfg.ModelsDir, name+".bin")
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
	name = normalizeModelName(name)
	if name == "" {
		return fmt.Errorf("model name is required")
	}
	path := filepath.Join(cfg.ModelsDir, name+".bin")
	if err := os.Remove(path); err != nil {
		return err
	}
	return nil
}

func normalizeModelName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimSuffix(name, ".bin")
	return name
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
