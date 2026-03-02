package app

import "time"

const (
	StatusQueued       = "queued"
	StatusPreparing    = "preparing"
	StatusTranscoding  = "transcoding"
	StatusTranscribing = "transcribing"
	StatusCompleted    = "completed"
	StatusFailed       = "failed"
	StatusCanceled     = "canceled"
)

// Job stores transcription state for queue processing.
type Job struct {
	ID         string    `json:"id"`
	FilePath   string    `json:"filePath"`
	OutputDir  string    `json:"outputDir"`
	Language   string    `json:"language"`
	Model      string    `json:"model"`
	Status     string    `json:"status"`
	Progress   int       `json:"progress"`
	Message    string    `json:"message"`
	Error      string    `json:"error,omitempty"`
	ResultText string    `json:"resultText,omitempty"`
	ResultSRT  string    `json:"resultSrt,omitempty"`
	ResultVTT  string    `json:"resultVtt,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
	StartedAt  time.Time `json:"startedAt,omitempty"`
	FinishedAt time.Time `json:"finishedAt,omitempty"`
}

// AddJobRequest is payload for queueing a new transcription job.
type AddJobRequest struct {
	FilePath  string `json:"filePath"`
	OutputDir string `json:"outputDir"`
	Language  string `json:"language"`
	Model     string `json:"model"`
}
