// Package worker contains background workers used by processing.
package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"audit-go/internal/features/processing"
	"audit-go/internal/platform/storage"
)

const pollInterval = 30 * time.Second
const defaultLockDuration = 5 * time.Minute
const maxChunkRunes = 2_000

// FileWorker polls for unprocessed documents and sends them to the Python service.
type FileWorker struct {
	Log          zerolog.Logger
	jobs         jobRepository
	blobs        blobReader
	parser       documentParser
	workerID     string
	pollInterval time.Duration
	lockDuration time.Duration
}

type jobRepository interface {
	ClaimNextJob(ctx context.Context, workerID string, lockDuration time.Duration) (*processing.Job, error)
	CompleteParseJob(ctx context.Context, jobID string, result processing.ParseResult) error
	RecordJobFailure(ctx context.Context, job processing.Job, failure error, retryDelay time.Duration) error
}

type blobReader interface {
	Download(ctx context.Context, storageKey string) (storage.DownloadedBlob, error)
}

type documentParser interface {
	ParseDocument(ctx context.Context, filename string, content []byte) (*processing.ParseResult, error)
}

// Option customizes a FileWorker.
type Option func(*FileWorker)

// WithWorkerID overrides the generated worker id.
func WithWorkerID(workerID string) Option {
	return func(w *FileWorker) {
		if workerID != "" {
			w.workerID = workerID
		}
	}
}

// WithPollInterval overrides the default polling interval.
func WithPollInterval(interval time.Duration) Option {
	return func(w *FileWorker) {
		if interval > 0 {
			w.pollInterval = interval
		}
	}
}

// WithLockDuration overrides the default job lock duration.
func WithLockDuration(duration time.Duration) Option {
	return func(w *FileWorker) {
		if duration > 0 {
			w.lockDuration = duration
		}
	}
}

// New creates a new FileWorker with the given dependencies.
func New(
	log zerolog.Logger,
	jobs jobRepository,
	blobs blobReader,
	parser documentParser,
	options ...Option,
) *FileWorker {
	w := &FileWorker{
		Log:          log,
		jobs:         jobs,
		blobs:        blobs,
		parser:       parser,
		workerID:     defaultWorkerID(),
		pollInterval: pollInterval,
		lockDuration: defaultLockDuration,
	}

	for _, option := range options {
		option(w)
	}

	return w
}

// Start runs the worker loop until ctx is cancelled.
func (w *FileWorker) Start(ctx context.Context) {
	if !w.ready() {
		w.Log.Warn().Msg("file worker disabled because dependencies are not configured")
		return
	}

	w.Log.Info().Msg("file worker started")

	w.RunOnce(ctx)

	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.Log.Info().Msg("file worker stopped")
			return
		case <-ticker.C:
			w.poll(ctx)
		}
	}
}

// RunOnce claims and processes at most one job.
func (w *FileWorker) RunOnce(ctx context.Context) bool {
	if !w.ready() {
		w.Log.Warn().Msg("file worker skipped because dependencies are not configured")
		return false
	}

	return w.poll(ctx)
}

func (w *FileWorker) poll(ctx context.Context) bool {
	w.Log.Debug().Str("worker_id", w.workerID).Msg("worker polling for processing jobs")

	job, err := w.jobs.ClaimNextJob(ctx, w.workerID, w.lockDuration)
	if err != nil {
		w.Log.Error().Err(err).Msg("failed to claim processing job")
		return false
	}
	if job == nil {
		return false
	}

	log := w.Log.With().
		Str("job_id", job.ID).
		Str("document_id", job.AggregateID).
		Logger()
	log.Info().Msg("processing job claimed")

	if err = w.processJob(ctx, *job); err != nil {
		if ctx.Err() != nil {
			log.Warn().Err(ctx.Err()).Msg("processing job interrupted; lock will expire for retry")
			return true
		}

		delay := nextRetryDelay(job.Attempts)
		if recordErr := w.jobs.RecordJobFailure(ctx, *job, err, delay); recordErr != nil {
			log.Error().Err(recordErr).Msg("failed to record processing job failure")
			return true
		}

		log.Error().
			Err(err).
			Int("attempts", job.Attempts).
			Int("max_attempts", job.MaxAttempts).
			Dur("retry_delay", delay).
			Msg("processing job failed")
		return true
	}

	log.Info().Msg("processing job completed")
	return true
}

func (w *FileWorker) processJob(ctx context.Context, job processing.Job) error {
	payload, err := processing.DecodeParseDocumentPayload(job)
	if err != nil {
		return err
	}

	blob, err := w.blobs.Download(ctx, payload.StorageKey)
	if err != nil {
		return fmt.Errorf("downloading raw document: %w", err)
	}

	filename := path.Base(payload.StorageKey)
	if filename == "." || filename == "/" || filename == "" {
		filename = payload.DocumentID
	}

	result, err := w.parser.ParseDocument(ctx, filename, blob.Content)
	if err != nil {
		return fmt.Errorf("parsing document with python service: %w", err)
	}
	if result == nil {
		return fmt.Errorf("python service returned empty parse result")
	}

	result.DocumentID = payload.DocumentID
	result.RawStorageKey = payload.StorageKey
	if result.Filename == "" {
		result.Filename = filename
	}
	result.ParsedAt = time.Now().UTC()
	result.Chunks = buildChunks(result.Markdown, result.Text)
	result.RawSHA256 = checksumBytes(blob.Content)
	result.TextSHA256 = checksumString(result.Text)
	result.MarkdownSHA256 = checksumString(result.Markdown)
	result.TablesSHA256, err = checksumJSON(result.Tables)
	if err != nil {
		return err
	}

	if err = w.jobs.CompleteParseJob(ctx, job.ID, *result); err != nil {
		return fmt.Errorf("persisting parsed document: %w", err)
	}

	return nil
}

func (w *FileWorker) ready() bool {
	return w != nil && w.jobs != nil && w.blobs != nil && w.parser != nil
}

func defaultWorkerID() string {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "worker"
	}
	return hostname + "-" + uuid.NewString()
}

func nextRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 6 {
		attempt = 6
	}

	return time.Duration(1<<(attempt-1)) * time.Minute
}

func buildChunks(markdown string, text string) []processing.DocumentChunk {
	source := strings.TrimSpace(markdown)
	if source == "" {
		source = strings.TrimSpace(text)
	}
	if source == "" {
		return []processing.DocumentChunk{}
	}

	paragraphs := strings.Split(source, "\n\n")
	chunks := make([]processing.DocumentChunk, 0)
	var current strings.Builder

	flush := func() {
		content := strings.TrimSpace(current.String())
		if content == "" {
			current.Reset()
			return
		}
		chunks = append(chunks, processing.DocumentChunk{
			Index:   len(chunks),
			Content: content,
		})
		current.Reset()
	}

	for _, paragraph := range paragraphs {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			continue
		}

		if runeLen(paragraph) > maxChunkRunes {
			flush()
			for _, part := range splitRunes(paragraph, maxChunkRunes) {
				chunks = append(chunks, processing.DocumentChunk{
					Index:   len(chunks),
					Content: part,
				})
			}
			continue
		}

		nextLen := runeLen(current.String()) + runeLen(paragraph)
		if current.Len() > 0 {
			nextLen += 2
		}
		if nextLen > maxChunkRunes {
			flush()
		}

		if current.Len() > 0 {
			current.WriteString("\n\n")
		}
		current.WriteString(paragraph)
	}
	flush()

	return chunks
}

func splitRunes(value string, size int) []string {
	if size <= 0 {
		return []string{value}
	}

	runes := []rune(value)
	parts := make([]string, 0, (len(runes)/size)+1)
	for start := 0; start < len(runes); start += size {
		end := start + size
		if end > len(runes) {
			end = len(runes)
		}
		parts = append(parts, string(runes[start:end]))
	}

	return parts
}

func runeLen(value string) int {
	return len([]rune(value))
}

func checksumBytes(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func checksumString(value string) string {
	return checksumBytes([]byte(value))
}

func checksumJSON(value any) (string, error) {
	content, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshaling artifact for checksum: %w", err)
	}
	return checksumBytes(content), nil
}
