package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"transcribe-cli/internal/app"
)

var version = "dev"

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

	if len(os.Args) < 2 {
		if err := runGUI(cfg, []string{}); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
		return
	}

	err = nil
	switch os.Args[1] {
	case "init":
		err = runInit(cfg, os.Args[2:])
	case "run":
		err = runRun(cfg, os.Args[2:])
	case "gui":
		err = runGUI(cfg, os.Args[2:])
	case "daemon":
		err = runDaemon(cfg, os.Args[2:])
	case "queue":
		err = runQueue(cfg, os.Args[2:])
	case "model":
		err = runModel(cfg, os.Args[2:])
	case "setup":
		err = runSetup(cfg)
	case "doctor":
		err = runDoctor(cfg)
	case "help", "-h", "--help":
		printUsage()
		return
	case "version":
		fmt.Println(version)
		return
	default:
		err = fmt.Errorf("unknown command: %s", os.Args[1])
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func runDaemon(cfg app.Config, args []string) error {
	if len(args) == 0 || args[0] != "run" {
		return errors.New("usage: transcribe daemon run")
	}

	fs := flag.NewFlagSet("daemon run", flag.ContinueOnError)
	addr := fs.String("addr", cfg.Addr, "daemon listen address")
	workers := fs.Int("workers", cfg.Workers, "number of workers")
	queueSize := fs.Int("queue-size", cfg.QueueSize, "queue size")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	cfg.Addr = strings.TrimSpace(*addr)
	cfg.Workers = *workers
	cfg.QueueSize = *queueSize
	cfg.ClientBaseURL = "http://" + cfg.Addr

	daemon, err := app.NewDaemon(cfg)
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	fmt.Printf("Transcribe daemon is running at http://%s\n", cfg.Addr)
	fmt.Printf("State: %s\n", cfg.StateDir)
	fmt.Printf("Default model: %s\n", cfg.DefaultModel)
	return daemon.Run(ctx)
}

func runInit(cfg app.Config, args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	model := fs.String("model", cfg.DefaultModel, "default model name (alias or ggml-*)")
	skipModel := fs.Bool("skip-model", false, "skip model installation/check")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if err := app.EnsureStateDirs(cfg); err != nil {
		return err
	}

	selectedModel := cfgSafeDefaultModel(*model)
	if !filepath.IsAbs(selectedModel) && !strings.ContainsRune(selectedModel, os.PathSeparator) {
		if err := saveDefaultModel(cfg, selectedModel); err != nil {
			return err
		}
	}

	fmt.Printf("State directory: %s\n", cfg.StateDir)
	fmt.Printf("Models directory: %s\n", cfg.ModelsDir)
	fmt.Printf("Default model: %s\n", selectedModel)

	if _, err := exec.LookPath(cfg.FFmpegBinary); err != nil {
		fmt.Printf("ffmpeg: missing (%s)\n", cfg.FFmpegBinary)
		fmt.Println("Install ffmpeg and rerun `transcribe init`.")
	} else {
		fmt.Printf("ffmpeg: ok (%s)\n", cfg.FFmpegBinary)
	}
	if _, err := exec.LookPath(cfg.WhisperBinary); err != nil {
		fmt.Printf("whisper: missing (%s)\n", cfg.WhisperBinary)
		fmt.Println("Install whisper-cli and rerun `transcribe init`.")
	} else {
		fmt.Printf("whisper: ok (%s)\n", cfg.WhisperBinary)
	}

	if *skipModel {
		fmt.Println("Model check skipped (--skip-model).")
		fmt.Println("Init complete.")
		return nil
	}

	modelCfg := cfg
	modelCfg.DefaultModel = selectedModel
	if path, err := app.ResolveModelPath(modelCfg, selectedModel); err == nil {
		fmt.Printf("Model: ok (%s)\n", path)
		fmt.Println("Init complete.")
		return nil
	}

	fmt.Printf("Model %s not found, downloading...\n", selectedModel)
	path, err := app.InstallModel(cfg, selectedModel, "", os.Stdout)
	if err != nil {
		return err
	}
	fmt.Printf("Model installed: %s\n", path)
	fmt.Println("Init complete.")
	return nil
}

func runRun(cfg app.Config, args []string) error {
	fileArg, language, model, outputDir, watch, watchInterval, err := parseRunArgs(args, cfg.DefaultModel)
	if err != nil {
		return err
	}

	filePath, err := filepath.Abs(fileArg)
	if err != nil {
		return err
	}

	stopDaemon, startedDaemon, runtimeCfg, err := ensureDaemonRunning(cfg)
	if err != nil {
		return err
	}
	if startedDaemon {
		fmt.Println("Daemon was not running, started temporary local daemon.")
		if runtimeCfg.Addr != cfg.Addr {
			fmt.Printf("Default port %s is busy, using %s.\n", cfg.Addr, runtimeCfg.Addr)
		}
	}
	defer stopDaemon()

	client := app.NewClient(runtimeCfg.ClientBaseURL)
	job, err := client.AddJob(app.AddJobRequest{
		FilePath:  filePath,
		OutputDir: outputDir,
		Language:  language,
		Model:     model,
	})
	if err != nil {
		return err
	}

	fmt.Printf("Queued: %s\n", job.ID)
	fmt.Printf("Status: %s (%d%%)\n", job.Status, job.Progress)
	fmt.Printf("Model: %s\n", job.Model)

	if !watch {
		return nil
	}
	return watchJob(client, job.ID, watchInterval)
}

func runGUI(cfg app.Config, args []string) error {
	fs := flag.NewFlagSet("gui", flag.ContinueOnError)
	openBrowser := fs.Bool("open", true, "open UI in default browser")
	if err := fs.Parse(args); err != nil {
		return err
	}

	stopDaemon, startedDaemon, runtimeCfg, err := ensureDaemonRunning(cfg)
	if err != nil {
		return err
	}

	uiURL := runtimeCfg.ClientBaseURL + "/"
	fmt.Printf("GUI URL: %s\n", uiURL)
	if *openBrowser {
		if err := openURL(uiURL); err != nil {
			fmt.Printf("Could not auto-open browser: %v\n", err)
			fmt.Println("Open the URL manually in your browser.")
		}
	}

	if !startedDaemon {
		fmt.Println("Daemon is already running. Press Ctrl+C to exit.")
	} else {
		fmt.Println("Started local daemon for GUI. Press Ctrl+C to stop.")
		if runtimeCfg.Addr != cfg.Addr {
			fmt.Printf("Default port %s is busy, daemon started on %s.\n", cfg.Addr, runtimeCfg.Addr)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	stopDaemon()
	return nil
}

func runQueue(cfg app.Config, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: transcribe queue <add|list|status|watch|cancel|retry>")
	}
	client := app.NewClient(cfg.ClientBaseURL)

	switch args[0] {
	case "add":
		fileArg, language, model, outputDir, err := parseQueueAddArgs(args[1:], cfg.DefaultModel)
		if err != nil {
			return err
		}
		filePath, err := filepath.Abs(fileArg)
		if err != nil {
			return err
		}

		job, err := client.AddJob(app.AddJobRequest{
			FilePath:  filePath,
			OutputDir: outputDir,
			Language:  language,
			Model:     model,
		})
		if err != nil {
			return err
		}
		fmt.Printf("Queued: %s\n", job.ID)
		fmt.Printf("Status: %s (%d%%)\n", job.Status, job.Progress)
		fmt.Printf("Model: %s\n", job.Model)
		return nil

	case "list":
		jobs, err := client.ListJobs()
		if err != nil {
			return err
		}
		if len(jobs) == 0 {
			fmt.Println("No jobs yet.")
			return nil
		}
		sort.Slice(jobs, func(i, j int) bool {
			return jobs[i].CreatedAt.After(jobs[j].CreatedAt)
		})
		fmt.Println("ID\tSTATUS\tPROGRESS\tMODEL\tFILE")
		for _, job := range jobs {
			fmt.Printf("%s\t%s\t%d%%\t%s\t%s\n", job.ID, job.Status, job.Progress, job.Model, job.FilePath)
		}
		return nil

	case "status":
		if len(args) < 2 {
			return errors.New("usage: transcribe queue status <job-id>")
		}
		job, err := client.GetJob(args[1])
		if err != nil {
			return err
		}
		return printJob(job)

	case "watch":
		fs := flag.NewFlagSet("queue watch", flag.ContinueOnError)
		interval := fs.Duration("interval", 2*time.Second, "poll interval")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		rest := fs.Args()
		if len(rest) != 1 {
			return errors.New("usage: transcribe queue watch <job-id> [--interval 2s]")
		}
		jobID := rest[0]
		return watchJob(client, jobID, *interval)

	case "cancel":
		if len(args) != 2 {
			return errors.New("usage: transcribe queue cancel <job-id>")
		}
		job, err := client.CancelJob(args[1])
		if err != nil {
			return err
		}
		fmt.Printf("Canceled: %s (%s)\n", job.ID, job.Status)
		return nil

	case "retry":
		if len(args) != 2 {
			return errors.New("usage: transcribe queue retry <job-id>")
		}
		job, err := client.RetryJob(args[1])
		if err != nil {
			return err
		}
		fmt.Printf("Re-queued: %s (%s)\n", job.ID, job.Status)
		return nil
	default:
		return fmt.Errorf("unknown queue command: %s", args[0])
	}
}

func parseQueueAddArgs(args []string, defaultModel string) (filePath string, language string, model string, outputDir string, err error) {
	language = "auto"
	model = strings.TrimSpace(defaultModel)
	if model == "" {
		model = "ggml-base"
	}

	readValue := func(current string, i *int) (string, error) {
		if strings.Contains(current, "=") {
			parts := strings.SplitN(current, "=", 2)
			return strings.TrimSpace(parts[1]), nil
		}
		if *i+1 >= len(args) {
			return "", fmt.Errorf("missing value for %s", current)
		}
		*i = *i + 1
		return strings.TrimSpace(args[*i]), nil
	}

	for i := 0; i < len(args); i++ {
		token := strings.TrimSpace(args[i])
		switch {
		case token == "--lang" || token == "-l" || strings.HasPrefix(token, "--lang="):
			value, parseErr := readValue(token, &i)
			if parseErr != nil {
				return "", "", "", "", parseErr
			}
			language = value
		case token == "--model" || token == "-m" || strings.HasPrefix(token, "--model="):
			value, parseErr := readValue(token, &i)
			if parseErr != nil {
				return "", "", "", "", parseErr
			}
			model = value
		case token == "--output-dir" || token == "-o" || strings.HasPrefix(token, "--output-dir="):
			value, parseErr := readValue(token, &i)
			if parseErr != nil {
				return "", "", "", "", parseErr
			}
			outputDir = value
		case strings.HasPrefix(token, "-"):
			return "", "", "", "", fmt.Errorf("unknown flag: %s", token)
		default:
			if filePath != "" {
				return "", "", "", "", errors.New("only one input file is allowed")
			}
			filePath = token
		}
	}

	if filePath == "" {
		return "", "", "", "", errors.New("usage: transcribe queue add [--lang ru] [--model ggml-base] [--output-dir ./out] <file>")
	}
	language = strings.TrimSpace(language)
	if language == "" {
		language = "auto"
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = cfgSafeDefaultModel(defaultModel)
	}
	if !filepath.IsAbs(model) && !strings.ContainsRune(model, os.PathSeparator) {
		model = app.CanonicalModelName(model)
	}
	return filePath, language, model, strings.TrimSpace(outputDir), nil
}

func parseRunArgs(args []string, defaultModel string) (filePath string, language string, model string, outputDir string, watch bool, watchInterval time.Duration, err error) {
	watch = true
	watchInterval = 2 * time.Second

	normalized := make([]string, 0, len(args))
	readValue := func(current string, i *int) (string, error) {
		if strings.Contains(current, "=") {
			parts := strings.SplitN(current, "=", 2)
			return strings.TrimSpace(parts[1]), nil
		}
		if *i+1 >= len(args) {
			return "", fmt.Errorf("missing value for %s", current)
		}
		*i = *i + 1
		return strings.TrimSpace(args[*i]), nil
	}

	for i := 0; i < len(args); i++ {
		token := strings.TrimSpace(args[i])
		switch {
		case token == "--no-watch":
			watch = false
		case token == "--watch":
			watch = true
		case token == "--interval" || strings.HasPrefix(token, "--interval="):
			value, parseErr := readValue(token, &i)
			if parseErr != nil {
				return "", "", "", "", false, 0, parseErr
			}
			parsed, parseErr := time.ParseDuration(value)
			if parseErr != nil {
				return "", "", "", "", false, 0, fmt.Errorf("invalid --interval: %w", parseErr)
			}
			if parsed <= 0 {
				return "", "", "", "", false, 0, errors.New("--interval must be > 0")
			}
			watchInterval = parsed
		default:
			normalized = append(normalized, token)
		}
	}

	filePath, language, model, outputDir, err = parseQueueAddArgs(normalized, defaultModel)
	return filePath, language, model, outputDir, watch, watchInterval, err
}

func ensureDaemonRunning(cfg app.Config) (stop func(), started bool, runtimeCfg app.Config, err error) {
	client := app.NewClient(cfg.ClientBaseURL)
	if err := client.Health(); err == nil {
		return func() {}, false, cfg, nil
	}

	candidate := cfg
	const maxAddressAttempts = 10
	for i := 0; i < maxAddressAttempts; i++ {
		stopFn, startErr := startDaemonProcess(candidate)
		if startErr == nil {
			return stopFn, true, candidate, nil
		}
		if !isAddressInUseError(startErr) {
			return nil, false, candidate, startErr
		}
		nextAddr, addrErr := bumpPort(candidate.Addr, 1)
		if addrErr != nil {
			return nil, false, candidate, fmt.Errorf("failed to pick fallback address after %s: %w", candidate.Addr, startErr)
		}
		candidate.Addr = nextAddr
		candidate.ClientBaseURL = "http://" + nextAddr
	}
	return nil, false, candidate, errors.New("failed to start daemon on fallback ports")
}

func startDaemonProcess(cfg app.Config) (func(), error) {
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

func saveDefaultModel(cfg app.Config, model string) error {
	settings, err := app.LoadSettings(cfg.SettingsFile)
	if err != nil {
		return err
	}
	settings.DefaultModel = app.CanonicalModelName(model)
	return app.SaveSettings(cfg.SettingsFile, settings)
}

func cfgSafeDefaultModel(defaultModel string) string {
	defaultModel = strings.TrimSpace(defaultModel)
	if defaultModel == "" {
		return "ggml-base"
	}
	return app.CanonicalModelName(defaultModel)
}

func runModel(cfg app.Config, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: transcribe model <list|presets|current|use|install|remove>")
	}

	switch args[0] {
	case "list":
		models, err := app.ListModels(cfg)
		if err != nil {
			return err
		}
		fmt.Printf("Default model: %s\n", cfg.DefaultModel)
		if path, err := app.ResolveModelPath(cfg, ""); err == nil {
			fmt.Printf("Default path: %s\n", path)
		} else {
			fmt.Printf("Default status: %v\n", err)
		}
		if len(models) == 0 {
			fmt.Println("No local models installed.")
			fmt.Println("Use: transcribe model presets")
			fmt.Println("Then: transcribe model install --name ggml-base")
			return nil
		}
		fmt.Println("NAME\tSIZE\tPATH")
		for _, m := range models {
			fmt.Printf("%s\t%s\t%s\n", m.Name, humanSize(m.SizeBytes), m.Path)
		}
		return nil

	case "presets":
		fmt.Println("Available preset models:")
		fmt.Println("ALIAS\tCANONICAL\tINSTALL COMMAND")
		for _, preset := range app.ListPresetModels() {
			fmt.Printf("%s\t%s\ttranscribe model install --name %s\n", preset.Alias, preset.Name, preset.Name)
		}
		fmt.Println("You can also use aliases (base/small/medium/large).")
		return nil

	case "current":
		fmt.Printf("Default model: %s\n", cfg.DefaultModel)
		path, err := app.ResolveModelPath(cfg, "")
		if err != nil {
			fmt.Printf("Status: missing (%v)\n", err)
			fmt.Printf("Fix: transcribe model install --name %s\n", app.CanonicalModelName(cfg.DefaultModel))
			return nil
		}
		fmt.Printf("Resolved path: %s\n", path)
		return nil

	case "use":
		if len(args) < 2 {
			return errors.New("usage: transcribe model use <name-or-absolute-path>")
		}
		selected := strings.TrimSpace(args[1])
		if selected == "" {
			return errors.New("model name is required")
		}
		if !filepath.IsAbs(selected) && !strings.ContainsRune(selected, os.PathSeparator) {
			selected = app.CanonicalModelName(selected)
		}

		settings, err := app.LoadSettings(cfg.SettingsFile)
		if err != nil {
			return err
		}
		settings.DefaultModel = selected
		if err := app.SaveSettings(cfg.SettingsFile, settings); err != nil {
			return err
		}

		fmt.Printf("Default model saved: %s\n", selected)
		tempCfg := cfg
		tempCfg.DefaultModel = selected
		if path, err := app.ResolveModelPath(tempCfg, ""); err != nil {
			fmt.Printf("Status: missing (%v)\n", err)
			fmt.Println("Install it with: transcribe model install --name <model>")
		} else {
			fmt.Printf("Resolved path: %s\n", path)
		}
		return nil

	case "install":
		fs := flag.NewFlagSet("model install", flag.ContinueOnError)
		name := fs.String("name", cfg.DefaultModel, "model name (preset alias or canonical ggml-*)")
		url := fs.String("url", "", "optional direct URL for model .bin")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		path, err := app.InstallModel(cfg, *name, *url, os.Stdout)
		if err != nil {
			return err
		}
		fmt.Printf("Installed model: %s\n", path)
		return nil

	case "remove":
		if len(args) < 2 {
			return errors.New("usage: transcribe model remove <name>")
		}
		if err := app.RemoveModel(cfg, args[1]); err != nil {
			return err
		}
		fmt.Printf("Removed model: %s\n", app.CanonicalModelName(args[1]))
		return nil

	default:
		return fmt.Errorf("unknown model command: %s", args[0])
	}
}

func runSetup(cfg app.Config) error {
	if err := app.EnsureStateDirs(cfg); err != nil {
		return err
	}
	fmt.Printf("State directory: %s\n", cfg.StateDir)
	fmt.Printf("Settings file: %s\n", cfg.SettingsFile)
	fmt.Printf("Models directory: %s\n", cfg.ModelsDir)
	fmt.Printf("Uploads directory: %s\n", cfg.UploadsDir)
	fmt.Printf("Outputs directory: %s\n", cfg.OutputsDir)
	fmt.Printf("Daemon address: %s\n", cfg.Addr)
	fmt.Printf("Default model: %s\n", cfg.DefaultModel)
	fmt.Println("Setup complete.")
	return nil
}

func runDoctor(cfg app.Config) error {
	if err := app.EnsureStateDirs(cfg); err != nil {
		return err
	}
	fmt.Printf("OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Printf("State: %s\n", cfg.StateDir)
	fmt.Printf("Settings: %s\n", cfg.SettingsFile)
	fmt.Printf("Models: %s\n", cfg.ModelsDir)
	fmt.Printf("Uploads: %s\n", cfg.UploadsDir)
	fmt.Printf("Outputs: %s\n", cfg.OutputsDir)
	fmt.Printf("Default model: %s\n", cfg.DefaultModel)
	if path, err := app.ResolveModelPath(cfg, ""); err != nil {
		fmt.Printf("default model: missing (%v)\n", err)
	} else {
		fmt.Printf("default model path: %s\n", path)
	}

	checkBinary(cfg.FFmpegBinary, "ffmpeg")
	checkBinary(cfg.WhisperBinary, "whisper")

	client := app.NewClient(cfg.ClientBaseURL)
	if err := client.Health(); err != nil {
		fmt.Printf("daemon: not reachable (%v)\n", err)
		fmt.Println("hint: start it with `transcribe daemon run` or use `transcribe run <file>`")
		return nil
	}
	fmt.Println("daemon: healthy")
	return nil
}

func checkBinary(binary, name string) {
	path, err := exec.LookPath(binary)
	if err != nil {
		fmt.Printf("%s: missing (%s)\n", name, binary)
		return
	}
	fmt.Printf("%s: ok (%s)\n", name, path)
}

func openURL(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// Prefer app-window mode so UI behaves like a local desktop app, not a browser tab.
		appLaunchers := []struct {
			bin  string
			args []string
		}{
			{bin: "msedge", args: []string{"--app=" + url, "--new-window"}},
			{bin: "chrome", args: []string{"--app=" + url, "--new-window"}},
			{bin: "brave", args: []string{"--app=" + url, "--new-window"}},
		}
		for _, launcher := range appLaunchers {
			if _, err := exec.LookPath(launcher.bin); err != nil {
				continue
			}
			if err := exec.Command(launcher.bin, launcher.args...).Start(); err == nil {
				return nil
			}
		}
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

func watchJob(client *app.Client, jobID string, interval time.Duration) error {
	var lastLine string
	for {
		job, err := client.GetJob(jobID)
		if err != nil {
			return err
		}
		line := fmt.Sprintf("%s %d%% %s", job.Status, job.Progress, job.Message)
		if line != lastLine {
			fmt.Println(line)
			lastLine = line
		}

		switch job.Status {
		case app.StatusCompleted:
			fmt.Printf("txt: %s\n", job.ResultText)
			if job.ResultSRT != "" {
				fmt.Printf("srt: %s\n", job.ResultSRT)
			}
			if job.ResultVTT != "" {
				fmt.Printf("vtt: %s\n", job.ResultVTT)
			}
			return nil
		case app.StatusFailed:
			if job.Error != "" {
				return errors.New(job.Error)
			}
			return errors.New("job failed")
		case app.StatusCanceled:
			return errors.New("job canceled")
		}
		time.Sleep(interval)
	}
}

func printJob(job *app.Job) error {
	payload, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(payload))
	return nil
}

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for value := n / unit; value >= unit; value /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func printUsage() {
	fmt.Print(`transcribe - offline transcription CLI

Default launch:
  transcribe                         Start GUI with automatic onboarding

Commands:
  init                            Prepare runtime checks and default model
  run [flags] <file>              One-shot transcription (auto-start daemon)
  gui [--open]                    Launch local web UI for queue and models
  setup                           Initialize local state directories
  doctor                          Check local dependencies and daemon health
  version                         Print app version
  daemon run                      Start local queue daemon

  model list                      List installed local models
  model presets                   Show known downloadable model presets
  model current                   Show current default model and status
  model use <name|path>           Set default model in config
  model install --name base       Download model preset
  model remove <name>             Remove model

  queue add [flags] <file>        Add audio/video file to queue
  queue list                      Show queue jobs
  queue status <job-id>           Show detailed job JSON
  queue watch <job-id>            Poll job progress until done
  queue cancel <job-id>           Cancel queued/running job
  queue retry <job-id>            Retry failed/canceled job

Environment variables:
  TRANSCRIBE_CLI_ADDR             Daemon listen address (default 127.0.0.1:9864)
  TRANSCRIBE_CLI_STATE_DIR        State directory (default OS user config dir)
  TRANSCRIBE_CLI_MODELS_DIR       Models directory
  TRANSCRIBE_CLI_DEFAULT_MODEL    Default model (overrides saved config)
  TRANSCRIBE_CLI_RELEASE_REPO     GitHub repo for auto-update checks
  TRANSCRIBE_CLI_VERSION          Build version (normally injected by release)
  TRANSCRIBE_CLI_FFMPEG           ffmpeg binary name/path
  TRANSCRIBE_CLI_WHISPER          whisper-cli binary name/path
`)
}
