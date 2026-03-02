package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Daemon runs local queue and transcription workers.
type Daemon struct {
	cfg   Config
	store *Store

	mu     sync.RWMutex
	jobs   map[string]*Job
	active map[string]context.CancelFunc

	queue chan string
}

func NewDaemon(cfg Config) (*Daemon, error) {
	if err := EnsureStateDirs(cfg); err != nil {
		return nil, err
	}

	store := NewStore(cfg.JobsFile)
	jobs, err := store.Load()
	if err != nil {
		return nil, err
	}

	changed := false
	for _, job := range jobs {
		switch job.Status {
		case StatusPreparing, StatusTranscoding, StatusTranscribing:
			job.Status = StatusQueued
			job.Progress = 0
			job.Message = "resumed after daemon restart"
			job.Error = ""
			job.FinishedAt = time.Time{}
			changed = true
		}
	}
	if changed {
		if err := store.Save(jobs); err != nil {
			return nil, err
		}
	}

	return &Daemon{
		cfg:    cfg,
		store:  store,
		jobs:   jobs,
		active: make(map[string]context.CancelFunc),
		queue:  make(chan string, cfg.QueueSize),
	}, nil
}

func (d *Daemon) Run(ctx context.Context) error {
	for i := 0; i < d.cfg.Workers; i++ {
		go d.worker(ctx, i+1)
	}
	go d.enqueuePending(ctx)

	mux := http.NewServeMux()
	d.registerUIRoutes(mux)
	mux.HandleFunc("/healthz", d.handleHealth)
	mux.HandleFunc("/v1/jobs", d.handleJobs)
	mux.HandleFunc("/v1/jobs/", d.handleJobPath)
	mux.HandleFunc("/v1/jobs/upload", d.handleUploadJob)
	mux.HandleFunc("/v1/models", d.handleModels)
	mux.HandleFunc("/v1/models/presets", d.handleModelPresets)
	mux.HandleFunc("/v1/models/install", d.handleModelInstall)
	mux.HandleFunc("/v1/models/use", d.handleModelUse)
	mux.HandleFunc("/v1/models/remove", d.handleModelRemove)

	server := &http.Server{
		Addr:              d.cfg.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	err := server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (d *Daemon) enqueuePending(ctx context.Context) {
	jobs := d.ListJobs()
	for _, job := range jobs {
		if job.Status != StatusQueued {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case d.queue <- job.ID:
		}
	}
}

func (d *Daemon) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (d *Daemon) handleJobs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string][]*Job{"jobs": d.ListJobs()})
	case http.MethodPost:
		d.handleAddJob(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (d *Daemon) handleAddJob(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	defer r.Body.Close()

	var req AddJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	req.FilePath = strings.TrimSpace(req.FilePath)
	if req.FilePath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "filePath is required"})
		return
	}

	job, err := d.createAndQueueJob(req.FilePath, req.Language, req.Model, req.OutputDir)
	if err != nil {
		writeJSON(w, statusFromError(err), map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

func (d *Daemon) handleUploadJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	// Keep only a small multipart buffer in memory; large uploads go to temp files.
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart payload"})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file is required"})
		return
	}
	defer file.Close()

	safeName := filepath.Base(strings.TrimSpace(header.Filename))
	if safeName == "" || safeName == "." || safeName == string(os.PathSeparator) {
		safeName = "upload.bin"
	}
	dstPath := filepath.Join(d.cfg.UploadsDir, makeID()+"-"+safeName)
	dst, err := os.Create(dstPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create upload file"})
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		_ = os.Remove(dstPath)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save upload"})
		return
	}

	language := r.FormValue("language")
	model := r.FormValue("model")
	outputDir := strings.TrimSpace(r.FormValue("outputDir"))
	if outputDir == "" {
		outputDir = d.cfg.OutputsDir
	}

	job, err := d.createAndQueueJob(dstPath, language, model, outputDir)
	if err != nil {
		_ = os.Remove(dstPath)
		writeJSON(w, statusFromError(err), map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

func (d *Daemon) createAndQueueJob(filePath, language, model, outputDir string) (*Job, error) {
	absPath, err := filepath.Abs(strings.TrimSpace(filePath))
	if err != nil {
		return nil, fmt.Errorf("invalid file path")
	}
	info, err := os.Stat(absPath)
	if err != nil || info.IsDir() {
		return nil, fmt.Errorf("file does not exist or is a directory")
	}

	language = strings.TrimSpace(language)
	if language == "" {
		language = "auto"
	}

	model = strings.TrimSpace(model)
	if model == "" {
		model = d.cfg.DefaultModel
	}
	if !filepath.IsAbs(model) && !strings.ContainsRune(model, os.PathSeparator) {
		model = CanonicalModelName(model)
	}

	outputDir = strings.TrimSpace(outputDir)
	if outputDir == "" {
		outputDir = filepath.Dir(absPath)
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("invalid outputDir")
	}

	now := time.Now().UTC()
	job := &Job{
		ID:        makeID(),
		FilePath:  absPath,
		OutputDir: outputDir,
		Language:  language,
		Model:     model,
		Status:    StatusQueued,
		Progress:  0,
		Message:   "queued",
		CreatedAt: now,
	}

	snapshot, err := d.insertJob(job)
	if err != nil {
		return nil, err
	}
	if err := d.store.Save(snapshot); err != nil {
		return nil, err
	}

	select {
	case d.queue <- job.ID:
		return job, nil
	default:
		snapshot, _ := d.failJob(job.ID, "queue is full")
		if snapshot != nil {
			_ = d.store.Save(snapshot)
		}
		return nil, errQueueFull
	}
}

func (d *Daemon) handleJobPath(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/v1/jobs/")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "job id is required"})
		return
	}

	parts := strings.Split(trimmed, "/")
	jobID := strings.TrimSpace(parts[0])
	if jobID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "job id is required"})
		return
	}

	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		job, ok := d.GetJob(jobID)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
			return
		}
		writeJSON(w, http.StatusOK, job)
		return
	}

	if len(parts) == 3 && parts[1] == "result" && r.Method == http.MethodGet {
		d.handleJobResultFile(w, r, jobID, strings.TrimSpace(parts[2]))
		return
	}
	if len(parts) != 2 || r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	action := strings.TrimSpace(parts[1])
	switch action {
	case "cancel":
		job, err := d.CancelJob(jobID)
		if err != nil {
			writeJSON(w, statusFromError(err), map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, job)
	case "retry":
		job, err := d.RetryJob(jobID)
		if err != nil {
			writeJSON(w, statusFromError(err), map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusAccepted, job)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown action"})
	}
}

func (d *Daemon) handleJobResultFile(w http.ResponseWriter, r *http.Request, jobID, format string) {
	job, ok := d.GetJob(jobID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
		return
	}
	if job.Status != StatusCompleted {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "job is not completed"})
		return
	}

	var path string
	switch format {
	case "txt":
		path = job.ResultText
	case "srt":
		path = job.ResultSRT
	case "vtt":
		path = job.ResultVTT
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown result format"})
		return
	}
	if strings.TrimSpace(path) == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "result file is not available"})
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename="+filepath.Base(path))
	http.ServeFile(w, r, path)
}

func (d *Daemon) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	models, err := ListModels(d.cfg)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	type modelView struct {
		Name      string `json:"name"`
		Path      string `json:"path"`
		SizeBytes int64  `json:"sizeBytes"`
	}
	items := make([]modelView, 0, len(models))
	for _, m := range models {
		items = append(items, modelView{Name: m.Name, Path: m.Path, SizeBytes: m.SizeBytes})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"defaultModel": d.cfg.DefaultModel,
		"modelsDir":    d.cfg.ModelsDir,
		"models":       items,
	})
}

func (d *Daemon) handleModelPresets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"presets": ListPresetModels()})
}

func (d *Daemon) handleModelInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	defer r.Body.Close()

	var req ModelInstallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	path, err := InstallModel(d.cfg, req.Name, req.URL, nil)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": path})
}

func (d *Daemon) handleModelUse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	defer r.Body.Close()

	var req ModelUseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	model := strings.TrimSpace(req.Name)
	if model == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	if !filepath.IsAbs(model) && !strings.ContainsRune(model, os.PathSeparator) {
		model = CanonicalModelName(model)
	}

	settings, err := LoadSettings(d.cfg.SettingsFile)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	settings.DefaultModel = model
	if err := SaveSettings(d.cfg.SettingsFile, settings); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	d.cfg.DefaultModel = model
	writeJSON(w, http.StatusOK, map[string]string{"defaultModel": model})
}

func (d *Daemon) handleModelRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	defer r.Body.Close()

	var req ModelUseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if err := RemoveModel(d.cfg, req.Name); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"removed": CanonicalModelName(req.Name)})
}

func statusFromError(err error) int {
	if err == nil {
		return http.StatusOK
	}
	if errors.Is(err, errNotFound) {
		return http.StatusNotFound
	}
	if errors.Is(err, errQueueFull) {
		return http.StatusServiceUnavailable
	}
	return http.StatusBadRequest
}

var (
	errNotFound  = errors.New("job not found")
	errQueueFull = errors.New("queue is full")
)

func (d *Daemon) CancelJob(id string) (*Job, error) {
	now := time.Now().UTC()
	var cancel context.CancelFunc

	snapshot, updated, err := d.updateJobWithError(id, func(job *Job) error {
		switch job.Status {
		case StatusCompleted:
			return fmt.Errorf("job is already completed")
		case StatusFailed:
			return fmt.Errorf("job is already failed; use retry")
		case StatusCanceled:
			return fmt.Errorf("job is already canceled")
		case StatusQueued, StatusPreparing, StatusTranscoding, StatusTranscribing:
			job.Status = StatusCanceled
			job.Progress = 100
			job.Message = "canceled by user"
			job.Error = ""
			job.FinishedAt = now
			if fn, ok := d.active[id]; ok {
				cancel = fn
			}
			return nil
		default:
			return fmt.Errorf("job is in unsupported status: %s", job.Status)
		}
	})
	if err != nil {
		return nil, err
	}
	if !updated {
		return nil, errNotFound
	}

	if cancel != nil {
		cancel()
	}
	if err := d.store.Save(snapshot); err != nil {
		return nil, err
	}
	job, _ := d.GetJob(id)
	return job, nil
}

func (d *Daemon) RetryJob(id string) (*Job, error) {
	snapshot, updated, err := d.updateJobWithError(id, func(job *Job) error {
		switch job.Status {
		case StatusFailed, StatusCanceled:
			job.Status = StatusQueued
			job.Progress = 0
			job.Message = "re-queued"
			job.Error = ""
			job.ResultText = ""
			job.ResultSRT = ""
			job.ResultVTT = ""
			job.StartedAt = time.Time{}
			job.FinishedAt = time.Time{}
			return nil
		case StatusCompleted:
			return fmt.Errorf("completed job cannot be retried")
		default:
			return fmt.Errorf("only failed or canceled jobs can be retried")
		}
	})
	if err != nil {
		return nil, err
	}
	if !updated {
		return nil, errNotFound
	}
	if err := d.store.Save(snapshot); err != nil {
		return nil, err
	}

	select {
	case d.queue <- id:
		job, _ := d.GetJob(id)
		return job, nil
	default:
		if failSnapshot, ok := d.failJob(id, "queue is full"); ok {
			_ = d.store.Save(failSnapshot)
		}
		return nil, errQueueFull
	}
}

func (d *Daemon) GetJob(id string) (*Job, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	job, ok := d.jobs[id]
	if !ok {
		return nil, false
	}
	return cloneJob(job), true
}

func (d *Daemon) ListJobs() []*Job {
	d.mu.RLock()
	defer d.mu.RUnlock()
	jobs := make([]*Job, 0, len(d.jobs))
	for _, job := range d.jobs {
		jobs = append(jobs, cloneJob(job))
	}
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].CreatedAt.After(jobs[j].CreatedAt)
	})
	return jobs
}

func (d *Daemon) insertJob(job *Job) (map[string]*Job, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, exists := d.jobs[job.ID]; exists {
		return nil, errors.New("job id collision")
	}
	d.jobs[job.ID] = cloneJob(job)
	return cloneJobs(d.jobs), nil
}

func (d *Daemon) updateJob(id string, updater func(*Job)) (map[string]*Job, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	job, ok := d.jobs[id]
	if !ok {
		return nil, false
	}
	updater(job)
	return cloneJobs(d.jobs), true
}

func (d *Daemon) updateJobWithError(id string, updater func(*Job) error) (map[string]*Job, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	job, ok := d.jobs[id]
	if !ok {
		return nil, false, nil
	}
	if err := updater(job); err != nil {
		return nil, true, err
	}
	return cloneJobs(d.jobs), true, nil
}

func (d *Daemon) withJob(id string, fn func(*Job)) (*Job, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	job, ok := d.jobs[id]
	if !ok {
		return nil, false
	}
	fn(job)
	return cloneJob(job), true
}

func (d *Daemon) setActiveCancel(id string, cancel context.CancelFunc) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.active[id] = cancel
}

func (d *Daemon) clearActiveCancel(id string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.active, id)
}

func (d *Daemon) failJob(id, errMessage string) (map[string]*Job, bool) {
	return d.updateJob(id, func(job *Job) {
		job.Status = StatusFailed
		job.Progress = 100
		job.Error = errMessage
		job.Message = "failed"
		job.FinishedAt = time.Now().UTC()
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func makeID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(buf)
}
