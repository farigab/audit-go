// Package python provides a client to the Python parsing service.
package python

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"time"

	"audit-go/internal/features/processing"
)

const defaultTimeout = 60 * time.Second

// ParsedTable represents one extracted table from any parser.
type ParsedTable = processing.ParsedTable

// ParseResult represents the parsed output returned by the Python service.
type ParseResult = processing.ParseResult

// ChatRequest is the payload sent to the Python chatbot endpoint.
type ChatRequest struct {
	Context      string  `json:"context"`
	Question     string  `json:"question"`
	SystemPrompt string  `json:"system_prompt"`
	UserTemplate string  `json:"user_template"`
	Model        string  `json:"model"`
	Temperature  float64 `json:"temperature"`
}

// ChatResponse is the chatbot response returned by the Python service.
type ChatResponse struct {
	Answer string `json:"answer"`
}

// Client is a lightweight client for the Python parsing service.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new Python service client.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: defaultTimeout},
	}
}

// NewClientWithHTTP creates a client using the provided http.Client.
func NewClientWithHTTP(baseURL string, hc *http.Client) *Client {
	return &Client{baseURL: baseURL, httpClient: hc}
}

// ParseDocument uploads a file to the Python service and returns the parsed result.
func (c *Client) ParseDocument(ctx context.Context, filename string, content []byte) (*ParseResult, error) {
	body, contentType, err := buildMultipart(filename, content)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/parse", body)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("request cancelled: %w", ctx.Err())
		}
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

// Chat asks the Python service to answer a question using the provided prompt and context.
func (c *Client) Chat(ctx context.Context, reqBody ChatRequest) (ChatResponse, error) {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("marshaling chat request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat", bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, fmt.Errorf("creating chat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ChatResponse{}, fmt.Errorf("chat request cancelled: %w", ctx.Err())
		}
		return ChatResponse{}, fmt.Errorf("calling python chat service: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		b, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return ChatResponse{}, fmt.Errorf("python chat service error %d (could not read body): %w", resp.StatusCode, readErr)
		}
		return ChatResponse{}, fmt.Errorf("python chat service error %d: %s", resp.StatusCode, b)
	}

	var result ChatResponse
	if err = json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return ChatResponse{}, fmt.Errorf("decoding chat response: %w", err)
	}

	return result, nil
}

// Health checks if the Python service is alive.
func (c *Client) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return fmt.Errorf("creating health request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("python service unreachable: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("python service unhealthy: status %d", resp.StatusCode)
	}

	return nil
}

func buildMultipart(filename string, content []byte) (*bytes.Buffer, string, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", filepath.Base(filename))
	if err != nil {
		return nil, "", fmt.Errorf("creating form file: %w", err)
	}

	if _, err = part.Write(content); err != nil {
		return nil, "", fmt.Errorf("writing file content: %w", err)
	}

	if err = writer.Close(); err != nil {
		return nil, "", fmt.Errorf("closing multipart writer: %w", err)
	}

	return body, writer.FormDataContentType(), nil
}
