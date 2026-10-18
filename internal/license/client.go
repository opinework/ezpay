package license

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// httpClient handles communication with the license server
type httpClient struct {
	baseURL    string
	httpClient *http.Client
}

func newHTTPClient(baseURL string) *httpClient {
	return &httpClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Verify sends a verification request to the license server
func (c *httpClient) Verify(req *VerifyRequest) (*VerifyResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := c.baseURL + "/api/v1/license/server/verify"
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var verifyResp VerifyResponse
	if err := json.Unmarshal(respBody, &verifyResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &verifyResp, nil
}

// Heartbeat sends a heartbeat to the license server
func (c *httpClient) Heartbeat(req *HeartbeatRequest) (*HeartbeatResponse, error) {
	return c.HeartbeatWithUsage(req, UsageReport{})
}

// HeartbeatWithUsage extends Heartbeat with a usage payload — replaces the
// plain Heartbeat at every call site. Sends a heartbeat with the given
// usage report attached so the license server can reconcile contractual
// limits with actual consumption.
func (c *httpClient) HeartbeatWithUsage(req *HeartbeatRequest, usage UsageReport) (*HeartbeatResponse, error) {
	payload := struct {
		*HeartbeatRequest
		Usage *UsageReport `json:"usage,omitempty"`
	}{
		HeartbeatRequest: req,
	}
	if usage.ReportedAt > 0 {
		payload.Usage = &usage
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := c.baseURL + "/api/v1/license/server/heartbeat"
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var heartbeatResp HeartbeatResponse
	if err := json.Unmarshal(respBody, &heartbeatResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &heartbeatResp, nil
}
