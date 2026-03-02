//go:build desktop

package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"transcribe-cli/internal/app"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

var version = "dev"

//go:embed frontend/dist
var assets embed.FS

type desktopApp struct {
	uiURL      string
	stopDaemon func()
}

func (d *desktopApp) startup(ctx context.Context) {
	wailsruntime.WindowSetTitle(ctx, "Transcribe")
	js := fmt.Sprintf(`window.__BACKEND_URL = %q; if (window.setBackendURL) window.setBackendURL(%q);`, d.uiURL, d.uiURL)
	_ = wailsruntime.WindowExecJS(ctx, js)
}

func (d *desktopApp) shutdown(_ context.Context) {
	if d.stopDaemon != nil {
		d.stopDaemon()
	}
}

func main() {
	shouldExit, err := app.ShouldApplyStagedUpdate()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Warning: failed to apply staged update:", err)
	}
	if shouldExit {
		return
	}

	cfg := app.LoadConfig()
	cfg.AppVersion = version

	stopFn, runtimeCfg, err := ensureDaemonRunningDesktop(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	desktop := &desktopApp{
		uiURL:      runtimeCfg.ClientBaseURL + "/",
		stopDaemon: stopFn,
	}

	err = wails.Run(&options.App{
		Title:         "Transcribe",
		Width:         1320,
		Height:        880,
		MinWidth:      960,
		MinHeight:     640,
		DisableResize: false,
		Frameless:     false,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  desktop.startup,
		OnShutdown: desktop.shutdown,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func ensureDaemonRunningDesktop(cfg app.Config) (stop func(), runtimeCfg app.Config, err error) {
	client := app.NewClient(cfg.ClientBaseURL)
	if err := client.Health(); err == nil {
		return func() {}, cfg, nil
	}

	candidate := cfg
	const maxAddressAttempts = 10
	for i := 0; i < maxAddressAttempts; i++ {
		stopFn, startErr := startDaemonProcessDesktop(candidate)
		if startErr == nil {
			return stopFn, candidate, nil
		}
		if !isAddressInUseError(startErr) {
			return nil, candidate, startErr
		}
		nextAddr, addrErr := bumpPort(candidate.Addr, 1)
		if addrErr != nil {
			return nil, candidate, fmt.Errorf("failed to pick fallback address after %s: %w", candidate.Addr, startErr)
		}
		candidate.Addr = nextAddr
		candidate.ClientBaseURL = "http://" + nextAddr
	}
	return nil, candidate, errors.New("failed to start daemon on fallback ports")
}

func startDaemonProcessDesktop(cfg app.Config) (func(), error) {
	daemon, err := app.NewDaemon(cfg)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- daemon.Run(ctx)
	}()

	client := app.NewClient(cfg.ClientBaseURL)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case runErr := <-errCh:
			cancel()
			if runErr == nil {
				return nil, errors.New("daemon exited unexpectedly")
			}
			return nil, fmt.Errorf("failed to start daemon at %s: %w", cfg.Addr, runErr)
		default:
		}

		if err := client.Health(); err == nil {
			return func() {
				cancel()
				select {
				case <-time.After(500 * time.Millisecond):
				case <-errCh:
				}
			}, nil
		}
		time.Sleep(150 * time.Millisecond)
	}

	cancel()
	select {
	case runErr := <-errCh:
		if runErr != nil {
			return nil, fmt.Errorf("failed to start daemon at %s: %w", cfg.Addr, runErr)
		}
	default:
	}
	return nil, fmt.Errorf("timed out waiting for daemon to start at %s", cfg.Addr)
}

func isAddressInUseError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "address already in use") ||
		strings.Contains(msg, "only one usage of each socket address")
}

func bumpPort(addr string, offset int) (string, error) {
	host, portStr, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return "", err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", err
	}
	port += offset
	if port <= 0 || port > 65535 {
		return "", fmt.Errorf("port out of range: %d", port)
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}
