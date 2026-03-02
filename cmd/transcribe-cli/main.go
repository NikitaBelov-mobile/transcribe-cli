package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	"transcribe-cli/internal/app"
)

func main() {
	cfg := app.LoadConfig()

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
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
	return daemon.Run(ctx)
}

func runQueue(cfg app.Config, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: transcribe queue <add|list|status|watch|cancel|retry>")
	}
	client := app.NewClient(cfg.ClientBaseURL)

	switch args[0] {
	case "add":
		fs := flag.NewFlagSet("queue add", flag.ContinueOnError)
		language := fs.String("lang", "auto", "language code (or auto)")
		model := fs.String("model", "ggml-base", "model name or absolute model path")
		outputDir := fs.String("output-dir", "", "output directory for txt/srt/vtt")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		rest := fs.Args()
		if len(rest) != 1 {
			return errors.New("usage: transcribe queue add <file> [--lang ru] [--model ggml-base]")
		}
		filePath, err := filepath.Abs(rest[0])
		if err != nil {
			return err
		}
		job, err := client.AddJob(app.AddJobRequest{
			FilePath:  filePath,
			OutputDir: strings.TrimSpace(*outputDir),
			Language:  strings.TrimSpace(*language),
			Model:     strings.TrimSpace(*model),
		})
		if err != nil {
			return err
		}
		fmt.Printf("Queued: %s\n", job.ID)
		fmt.Printf("Status: %s (%d%%)\n", job.Status, job.Progress)
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
		fmt.Println("ID\tSTATUS\tPROGRESS\tFILE")
		for _, job := range jobs {
			fmt.Printf("%s\t%s\t%d%%\t%s\n", job.ID, job.Status, job.Progress, job.FilePath)
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

func runModel(cfg app.Config, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: transcribe model <list|install|remove>")
	}

	switch args[0] {
	case "list":
		models, err := app.ListModels(cfg)
		if err != nil {
			return err
		}
		if len(models) == 0 {
			fmt.Println("No local models installed.")
			fmt.Println("Use: transcribe model install --name base")
			return nil
		}
		fmt.Println("NAME\tSIZE\tPATH")
		for _, m := range models {
			fmt.Printf("%s\t%s\t%s\n", m.Name, humanSize(m.SizeBytes), m.Path)
		}
		return nil

	case "install":
		fs := flag.NewFlagSet("model install", flag.ContinueOnError)
		name := fs.String("name", "base", "model name (preset: tiny|base|small|medium|large)")
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
		fmt.Printf("Removed model: %s\n", args[1])
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
	fmt.Printf("Models directory: %s\n", cfg.ModelsDir)
	fmt.Printf("Daemon address: %s\n", cfg.Addr)
	fmt.Println("Setup complete.")
	return nil
}

func runDoctor(cfg app.Config) error {
	if err := app.EnsureStateDirs(cfg); err != nil {
		return err
	}
	fmt.Printf("OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Printf("State: %s\n", cfg.StateDir)
	fmt.Printf("Models: %s\n", cfg.ModelsDir)

	checkBinary(cfg.FFmpegBinary, "ffmpeg")
	checkBinary(cfg.WhisperBinary, "whisper")

	client := app.NewClient(cfg.ClientBaseURL)
	if err := client.Health(); err != nil {
		fmt.Printf("daemon: not reachable (%v)\n", err)
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
	fmt.Println(`transcribe - offline transcription CLI

Commands:
  setup                           Initialize local state directories
  doctor                          Check local dependencies and daemon health
  daemon run                      Start local queue daemon

  model list                      List installed models
  model install --name base       Download model preset
  model remove <name>             Remove model

  queue add <file> [flags]        Add audio/video file to queue
  queue list                      Show queue jobs
  queue status <job-id>           Show detailed job JSON
  queue watch <job-id>            Poll job progress until done
  queue cancel <job-id>           Cancel queued/running job
  queue retry <job-id>            Retry failed/canceled job

Environment variables:
  TRANSCRIBE_CLI_ADDR             Daemon listen address (default 127.0.0.1:9864)
  TRANSCRIBE_CLI_STATE_DIR        State directory (default OS user config dir)
  TRANSCRIBE_CLI_MODELS_DIR       Models directory
  TRANSCRIBE_CLI_FFMPEG           ffmpeg binary name/path
  TRANSCRIBE_CLI_WHISPER          whisper-cli binary name/path
`)
}
