package app

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	componentReady      = "ready"
	componentMissing    = "missing"
	componentInstalling = "installing"
	componentFailed     = "failed"
)

// ComponentStatus describes bootstrap state for one runtime dependency.
type ComponentStatus struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	Path    string `json:"path,omitempty"`
}

// BootstrapStatus is used by GUI onboarding to render environment readiness.
type BootstrapStatus struct {
	Ready      bool              `json:"ready"`
	InProgress bool              `json:"inProgress"`
	Error      string            `json:"error,omitempty"`
	Components []ComponentStatus `json:"components"`
}

// BootstrapManager auto-installs runtime dependencies for zero-setup UX.
type BootstrapManager struct {
	cfg    Config
	mu     sync.RWMutex
	status BootstrapStatus
}

func NewBootstrapManager(cfg Config) *BootstrapManager {
	m := &BootstrapManager{cfg: cfg}
	m.status = m.detectStatusLocked()
	return m
}

func (m *BootstrapManager) Status() BootstrapStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.status.InProgress {
		m.status = m.detectStatusLocked()
	}
	return cloneBootstrapStatus(m.status)
}

func (m *BootstrapManager) EnsureAsync(onReady func(ffmpegPath, whisperPath string)) bool {
	m.mu.Lock()
	if m.status.InProgress {
		m.mu.Unlock()
		return false
	}
	m.status = m.detectStatusLocked()
	if m.status.Ready {
		ffmpegPath := m.findComponentPath("ffmpeg")
		whisperPath := m.findComponentPath("whisper")
		m.mu.Unlock()
		if onReady != nil {
			onReady(ffmpegPath, whisperPath)
		}
		return false
	}
	m.status.InProgress = true
	m.status.Error = ""
	m.mu.Unlock()

	go func() {
		ffmpegPath, whisperPath, err := m.ensure(context.Background())
		m.mu.Lock()
		defer m.mu.Unlock()
		if err != nil {
			m.status.InProgress = false
			m.status.Ready = false
			m.status.Error = err.Error()
			return
		}
		m.status = m.detectStatusLocked()
		m.status.InProgress = false
		m.status.Error = ""
		if onReady != nil {
			go onReady(ffmpegPath, whisperPath)
		}
	}()
	return true
}

func (m *BootstrapManager) ensure(ctx context.Context) (string, string, error) {
	if err := EnsureStateDirs(m.cfg); err != nil {
		return "", "", err
	}

	ffmpegPath, err := m.ensureFFmpeg(ctx)
	if err != nil {
		m.setComponent("ffmpeg", componentFailed, err.Error(), "")
		return "", "", err
	}
	m.setComponent("ffmpeg", componentReady, "ready", ffmpegPath)

	whisperPath, err := m.ensureWhisper(ctx)
	if err != nil {
		m.setComponent("whisper", componentFailed, err.Error(), "")
		return "", "", err
	}
	m.setComponent("whisper", componentReady, "ready", whisperPath)

	m.setComponent("model", componentInstalling, "checking default model", "")
	if _, err := ResolveModelPath(m.cfg, ""); err != nil {
		if _, installErr := InstallModel(m.cfg, m.cfg.DefaultModel, "", nil); installErr != nil {
			m.setComponent("model", componentFailed, installErr.Error(), "")
			return "", "", installErr
		}
	}
	modelPath, _ := ResolveModelPath(m.cfg, "")
	m.setComponent("model", componentReady, "ready", modelPath)
	return ffmpegPath, whisperPath, nil
}

func (m *BootstrapManager) ensureFFmpeg(ctx context.Context) (string, error) {
	if path, ok := resolveBinaryPath(m.cfg.FFmpegBinary); ok {
		return path, nil
	}
	local := localToolPath(m.cfg.StateDir, "ffmpeg")
	if fileExists(local) {
		_ = m.persistBinaryPaths(local, "")
		m.cfg.FFmpegBinary = local
		return local, nil
	}

	m.setComponent("ffmpeg", componentInstalling, "downloading ffmpeg", "")
	var installed string
	var err error
	switch runtime.GOOS {
	case "windows":
		installed, err = installFFmpegWindows(ctx, m.cfg)
	case "darwin":
		installed, err = installFFmpegDarwin(ctx, m.cfg)
	default:
		err = fmt.Errorf("automatic ffmpeg install is not supported on %s", runtime.GOOS)
	}
	if err != nil {
		return "", err
	}
	m.cfg.FFmpegBinary = installed
	if saveErr := m.persistBinaryPaths(installed, ""); saveErr != nil {
		return "", saveErr
	}
	return installed, nil
}

func (m *BootstrapManager) ensureWhisper(ctx context.Context) (string, error) {
	if path, ok := resolveBinaryPath(m.cfg.WhisperBinary); ok {
		return path, nil
	}
	local := localToolPath(m.cfg.StateDir, "whisper-cli")
	if fileExists(local) {
		_ = m.persistBinaryPaths("", local)
		m.cfg.WhisperBinary = local
		return local, nil
	}

	m.setComponent("whisper", componentInstalling, "installing whisper-cli", "")
	var installed string
	var err error
	switch runtime.GOOS {
	case "windows":
		installed, err = installWhisperWindows(ctx, m.cfg)
	case "darwin":
		installed, err = installWhisperDarwin(ctx, m.cfg)
	default:
		err = fmt.Errorf("automatic whisper-cli install is not supported on %s", runtime.GOOS)
	}
	if err != nil {
		return "", err
	}
	m.cfg.WhisperBinary = installed
	if saveErr := m.persistBinaryPaths("", installed); saveErr != nil {
		return "", saveErr
	}
	return installed, nil
}

func (m *BootstrapManager) persistBinaryPaths(ffmpegPath, whisperPath string) error {
	settings, err := LoadSettings(m.cfg.SettingsFile)
	if err != nil {
		return err
	}
	if strings.TrimSpace(ffmpegPath) != "" {
		settings.FFmpegBinary = ffmpegPath
	}
	if strings.TrimSpace(whisperPath) != "" {
		settings.WhisperBinary = whisperPath
	}
	return SaveSettings(m.cfg.SettingsFile, settings)
}

func (m *BootstrapManager) detectStatusLocked() BootstrapStatus {
	status := BootstrapStatus{Components: make([]ComponentStatus, 0, 3)}

	ffmpegPath, ffmpegOk := resolveBinaryPath(m.cfg.FFmpegBinary)
	if ffmpegOk {
		status.Components = append(status.Components, ComponentStatus{Name: "ffmpeg", Status: componentReady, Path: ffmpegPath, Message: "ready"})
	} else {
		status.Components = append(status.Components, ComponentStatus{Name: "ffmpeg", Status: componentMissing, Message: "missing"})
	}

	whisperPath, whisperOk := resolveBinaryPath(m.cfg.WhisperBinary)
	if whisperOk {
		status.Components = append(status.Components, ComponentStatus{Name: "whisper", Status: componentReady, Path: whisperPath, Message: "ready"})
	} else {
		status.Components = append(status.Components, ComponentStatus{Name: "whisper", Status: componentMissing, Message: "missing"})
	}

	modelPath, modelErr := ResolveModelPath(m.cfg, "")
	if modelErr == nil {
		status.Components = append(status.Components, ComponentStatus{Name: "model", Status: componentReady, Path: modelPath, Message: "ready"})
	} else {
		status.Components = append(status.Components, ComponentStatus{Name: "model", Status: componentMissing, Message: modelErr.Error()})
	}

	status.Ready = ffmpegOk && whisperOk && modelErr == nil
	return status
}

func (m *BootstrapManager) findComponentPath(name string) string {
	for _, c := range m.status.Components {
		if c.Name == name {
			return c.Path
		}
	}
	return ""
}

func (m *BootstrapManager) setComponent(name, state, message, path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.status.Components) == 0 {
		m.status = m.detectStatusLocked()
	}
	updated := false
	for i := range m.status.Components {
		if m.status.Components[i].Name != name {
			continue
		}
		m.status.Components[i].Status = state
		m.status.Components[i].Message = message
		m.status.Components[i].Path = path
		updated = true
		break
	}
	if !updated {
		m.status.Components = append(m.status.Components, ComponentStatus{Name: name, Status: state, Message: message, Path: path})
	}
	m.status.Ready = false
}

func cloneBootstrapStatus(status BootstrapStatus) BootstrapStatus {
	status.Components = append([]ComponentStatus(nil), status.Components...)
	return status
}

func resolveBinaryPath(binary string) (string, bool) {
	binary = strings.TrimSpace(binary)
	if binary == "" {
		return "", false
	}
	if filepath.IsAbs(binary) || strings.ContainsRune(binary, os.PathSeparator) {
		if fileExists(binary) {
			return binary, true
		}
		return "", false
	}
	path, err := exec.LookPath(binary)
	if err != nil {
		return "", false
	}
	return path, true
}

func installFFmpegWindows(ctx context.Context, cfg Config) (string, error) {
	release, err := fetchLatestRelease(ctx, "BtbN/FFmpeg-Builds")
	if err != nil {
		return "", err
	}

	archToken := "win64"
	if runtime.GOARCH == "arm64" {
		archToken = "winarm64"
	}
	asset, ok := pickAsset(release.Assets, []string{archToken, "gpl", ".zip"}, []string{"shared"})
	if !ok {
		return "", fmt.Errorf("no ffmpeg asset found for %s", archToken)
	}

	tmpDir, err := os.MkdirTemp("", "transcribe-ffmpeg-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, "ffmpeg.zip")
	if err := downloadFile(ctx, asset.BrowserDownloadURL, archivePath); err != nil {
		return "", err
	}

	extractDir := filepath.Join(tmpDir, "extract")
	if err := unzipFile(archivePath, extractDir); err != nil {
		return "", err
	}

	ffmpegPath, err := findFileByBaseName(extractDir, "ffmpeg.exe")
	if err != nil {
		return "", err
	}
	dstFFmpeg := localToolPath(cfg.StateDir, "ffmpeg")
	if err := copyFile(ffmpegPath, dstFFmpeg); err != nil {
		return "", err
	}

	if ffprobePath, probeErr := findFileByBaseName(extractDir, "ffprobe.exe"); probeErr == nil {
		dstProbe := filepath.Join(cfg.BinDir, "ffprobe.exe")
		_ = copyFile(ffprobePath, dstProbe)
	}
	return dstFFmpeg, nil
}

func installWhisperWindows(ctx context.Context, cfg Config) (string, error) {
	release, err := fetchLatestRelease(ctx, "ggml-org/whisper.cpp")
	if err != nil {
		return "", err
	}

	assetName := "whisper-bin-x64.zip"
	if runtime.GOARCH == "386" {
		assetName = "whisper-bin-Win32.zip"
	}
	asset, ok := pickAsset(release.Assets, []string{assetName}, nil)
	if !ok {
		asset, ok = pickAsset(release.Assets, []string{"whisper-bin", "x64", ".zip"}, []string{"blas", "cublas"})
		if !ok {
			return "", fmt.Errorf("no whisper-cli asset found for windows")
		}
	}

	tmpDir, err := os.MkdirTemp("", "transcribe-whisper-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, "whisper.zip")
	if err := downloadFile(ctx, asset.BrowserDownloadURL, archivePath); err != nil {
		return "", err
	}

	distDir := filepath.Join(cfg.BinDir, "whisper")
	_ = os.RemoveAll(distDir)
	if err := os.MkdirAll(distDir, 0o755); err != nil {
		return "", err
	}
	if err := unzipFile(archivePath, distDir); err != nil {
		return "", err
	}

	cliPath, err := findFileByBaseName(distDir, "whisper-cli.exe")
	if err != nil {
		return "", err
	}
	return cliPath, nil
}

func installFFmpegDarwin(ctx context.Context, cfg Config) (string, error) {
	// Evermeet provides a stable direct endpoint for macOS ffmpeg zip bundle.
	const ffmpegURL = "https://evermeet.cx/ffmpeg/getrelease/zip"
	tmpDir, err := os.MkdirTemp("", "transcribe-ffmpeg-mac-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, "ffmpeg.zip")
	if err := downloadFile(ctx, ffmpegURL, archivePath); err != nil {
		return "", err
	}
	extractDir := filepath.Join(tmpDir, "extract")
	if err := unzipFile(archivePath, extractDir); err != nil {
		return "", err
	}

	found, err := findFileByBaseName(extractDir, "ffmpeg")
	if err != nil {
		return "", err
	}
	dst := localToolPath(cfg.StateDir, "ffmpeg")
	if err := copyFile(found, dst); err != nil {
		return "", err
	}
	if err := os.Chmod(dst, 0o755); err != nil {
		return "", err
	}
	return dst, nil
}

func installWhisperDarwin(ctx context.Context, cfg Config) (string, error) {
	if path, ok := resolveBinaryPath("whisper-cli"); ok {
		return path, nil
	}
	if _, err := exec.LookPath("cmake"); err != nil {
		return "", errors.New("whisper-cli is missing and cmake is not available for auto-build")
	}

	release, err := fetchLatestRelease(ctx, "ggml-org/whisper.cpp")
	if err != nil {
		return "", err
	}
	tag := strings.TrimSpace(release.TagName)
	if tag == "" {
		return "", errors.New("failed to resolve whisper.cpp release tag")
	}

	sourceURL := fmt.Sprintf("https://github.com/ggml-org/whisper.cpp/archive/refs/tags/%s.tar.gz", tag)
	tmpDir, err := os.MkdirTemp("", "transcribe-whisper-src-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	srcArchive := filepath.Join(tmpDir, "whisper-src.tar.gz")
	if err := downloadFile(ctx, sourceURL, srcArchive); err != nil {
		return "", err
	}

	srcDir := filepath.Join(tmpDir, "src")
	if err := untarGz(srcArchive, srcDir); err != nil {
		return "", err
	}

	projectRoot, err := findFileParent(srcDir, "CMakeLists.txt")
	if err != nil {
		return "", fmt.Errorf("cannot find whisper.cpp sources: %w", err)
	}

	buildDir := filepath.Join(tmpDir, "build")
	if err := runCmd(ctx, projectRoot, "cmake", "-S", projectRoot, "-B", buildDir, "-DCMAKE_BUILD_TYPE=Release"); err != nil {
		return "", err
	}
	jobs := strconv.Itoa(max(1, runtime.NumCPU()/2))
	if err := runCmd(ctx, projectRoot, "cmake", "--build", buildDir, "--config", "Release", "-j", jobs); err != nil {
		return "", err
	}

	cliPath, err := findFileByBaseName(buildDir, "whisper-cli")
	if err != nil {
		return "", err
	}
	dst := localToolPath(cfg.StateDir, "whisper-cli")
	if err := copyFile(cliPath, dst); err != nil {
		return "", err
	}
	if err := os.Chmod(dst, 0o755); err != nil {
		return "", err
	}
	return dst, nil
}

func runCmd(ctx context.Context, workdir, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = workdir
	output, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		if trimmed == "" {
			return fmt.Errorf("%s failed: %w", name, err)
		}
		return fmt.Errorf("%s failed: %w: %s", name, err, trimmed)
	}
	return nil
}

type releaseInfo struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func fetchLatestRelease(ctx context.Context, repo string) (releaseInfo, error) {
	url := "https://api.github.com/repos/" + strings.TrimSpace(repo) + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return releaseInfo{}, err
	}
	req.Header.Set("User-Agent", "transcribe-cli")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return releaseInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return releaseInfo{}, fmt.Errorf("release API returned HTTP %d", resp.StatusCode)
	}

	var release releaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return releaseInfo{}, err
	}
	return release, nil
}

func pickAsset(assets []releaseAsset, includes, excludes []string) (releaseAsset, bool) {
	for _, asset := range assets {
		nameLower := strings.ToLower(asset.Name)
		ok := true
		for _, token := range includes {
			if !strings.Contains(nameLower, strings.ToLower(token)) {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		for _, token := range excludes {
			if strings.Contains(nameLower, strings.ToLower(token)) {
				ok = false
				break
			}
		}
		if ok {
			return asset, true
		}
	}
	return releaseAsset{}, false
}

func downloadFile(ctx context.Context, url, dstPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "transcribe-cli")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return err
	}
	tmpPath := dstPath + ".part"
	file, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(file, resp.Body); err != nil {
		_ = file.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, dstPath)
}

func unzipFile(zipPath, dstDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}

	for _, f := range r.File {
		cleanName := filepath.Clean(f.Name)
		targetPath := filepath.Join(dstDir, cleanName)
		if !strings.HasPrefix(targetPath, filepath.Clean(dstDir)+string(os.PathSeparator)) && filepath.Clean(targetPath) != filepath.Clean(dstDir) {
			return fmt.Errorf("invalid zip path: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(targetPath)
		if err != nil {
			_ = rc.Close()
			return err
		}
		if _, err := io.Copy(out, rc); err != nil {
			_ = out.Close()
			_ = rc.Close()
			return err
		}
		_ = rc.Close()
		if err := out.Close(); err != nil {
			return err
		}
	}
	return nil
}

func untarGz(tarGzPath, dstDir string) error {
	file, err := os.Open(tarGzPath)
	if err != nil {
		return err
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}

		cleanName := filepath.Clean(hdr.Name)
		targetPath := filepath.Join(dstDir, cleanName)
		if !strings.HasPrefix(targetPath, filepath.Clean(dstDir)+string(os.PathSeparator)) && filepath.Clean(targetPath) != filepath.Clean(dstDir) {
			return fmt.Errorf("invalid tar path: %s", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return err
			}
			out, err := os.Create(targetPath)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				_ = out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		}
	}
	return nil
}

func findFileByBaseName(root, baseName string) (string, error) {
	baseName = strings.ToLower(strings.TrimSpace(baseName))
	if baseName == "" {
		return "", errors.New("file name is required")
	}
	var found string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Base(path), baseName) {
			found = path
			return io.EOF
		}
		return nil
	})
	if errors.Is(err, io.EOF) && strings.TrimSpace(found) != "" {
		return found, nil
	}
	if err != nil {
		return "", err
	}
	return "", fmt.Errorf("file %s not found", baseName)
}

func findFileParent(root, baseName string) (string, error) {
	filePath, err := findFileByBaseName(root, baseName)
	if err != nil {
		return "", err
	}
	return filepath.Dir(filePath), nil
}

// ShouldApplyStagedUpdate replaces the current executable with a staged update when available.
// Return true when current process should exit immediately (windows handoff in progress).
func ShouldApplyStagedUpdate() (bool, error) {
	exePath, err := os.Executable()
	if err != nil {
		return false, err
	}
	staged := exePath + ".new"
	if !fileExists(staged) {
		return false, nil
	}

	if runtime.GOOS == "windows" {
		scriptPath := filepath.Join(os.TempDir(), fmt.Sprintf("transcribe-update-%d.cmd", time.Now().UnixNano()))
		script := strings.Join([]string{
			"@echo off",
			"setlocal",
			"set EXE=" + quoteForCmd(exePath),
			"set NEW=" + quoteForCmd(staged),
			":retry",
			"move /Y %NEW% %EXE% >nul 2>nul",
			"if errorlevel 1 (",
			"  timeout /t 1 /nobreak >nul",
			"  goto retry",
			")",
			"start \"\" %EXE%",
			"del /f /q \"%~f0\" >nul 2>nul",
		}, "\r\n") + "\r\n"
		if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
			return false, err
		}
		cmd := exec.Command("cmd", "/C", scriptPath)
		if err := cmd.Start(); err != nil {
			return false, err
		}
		return true, nil
	}

	tmpPath := exePath + ".tmp-update"
	if err := copyFile(staged, tmpPath); err != nil {
		return false, err
	}
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return false, err
	}
	if err := os.Rename(tmpPath, exePath); err != nil {
		return false, err
	}
	_ = os.Remove(staged)
	return false, nil
}

func quoteForCmd(path string) string {
	path = strings.ReplaceAll(path, "\"", "")
	return "\"" + path + "\""
}
