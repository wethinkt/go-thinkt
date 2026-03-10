package share

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type TokenResponse struct {
	Token string `json:"token"`
	Error string `json:"error,omitempty"`
	User  struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	} `json:"user,omitempty"`
}

type DeviceFlowClient struct {
	Endpoint   string
	HTTPClient *http.Client
}

func NewDeviceFlowClient(endpoint string) *DeviceFlowClient {
	return &DeviceFlowClient{
		Endpoint:   endpoint,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *DeviceFlowClient) RequestCode() (*DeviceCodeResponse, error) {
	resp, err := c.HTTPClient.Post(c.Endpoint+"/api/auth/device/code", "application/json", nil)
	if err != nil {
		return nil, fmt.Errorf("request device code: %w", err)
	}
	defer resp.Body.Close()

	var result DeviceCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode device code response: %w", err)
	}
	return &result, nil
}

func (c *DeviceFlowClient) PollForToken(deviceCode string, interval int) (*TokenResponse, error) {
	body, _ := json.Marshal(map[string]string{"device_code": deviceCode})

	for {
		time.Sleep(time.Duration(interval) * time.Second)

		resp, err := c.HTTPClient.Post(
			c.Endpoint+"/api/auth/device/token",
			"application/json",
			bytes.NewReader(body),
		)
		if err != nil {
			return nil, fmt.Errorf("poll token: %w", err)
		}

		var result TokenResponse
		json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()

		if resp.StatusCode == 202 {
			continue // still pending
		}
		if resp.StatusCode == 200 && result.Token != "" {
			return &result, nil
		}
		if result.Error != "" {
			return nil, fmt.Errorf("auth failed: %s", result.Error)
		}
	}
}
