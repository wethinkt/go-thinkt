package share

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type UploadRequest struct {
	Visibility string          `json:"visibility"`
	Title      string          `json:"title"`
	Trace      json.RawMessage `json:"trace"`
}

type UploadResponse struct {
	ID         string `json:"id"`
	Slug       string `json:"slug"`
	URL        string `json:"url"`
	Visibility string `json:"visibility"`
	Error      string `json:"error,omitempty"`
}

type UploadClient struct {
	Endpoint   string
	Token      string
	HTTPClient *http.Client
}

func NewUploadClient(creds *Credentials) *UploadClient {
	return &UploadClient{
		Endpoint:   creds.Endpoint,
		Token:      creds.Token,
		HTTPClient: &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *UploadClient) Upload(traceData []byte, visibility, title string) (*UploadResponse, error) {
	reqBody := UploadRequest{
		Visibility: visibility,
		Title:      title,
		Trace:      json.RawMessage(traceData),
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.Endpoint+"/api/traces", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.Token)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upload trace: %w", err)
	}
	defer resp.Body.Close()

	var result UploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if resp.StatusCode != 201 {
		return nil, fmt.Errorf("upload failed (%d): %s", resp.StatusCode, result.Error)
	}

	return &result, nil
}
