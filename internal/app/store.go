package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

type Store struct {
	path string
	mu   sync.Mutex
}

type jobsEnvelope struct {
	Jobs []*Job `json:"jobs"`
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Load() (map[string]*Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	payload, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return make(map[string]*Job), nil
	}
	if err != nil {
		return nil, err
	}

	var envelope jobsEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return nil, err
	}

	jobs := make(map[string]*Job, len(envelope.Jobs))
	for _, job := range envelope.Jobs {
		jobs[job.ID] = cloneJob(job)
	}
	return jobs, nil
}

func (s *Store) Save(jobs map[string]*Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	list := make([]*Job, 0, len(jobs))
	for _, job := range jobs {
		list = append(list, cloneJob(job))
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.After(list[j].CreatedAt)
	})

	payload, err := json.MarshalIndent(jobsEnvelope{Jobs: list}, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := s.path + ".tmp"
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(tmpPath, payload, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, s.path)
}

func cloneJobs(src map[string]*Job) map[string]*Job {
	out := make(map[string]*Job, len(src))
	for id, job := range src {
		out[id] = cloneJob(job)
	}
	return out
}

func cloneJob(job *Job) *Job {
	if job == nil {
		return nil
	}
	copyJob := *job
	return &copyJob
}
