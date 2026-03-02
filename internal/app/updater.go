package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// UpdateStatus describes auto-update check progress.
type UpdateStatus struct {
	Enabled         bool   `json:"enabled"`
	InProgress      bool   `json:"inProgress"`
	CurrentVersion  string `json:"currentVersion"`
	LatestVersion   string `json:"latestVersion,omitempty"`
	UpdateAvailable bool   `json:"updateAvailable"`
	Downloaded      bool   `json:"downloaded"`
	Message         string `json:"message,omitempty"`
	Error           string `json:"error,omitempty"`
	CheckedAt       string `json:"checkedAt,omitempty"`
}

// Updater checks GitHub Releases and stages updates for next launch.
type Updater struct {
	cfg    Config
	mu     sync.RWMutex
	status UpdateStatus
}

func NewUpdater(cfg Config) *Updater {
	u := &Updater{cfg: cfg}
	u.status = UpdateStatus{
		Enabled:        strings.TrimSpace(cfg.ReleaseRepo) != "" && normalizeVersion(cfg.AppVersion) != "",
		CurrentVersion: normalizeVersion(cfg.AppVersion),
	}
	if u.status.CurrentVersion == "" {
		u.status.CurrentVersion = "dev"
	}
	return u
}

func (u *Updater) Status() UpdateStatus {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.status
}

func (u *Updater) CheckAsync() bool {
	u.mu.Lock()
	if !u.status.Enabled || u.status.InProgress {
		u.mu.Unlock()
		return false
	}
	u.status.InProgress = true
	u.status.Error = ""
	u.status.Message = "checking updates"
	u.mu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		latest, staged, err := u.checkAndStage(ctx)
		u.mu.Lock()
		defer u.mu.Unlock()
		u.status.InProgress = false
		u.status.CheckedAt = time.Now().UTC().Format(time.RFC3339)
		if err != nil {
			u.status.Error = err.Error()
			u.status.Message = "update check failed"
			return
		}
		u.status.LatestVersion = latest
		u.status.UpdateAvailable = compareVersions(normalizeVersion(latest), normalizeVersion(u.cfg.AppVersion)) > 0
		u.status.Downloaded = staged
		if u.status.UpdateAvailable && staged {
			u.status.Message = "update downloaded and will be applied on next launch"
		} else if u.status.UpdateAvailable {
			u.status.Message = "new update available"
		} else {
			u.status.Message = "up to date"
		}
	}()

	return true
}

func (u *Updater) checkAndStage(ctx context.Context) (latest string, staged bool, err error) {
	repo := strings.TrimSpace(u.cfg.ReleaseRepo)
	if repo == "" {
		return "", false, nil
	}
	release, err := fetchLatestRelease(ctx, repo)
	if err != nil {
		return "", false, err
	}
	latest = strings.TrimSpace(release.TagName)
	if compareVersions(normalizeVersion(latest), normalizeVersion(u.cfg.AppVersion)) <= 0 {
		return latest, false, nil
	}

	exePath, err := os.Executable()
	if err != nil {
		return latest, false, err
	}
	stagedPath := exePath + ".new"
	if fileExists(stagedPath) {
		return latest, true, nil
	}

	asset, ok := pickUpdateAsset(release.Assets)
	if !ok {
		return latest, false, fmt.Errorf("no matching update asset for %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	tmpDir, err := os.MkdirTemp("", "transcribe-update-*")
	if err != nil {
		return latest, false, err
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, "update.zip")
	if err := downloadFile(ctx, asset.BrowserDownloadURL, archivePath); err != nil {
		return latest, false, err
	}

	extractDir := filepath.Join(tmpDir, "extract")
	if err := unzipFile(archivePath, extractDir); err != nil {
		return latest, false, err
	}

	binaryName := "transcribe"
	if runtime.GOOS == "windows" {
		binaryName = "transcribe.exe"
	}
	newBinary, err := findFileByBaseName(extractDir, binaryName)
	if err != nil {
		return latest, false, err
	}
	if err := copyFile(newBinary, stagedPath); err != nil {
		return latest, false, err
	}
	if runtime.GOOS != "windows" {
		_ = os.Chmod(stagedPath, 0o755)
	}
	return latest, true, nil
}

func pickUpdateAsset(assets []releaseAsset) (releaseAsset, bool) {
	osToken := strings.ToLower(runtime.GOOS)
	archToken := strings.ToLower(runtime.GOARCH)
	nameIncludes := []string{"transcribe-cli_", "_" + osToken + "_", "_" + archToken + ".zip"}
	asset, ok := pickAsset(assets, nameIncludes, nil)
	if ok {
		return asset, true
	}
	return pickAsset(assets, []string{osToken, archToken, ".zip"}, nil)
}

func normalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if strings.EqualFold(v, "dev") {
		return ""
	}
	v = strings.TrimPrefix(v, "v")
	if v == "" {
		return ""
	}
	head := v[0]
	if head < '0' || head > '9' {
		return ""
	}
	return v
}

func compareVersions(a, b string) int {
	if a == b {
		return 0
	}
	parse := func(v string) []int {
		parts := strings.Split(v, ".")
		out := make([]int, 0, len(parts))
		for _, part := range parts {
			n := strings.TrimSpace(part)
			n = strings.TrimLeft(n, "v")
			n = strings.SplitN(n, "-", 2)[0]
			value, err := strconv.Atoi(n)
			if err != nil {
				value = 0
			}
			out = append(out, value)
		}
		return out
	}
	av := parse(a)
	bv := parse(b)
	maxLen := len(av)
	if len(bv) > maxLen {
		maxLen = len(bv)
	}
	for i := 0; i < maxLen; i++ {
		ai := 0
		bi := 0
		if i < len(av) {
			ai = av[i]
		}
		if i < len(bv) {
			bi = bv[i]
		}
		if ai > bi {
			return 1
		}
		if ai < bi {
			return -1
		}
	}
	return 0
}
