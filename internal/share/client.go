package share

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// ErrUnauthorized is returned when the API returns 401.
var ErrUnauthorized = errors.New("unauthorized — try: thinkt share login")

// Client is an HTTP client for the wethinkt Share API.
type Client struct {
	Endpoint   string
	Token      string
	HTTPClient *http.Client
}

func NewClient(endpoint, token string) *Client {
	return &Client{
		Endpoint:   endpoint,
		Token:      token,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func NewClientFromCreds(creds *Credentials) *Client {
	return NewClient(creds.Endpoint, creds.Token)
}

func (c *Client) doJSON(method, path string, target any) error {
	req, err := http.NewRequest(method, c.Endpoint+path, nil)
	if err != nil {
		return err
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return ErrUnauthorized
	}
	if resp.StatusCode >= 400 {
		var errResp ErrorResponse
		json.NewDecoder(resp.Body).Decode(&errResp)
		if errResp.Error != "" {
			return fmt.Errorf("%s %s: %s (%d)", method, path, errResp.Error, resp.StatusCode)
		}
		return fmt.Errorf("%s %s: %d", method, path, resp.StatusCode)
	}

	if target != nil {
		return json.NewDecoder(resp.Body).Decode(target)
	}
	return nil
}

func (c *Client) ListTraces() ([]Trace, error) {
	var resp TraceList
	if err := c.doJSON(http.MethodGet, "/api/traces", &resp); err != nil {
		return nil, err
	}
	return resp.Traces, nil
}

func (c *Client) GetTrace(slug string) (*Trace, error) {
	var t Trace
	if err := c.doJSON(http.MethodGet, "/api/traces/"+url.PathEscape(slug), &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (c *Client) DeleteTrace(slug string) error {
	return c.doJSON(http.MethodDelete, "/api/traces/"+url.PathEscape(slug), nil)
}

func (c *Client) Explore(sort, tag string, page int) (*ExploreResponse, error) {
	params := url.Values{}
	if sort != "" {
		params.Set("sort", sort)
	}
	if tag != "" {
		params.Set("tag", tag)
	}
	if page > 1 {
		params.Set("page", strconv.Itoa(page))
	}
	path := "/api/explore"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	var resp ExploreResponse
	if err := c.doJSON(http.MethodGet, path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) ExploreTags() ([]TagCount, error) {
	var resp TagCloud
	if err := c.doJSON(http.MethodGet, "/api/explore/tags", &resp); err != nil {
		return nil, err
	}
	return resp.Tags, nil
}

func (c *Client) GetProfile() (*Profile, error) {
	var p Profile
	if err := c.doJSON(http.MethodGet, "/api/profile", &p); err != nil {
		return nil, err
	}
	return &p, nil
}
