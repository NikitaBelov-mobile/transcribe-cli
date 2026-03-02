# transcribe-cli

Offline CLI tool for audio/video transcription on macOS and Windows.

- Full local processing (no server required)
- Simple one-shot mode: `init` + `run`
- Advanced queue mode with progress/cancel/retry
- Model management via `model presets/current/use/install/remove`
- Outputs: `txt`, `srt`, `vtt`

## Requirements

- `ffmpeg` in PATH
- `whisper-cli` in PATH (from `whisper.cpp` build)
- Go 1.23+ (for local build)

## Install

### Option A: from source

```bash
cd transcribe-cli
go build -o transcribe ./cmd/transcribe-cli
```

### Option B: release binary

Download from GitHub Releases and add binary to PATH.

## Easiest flow (recommended)

1. Prepare environment and model:

```bash
transcribe init
```

2. Transcribe file (daemon starts automatically if needed):

```bash
transcribe run ./sample.mp4 --lang ru --model ggml-base
```

`run` waits for completion and prints output file paths.

## Advanced queue flow

1. Start daemon manually:

```bash
transcribe daemon run
```

2. Add a job:

```bash
transcribe queue add --lang ru --model ggml-base ./sample.mp4
```

3. Watch job:

```bash
transcribe queue watch <job-id>
```

4. Cancel or retry:

```bash
transcribe queue cancel <job-id>
transcribe queue retry <job-id>
```

## Commands

```bash
transcribe init [--model ggml-base] [--skip-model]
transcribe run [--lang auto] [--model ggml-base] [--output-dir ./out] [--no-watch] [--interval 2s] <file>

transcribe setup
transcribe doctor
transcribe daemon run [--addr 127.0.0.1:9864] [--workers 4] [--queue-size 16]

transcribe model list
transcribe model presets
transcribe model current
transcribe model use ggml-base
transcribe model install --name ggml-base
transcribe model install --name my-custom --url https://example.com/model.bin
transcribe model remove ggml-base

transcribe queue add [--lang auto] [--model ggml-base] [--output-dir ./out] <file>
transcribe queue list
transcribe queue status <job-id>
transcribe queue watch <job-id> --interval 2s
transcribe queue cancel <job-id>
transcribe queue retry <job-id>
```

## Model handling

Models are stored in:

- macOS: `~/Library/Application Support/TranscribeCLI/models`
- Windows: `%AppData%\\TranscribeCLI\\models`

Canonical model names are `ggml-*` (for example `ggml-base`).

Preset aliases:

- `tiny` -> `ggml-tiny`
- `base` -> `ggml-base`
- `small` -> `ggml-small`
- `medium` -> `ggml-medium`
- `large` -> `ggml-large-v3`

By default, model names resolve to `<models-dir>/<name>.bin`.

You can also pass an absolute model file path in `--model`.

Default model is read in this order:

1. `TRANSCRIBE_CLI_DEFAULT_MODEL` environment variable
2. saved config in `<state-dir>/config.json`
3. fallback: `ggml-base`

## How progress works

Progress is stage-based:

- `queued` (0%)
- `preparing` (5%)
- `transcoding` (20%)
- `transcribing` (45..89%, updates every ~2 seconds while whisper runs)
- `completed` (100%), `failed` (100%), or `canceled` (100%)

Job states are persisted in `jobs.json`, so queue metadata survives daemon restart.

## Output files

For an input `lecture.mp4`, the daemon writes:

- `lecture.txt`
- `lecture.srt` (if produced)
- `lecture.vtt` (if produced)

Default output directory is the source file directory, override with `--output-dir`.

## Environment variables

- `TRANSCRIBE_CLI_ADDR` (default `127.0.0.1:9864`)
- `TRANSCRIBE_CLI_STATE_DIR` (default OS user config dir)
- `TRANSCRIBE_CLI_MODELS_DIR`
- `TRANSCRIBE_CLI_DEFAULT_MODEL`
- `TRANSCRIBE_CLI_WORKERS`
- `TRANSCRIBE_CLI_QUEUE_SIZE`
- `TRANSCRIBE_CLI_FFMPEG` (default `ffmpeg`)
- `TRANSCRIBE_CLI_WHISPER` (default `whisper-cli`)

## Release Automation

Release pipeline is configured in `.github/workflows/release.yml`.

On tag push (`v*`) it:

1. Builds cross-platform artifacts with GoReleaser.
2. Publishes GitHub Release artifacts.
3. Optionally updates Homebrew tap via `scripts/release/update-homebrew-tap.sh`.
4. Optionally updates Winget manifests repo via `scripts/release/update-winget-manifests.sh`.

Required GitHub repo config:

- Variables:
  - `RELEASE_REPO` (e.g. `your-org/transcribe-cli`)
  - `HOMEBREW_TAP_REPO` (optional, e.g. `your-org/homebrew-tap`)
  - `WINGET_REPO` (optional, target manifests repo)
  - `WINGET_PACKAGE_ID` (optional, e.g. `YourOrg.TranscribeCLI`)
- Secrets:
  - `HOMEBREW_TAP_TOKEN` (optional)
  - `WINGET_REPO_TOKEN` (optional)
