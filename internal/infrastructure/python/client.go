// Package python provides a client to the Python parsing microservice.
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
)

const defaultTimeout = 60 * time.Second

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

// NewClient creates a new Python service client.
// O timeout de 60s é um fallback de segurança — o context do caller
// sempre tem precedência. Se o worker cancelar o contexto antes de 60s,
// a chamada HTTP é abortada imediatamente.
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
// O ctx é propagado para o http.Request, garantindo que:
//   - Cancelamento do contexto (ex: SIGTERM no worker) aborta o upload imediatamente.
//   - Deadlines do caller (ex: timeout de um handler HTTP) são respeitados.
//   - O timeout de 60s no httpClient serve só como proteção de último recurso.
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
		// Contexto cancelado? Retorna o erro do contexto diretamente —
		// mais informativo do que "connection reset".
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

// Health checks if the Python service is alive.
// Útil para health checks compostos no endpoint /health do Go.
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

// buildMultipart constrói o body multipart e retorna o content-type correto.
// Separado em função própria para facilitar testes unitários do encoding
// sem precisar subir um servidor HTTP.
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
