// Package python provides a client to the Python parsing microservice.
package python

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"time"
)

const defaultTimeout = 60 * time.Second // parsing large PDFs can be slow

// ParseResult represents the parsed output returned by the Python service.
type ParseResult struct {
	Filename string       `json:"filename"`
	Pages    int          `json:"pages"`
	Text     string       `json:"text"`
	Markdown string       `json:"markdown"`
	Tables   [][][]string `json:"tables"`
}

// Client is a lightweight client for the Python parsing service.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new Python service client with a sensible default timeout.
// Pass a custom http.Client via NewClientWithHTTP when you need finer control
// (e.g. different timeouts per environment).
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
	}
}

// NewClientWithHTTP creates a client using the provided http.Client.
// Useful for tests and custom transport configurations.
func NewClientWithHTTP(baseURL string, hc *http.Client) *Client {
	return &Client{baseURL: baseURL, httpClient: hc}
}

// ParseDocument uploads a file to the Python service and returns the parsed result.
func (c *Client) ParseDocument(filename string, content []byte) (*ParseResult, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", filepath.Base(filename))
	if err != nil {
		return nil, fmt.Errorf("creating form file: %w", err)
	}

	if _, err = part.Write(content); err != nil {
		return nil, fmt.Errorf("writing file content: %w", err)
	}

	if err = writer.Close(); err != nil {
		return nil, fmt.Errorf("closing multipart writer: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/parse", body)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling python service: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		b, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return nil, fmt.Errorf("python service error %d (could not read body): %w", resp.StatusCode, readErr)
		}
		return nil, fmt.Errorf("python service error %d: %s", resp.StatusCode, b)
	}

	var result ParseResult
	if err = json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &result, nil
}
