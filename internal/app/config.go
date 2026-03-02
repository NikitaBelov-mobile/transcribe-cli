package app

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// Config defines runtime settings for the CLI and local daemon.
type Config struct {
	Addr          string
	Workers       int
	QueueSize     int
	StateDir      string
	BinDir        string
	UpdatesDir    string
	JobsFile      string
	SettingsFile  string
	ModelsDir     string
	UploadsDir    string
	OutputsDir    string
	DefaultModel  string
	FFmpegBinary  string
	WhisperBinary string
	ReleaseRepo   string
	AppVersion    string
	ClientBaseURL string
}

func LoadConfig() Config {
	stateDir := getEnv("TRANSCRIBE_CLI_STATE_DIR", defaultStateDir())
	settingsFile := filepath.Join(stateDir, "config.json")
	modelsDir := filepath.Join(stateDir, "models")
	binDir := filepath.Join(stateDir, "bin")
	updatesDir := filepath.Join(stateDir, "updates")
	addr := getEnv("TRANSCRIBE_CLI_ADDR", "127.0.0.1:9864")
	releaseRepo := getEnv("TRANSCRIBE_CLI_RELEASE_REPO", "NikitaBelov-mobile/transcribe-cli")

	defaultModel := strings.TrimSpace(getEnv("TRANSCRIBE_CLI_DEFAULT_MODEL", ""))
	settings, _ := LoadSettings(settingsFile)
	if defaultModel == "" {
		if strings.TrimSpace(settings.DefaultModel) != "" {
			defaultModel = settings.DefaultModel
		}
	}
	if defaultModel == "" {
		defaultModel = "ggml-base"
	}

	ffmpegBinary := strings.TrimSpace(getEnv("TRANSCRIBE_CLI_FFMPEG", ""))
	if ffmpegBinary == "" {
		ffmpegBinary = strings.TrimSpace(settings.FFmpegBinary)
	}
	if ffmpegBinary == "" {
		ffmpegBinary = localToolPath(stateDir, "ffmpeg")
		if !fileExists(ffmpegBinary) {
			ffmpegBinary = "ffmpeg"
		}
	}

	whisperBinary := strings.TrimSpace(getEnv("TRANSCRIBE_CLI_WHISPER", ""))
	if whisperBinary == "" {
		whisperBinary = strings.TrimSpace(settings.WhisperBinary)
	}
	if whisperBinary == "" {
		whisperBinary = localToolPath(stateDir, "whisper-cli")
		if !fileExists(whisperBinary) {
			whisperBinary = "whisper-cli"
		}
	}

	cfg := Config{
		Addr:          addr,
		Workers:       getEnvInt("TRANSCRIBE_CLI_WORKERS", max(1, runtime.NumCPU()/2)),
		QueueSize:     getEnvInt("TRANSCRIBE_CLI_QUEUE_SIZE", max(8, runtime.NumCPU()*2)),
		StateDir:      stateDir,
		BinDir:        binDir,
		UpdatesDir:    updatesDir,
		JobsFile:      filepath.Join(stateDir, "jobs.json"),
		SettingsFile:  settingsFile,
		ModelsDir:     getEnv("TRANSCRIBE_CLI_MODELS_DIR", modelsDir),
		UploadsDir:    filepath.Join(stateDir, "uploads"),
		OutputsDir:    filepath.Join(stateDir, "outputs"),
		DefaultModel:  CanonicalModelName(defaultModel),
		FFmpegBinary:  ffmpegBinary,
		WhisperBinary: whisperBinary,
		ReleaseRepo:   releaseRepo,
		AppVersion:    strings.TrimSpace(getEnv("TRANSCRIBE_CLI_VERSION", "")),
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
	if err := os.MkdirAll(cfg.BinDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.UpdatesDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.UploadsDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.OutputsDir, 0o755); err != nil {
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

func localToolPath(stateDir, tool string) string {
	binDir := filepath.Join(stateDir, "bin")
	if runtime.GOOS == "windows" {
		return filepath.Join(binDir, tool+".exe")
	}
	return filepath.Join(binDir, tool)
}

func fileExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
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
