package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"audit-go/internal/features/processing"
	"audit-go/internal/platform/storage"
)

func TestRunOnceCompletesParseJob(t *testing.T) {
	job := processing.Job{
		ID:            "00000000-0000-0000-0000-000000000010",
		JobType:       processing.JobTypeParseDocument,
		AggregateType: processing.AggregateDocument,
		AggregateID:   "00000000-0000-0000-0000-000000000001",
		Status:        processing.JobRunning,
		Payload: map[string]any{
			"document_id": "00000000-0000-0000-0000-000000000001",
			"jv_id":       "00000000-0000-0000-0000-000000000002",
			"storage_key": "jvs/00000000-0000-0000-0000-000000000002/documents/00000000-0000-0000-0000-000000000001/raw/report.pdf",
		},
		Attempts:    1,
		MaxAttempts: 5,
	}
	jobs := &fakeJobRepository{job: &job}
	blobs := &fakeBlobReader{blob: storage.DownloadedBlob{Content: []byte("pdf-bytes")}}
	parser := &fakeParser{result: &processing.ParseResult{
		Filename: "report.pdf",
		Pages:    2,
		Markdown: "first paragraph\n\nsecond paragraph",
		Tables: []processing.ParsedTable{
			{Page: 1, Rows: [][]string{{"A", "B"}, {"1", "2"}}},
		},
	}}
	worker := New(zerolog.Nop(), jobs, blobs, parser, WithWorkerID("test-worker"))

	if processed := worker.RunOnce(context.Background()); !processed {
		t.Fatal("expected one job to be processed")
	}

	if jobs.claimWorkerID != "test-worker" {
		t.Fatalf("expected worker id to be passed to claim, got %q", jobs.claimWorkerID)
	}
	if blobs.storageKey != "jvs/00000000-0000-0000-0000-000000000002/documents/00000000-0000-0000-0000-000000000001/raw/report.pdf" {
		t.Fatalf("unexpected blob storage key %q", blobs.storageKey)
	}
	if parser.filename != "report.pdf" {
		t.Fatalf("expected parser filename report.pdf, got %q", parser.filename)
	}
	if jobs.completed == nil {
		t.Fatal("expected parse result to be completed")
	}
	if jobs.completed.DocumentID != "00000000-0000-0000-0000-000000000001" {
		t.Fatalf("expected document id to be set on result, got %q", jobs.completed.DocumentID)
	}
	if jobs.completed.RawStorageKey != "jvs/00000000-0000-0000-0000-000000000002/documents/00000000-0000-0000-0000-000000000001/raw/report.pdf" {
		t.Fatalf("expected raw storage key to be set, got %q", jobs.completed.RawStorageKey)
	}
	if len(jobs.completed.RawSHA256) != 64 {
		t.Fatalf("expected raw sha256 checksum, got %q", jobs.completed.RawSHA256)
	}
	if len(jobs.completed.MarkdownSHA256) != 64 || len(jobs.completed.TablesSHA256) != 64 {
		t.Fatalf("expected parsed artifact checksums, got markdown=%q tables=%q", jobs.completed.MarkdownSHA256, jobs.completed.TablesSHA256)
	}
	if len(jobs.completed.Chunks) != 1 || jobs.completed.Chunks[0].Content == "" {
		t.Fatalf("expected parsed chunks, got %#v", jobs.completed.Chunks)
	}
	if jobs.failed != nil {
		t.Fatalf("did not expect failure to be recorded: %v", jobs.failed)
	}
}

func TestRunOnceRecordsFailure(t *testing.T) {
	parseErr := errors.New("parse failed")
	job := processing.Job{
		ID:            "00000000-0000-0000-0000-000000000010",
		AggregateID:   "00000000-0000-0000-0000-000000000001",
		Attempts:      3,
		MaxAttempts:   5,
		AvailableAt:   time.Now().UTC(),
		AggregateType: processing.AggregateDocument,
		Payload: map[string]any{
			"document_id": "00000000-0000-0000-0000-000000000001",
			"jv_id":       "00000000-0000-0000-0000-000000000002",
			"storage_key": "jvs/jv/documents/doc/raw/report.pdf",
		},
	}
	jobs := &fakeJobRepository{job: &job}
	worker := New(
		zerolog.Nop(),
		jobs,
		&fakeBlobReader{blob: storage.DownloadedBlob{Content: []byte("pdf-bytes")}},
		&fakeParser{err: parseErr},
	)

	if processed := worker.RunOnce(context.Background()); !processed {
		t.Fatal("expected one job to be processed")
	}

	if jobs.failed == nil {
		t.Fatal("expected failure to be recorded")
	}
	if !errors.Is(jobs.failed, parseErr) {
		t.Fatalf("expected parse error to be wrapped, got %v", jobs.failed)
	}
	if jobs.retryDelay != 4*time.Minute {
		t.Fatalf("expected retry delay 4m, got %s", jobs.retryDelay)
	}
	if jobs.completed != nil {
		t.Fatalf("did not expect completion, got %#v", jobs.completed)
	}
}

func TestBuildChunksSplitsLargeParagraphs(t *testing.T) {
	large := make([]rune, maxChunkRunes+10)
	for i := range large {
		large[i] = 'a'
	}

	chunks := buildChunks(string(large), "")
	if len(chunks) != 2 {
		t.Fatalf("expected large paragraph to split into two chunks, got %d", len(chunks))
	}
	if chunks[0].Index != 0 || chunks[1].Index != 1 {
		t.Fatalf("expected stable chunk indexes, got %#v", chunks)
	}
}

type fakeJobRepository struct {
	job           *processing.Job
	claimWorkerID string
	completed     *processing.ParseResult
	failed        error
	retryDelay    time.Duration
}

func (f *fakeJobRepository) ClaimNextJob(
	_ context.Context,
	workerID string,
	_ time.Duration,
) (*processing.Job, error) {
	f.claimWorkerID = workerID
	return f.job, nil
}

func (f *fakeJobRepository) CompleteParseJob(_ context.Context, _ string, result processing.ParseResult) error {
	f.completed = &result
	return nil
}

func (f *fakeJobRepository) RecordJobFailure(
	_ context.Context,
	_ processing.Job,
	failure error,
	retryDelay time.Duration,
) error {
	f.failed = failure
	f.retryDelay = retryDelay
	return nil
}

type fakeBlobReader struct {
	storageKey string
	blob       storage.DownloadedBlob
	err        error
}

func (f *fakeBlobReader) Download(_ context.Context, storageKey string) (storage.DownloadedBlob, error) {
	f.storageKey = storageKey
	return f.blob, f.err
}

type fakeParser struct {
	filename string
	result   *processing.ParseResult
	err      error
}

func (f *fakeParser) ParseDocument(
	_ context.Context,
	filename string,
	_ []byte,
) (*processing.ParseResult, error) {
	f.filename = filename
	return f.result, f.err
}
