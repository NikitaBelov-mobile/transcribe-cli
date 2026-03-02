package app

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
)

// Config defines runtime settings for the CLI and local daemon.
type Config struct {
	Addr          string
	Workers       int
	QueueSize     int
	StateDir      string
	JobsFile      string
	ModelsDir     string
	FFmpegBinary  string
	WhisperBinary string
	ClientBaseURL string
}

func LoadConfig() Config {
	stateDir := getEnv("TRANSCRIBE_CLI_STATE_DIR", defaultStateDir())
	modelsDir := filepath.Join(stateDir, "models")
	addr := getEnv("TRANSCRIBE_CLI_ADDR", "127.0.0.1:9864")
	cfg := Config{
		Addr:          addr,
		Workers:       getEnvInt("TRANSCRIBE_CLI_WORKERS", max(1, runtime.NumCPU()/2)),
		QueueSize:     getEnvInt("TRANSCRIBE_CLI_QUEUE_SIZE", max(8, runtime.NumCPU()*2)),
		StateDir:      stateDir,
		JobsFile:      filepath.Join(stateDir, "jobs.json"),
		ModelsDir:     getEnv("TRANSCRIBE_CLI_MODELS_DIR", modelsDir),
		FFmpegBinary:  getEnv("TRANSCRIBE_CLI_FFMPEG", "ffmpeg"),
		WhisperBinary: getEnv("TRANSCRIBE_CLI_WHISPER", "whisper-cli"),
		ClientBaseURL: "http://" + addr,
	}
	if cfg.Workers <= 0 {
		cfg.Workers = 1
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 8
	}
	return cfg
}

func EnsureStateDirs(cfg Config) error {
	if err := os.MkdirAll(cfg.StateDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.ModelsDir, 0o755); err != nil {
		return err
	}
	return nil
}

func defaultStateDir() string {
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		return ".transcribe-cli"
	}
	return filepath.Join(base, "TranscribeCLI")
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if value := os.Getenv(key); value != "" {
		if n, err := strconv.Atoi(value); err == nil {
			return n
		}
	}
	return fallback
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
