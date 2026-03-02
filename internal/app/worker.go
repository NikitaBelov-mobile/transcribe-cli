package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func (d *Daemon) worker(ctx context.Context, _ int) {
	for {
		select {
		case <-ctx.Done():
			return
		case id := <-d.queue:
			d.processJob(ctx, id)
		}
	}
}

func (d *Daemon) processJob(ctx context.Context, id string) {
	job, ok := d.GetJob(id)
	if !ok {
		return
	}
	if job.Status == StatusCompleted || job.Status == StatusFailed || job.Status == StatusCanceled {
		return
	}

	jobCtx, cancel := context.WithCancel(ctx)
	d.setActiveCancel(id, cancel)
	defer func() {
		cancel()
		d.clearActiveCancel(id)
	}()

	if snapshot, ok := d.updateJob(id, func(job *Job) {
		job.Status = StatusPreparing
		job.Progress = 5
		job.Message = "preparing"
		job.Error = ""
		if job.StartedAt.IsZero() {
			job.StartedAt = time.Now().UTC()
		}
	}); ok {
		_ = d.store.Save(snapshot)
	}

	textPath, srtPath, vttPath, err := d.runTranscription(jobCtx, job)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			shouldRequeue := true
			snapshot, ok := d.updateJob(id, func(job *Job) {
				if job.Status == StatusCanceled {
					shouldRequeue = false
					return
				}
				job.Status = StatusQueued
				job.Progress = 0
				job.Message = "interrupted, queued for retry"
				job.Error = ""
				job.FinishedAt = time.Time{}
			})
			if ok {
				_ = d.store.Save(snapshot)
			}
			if shouldRequeue && ctx.Err() == nil {
				select {
				case d.queue <- id:
				default:
					if failSnapshot, ok := d.failJob(id, "queue is full"); ok {
						_ = d.store.Save(failSnapshot)
					}
				}
			}
			return
		}

		if snapshot, ok := d.updateJob(id, func(job *Job) {
			job.Status = StatusFailed
			job.Progress = 100
			job.Message = "failed"
			job.Error = err.Error()
			job.FinishedAt = time.Now().UTC()
		}); ok {
			_ = d.store.Save(snapshot)
		}
		return
	}

	if snapshot, ok := d.updateJob(id, func(job *Job) {
		job.Status = StatusCompleted
		job.Progress = 100
		job.Message = "completed"
		job.ResultText = textPath
		job.ResultSRT = srtPath
		job.ResultVTT = vttPath
		job.FinishedAt = time.Now().UTC()
	}); ok {
		_ = d.store.Save(snapshot)
	}
}

func (d *Daemon) runTranscription(ctx context.Context, job *Job) (string, string, string, error) {
	tempDir, err := os.MkdirTemp("", "transcribe-cli-*")
	if err != nil {
		return "", "", "", err
	}
	defer os.RemoveAll(tempDir)

	wavPath := filepath.Join(tempDir, "input-16k.wav")
	if snapshot, ok := d.updateJob(job.ID, func(job *Job) {
		job.Status = StatusTranscoding
		job.Progress = 20
		job.Message = "extracting audio"
	}); ok {
		_ = d.store.Save(snapshot)
	}

	if err := d.extractAudio(ctx, job.FilePath, wavPath); err != nil {
		return "", "", "", err
	}

	if snapshot, ok := d.updateJob(job.ID, func(job *Job) {
		job.Status = StatusTranscribing
		job.Progress = 45
		job.Message = "running transcription"
	}); ok {
		_ = d.store.Save(snapshot)
	}

	if err := os.MkdirAll(job.OutputDir, 0o755); err != nil {
		return "", "", "", err
	}
	baseName := strings.TrimSuffix(filepath.Base(job.FilePath), filepath.Ext(job.FilePath))
	outputBase := filepath.Join(tempDir, "transcript")
	wavSeconds := estimateWAVSeconds(wavPath)

	if err := d.runWhisper(ctx, wavPath, outputBase, job.Language, job.Model, func(progress int) {
		snapshot, ok := d.updateJob(job.ID, func(j *Job) {
			if j.Status == StatusCanceled || j.Status == StatusFailed || j.Status == StatusCompleted {
				return
			}
			if progress > j.Progress {
				j.Progress = progress
			}
			j.Message = "running transcription"
		})
		if ok {
			_ = d.store.Save(snapshot)
		}
	}, wavSeconds); err != nil {
		return "", "", "", err
	}

	if snapshot, ok := d.updateJob(job.ID, func(job *Job) {
		job.Progress = 90
		job.Message = "writing output files"
	}); ok {
		_ = d.store.Save(snapshot)
	}

	textDst := filepath.Join(job.OutputDir, baseName+".txt")
	srtDst := filepath.Join(job.OutputDir, baseName+".srt")
	vttDst := filepath.Join(job.OutputDir, baseName+".vtt")

	if err := copyFile(filepath.Join(outputBase+".txt"), textDst); err != nil {
		return "", "", "", err
	}
	if err := copyIfExists(filepath.Join(outputBase+".srt"), srtDst); err != nil {
		return "", "", "", err
	}
	if err := copyIfExists(filepath.Join(outputBase+".vtt"), vttDst); err != nil {
		return "", "", "", err
	}

	if _, err := os.Stat(srtDst); err != nil {
		srtDst = ""
	}
	if _, err := os.Stat(vttDst); err != nil {
		vttDst = ""
	}

	return textDst, srtDst, vttDst, nil
}

func (d *Daemon) extractAudio(ctx context.Context, inputPath, outputPath string) error {
	cfg := d.currentConfig()
	cmd := exec.CommandContext(ctx, cfg.FFmpegBinary,
		"-y",
		"-i", inputPath,
		"-vn",
		"-ac", "1",
		"-ar", "16000",
		"-f", "wav",
		outputPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (d *Daemon) runWhisper(ctx context.Context, wavPath, outputBase, language, model string, onProgress func(int), wavSeconds float64) error {
	cfg := d.currentConfig()
	modelPath, err := ResolveModelPath(cfg, model)
	if err != nil {
		return err
	}

	args := []string{
		"-m", modelPath,
		"-f", wavPath,
		"-otxt",
		"-osrt",
		"-ovtt",
		"-of", outputBase,
	}
	language = strings.TrimSpace(language)
	if language != "" && language != "auto" {
		args = append(args, "-l", language)
	}

	cmd := exec.CommandContext(ctx, cfg.WhisperBinary, args...)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	if err := cmd.Start(); err != nil {
		return err
	}

	started := time.Now()
	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case err := <-waitCh:
			if err != nil {
				return fmt.Errorf("whisper failed: %w: %s", err, strings.TrimSpace(output.String()))
			}
			if onProgress != nil {
				onProgress(89)
			}
			return nil
		case <-ticker.C:
			if onProgress != nil {
				onProgress(estimateTranscribeProgress(started, wavSeconds))
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func estimateTranscribeProgress(started time.Time, wavSeconds float64) int {
	elapsed := time.Since(started).Seconds()
	estimatedTotal := math.Max(20.0, wavSeconds*1.2)
	ratio := elapsed / estimatedTotal
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	value := 45 + int(ratio*43.0)
	if value < 46 {
		value = 46
	}
	if value > 88 {
		value = 88
	}
	return value
}

func estimateWAVSeconds(wavPath string) float64 {
	info, err := os.Stat(wavPath)
	if err != nil {
		return 0
	}
	if info.Size() <= 44 {
		return 0
	}
	// WAV generated by ffmpeg is mono PCM16 16kHz: 32000 bytes per second.
	return float64(info.Size()-44) / 32000.0
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := out.ReadFrom(in); err != nil {
		return err
	}
	return out.Sync()
}

func copyIfExists(src, dst string) error {
	if _, err := os.Stat(src); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return copyFile(src, dst)
}
