package explorer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type HTTPClient struct {
	baseURL string
	client  *http.Client
}

func NewHTTPClient(baseURL string, timeout time.Duration) (*HTTPClient, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("explorer base_url is required")
	}
	parsedBaseURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse explorer base_url: %w", err)
	}
	if parsedBaseURL.Scheme == "" || parsedBaseURL.Host == "" {
		return nil, fmt.Errorf("explorer base_url must include scheme and host")
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("explorer timeout must be positive")
	}
	return &HTTPClient{baseURL: baseURL, client: &http.Client{Timeout: timeout}}, nil
}

const completedBlocksPath = "/internal/wallet/completed-blocks"

func (c *HTTPClient) ListCompletedBlocks(
	ctx context.Context,
	req ListCompletedBlocksRequest,
) (*ListCompletedBlocksResponse, error) {
	if err := validateListCompletedBlocksRequest(req); err != nil {
		return nil, fmt.Errorf("validate completed blocks request: %w", err)
	}

	endpoint, err := c.buildCompletedBlocksURL(req)
	if err != nil {
		return nil, fmt.Errorf("build completed blocks url: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create completed blocks request: %w", err)
	}

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request completed blocks: chain_id=%d from_block=%d limit=%d: %w",
			req.ChainID,
			req.FromBlock,
			req.Limit,
			err)
	}
	defer resp.Body.Close()

	if err := checkCompletedBlocksResponseStatus(resp); err != nil {
		return nil, fmt.Errorf("check completed blocks status: %w", err)
	}

	var out ListCompletedBlocksResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode completed blocks response: %w", err)
	}

	if out.ChainID != req.ChainID {
		return nil, fmt.Errorf("unexpected response chain_id: got=%d expected=%d", out.ChainID, req.ChainID)
	}

	return &out, nil
}

func validateListCompletedBlocksRequest(req ListCompletedBlocksRequest) error {
	if req.ChainID <= 0 {
		return fmt.Errorf("chain_id must be positive")
	}

	if req.FromBlock < 0 {
		return fmt.Errorf("from_block must be non-negative")
	}

	if req.Limit <= 0 {
		return fmt.Errorf("limit must be positive")
	}

	return nil
}

func (c *HTTPClient) buildCompletedBlocksURL(req ListCompletedBlocksRequest) (string, error) {
	endpoint, err := url.Parse(c.baseURL + completedBlocksPath)
	if err != nil {
		return "", fmt.Errorf("parse completed blocks url: %w", err)
	}

	query := endpoint.Query()
	query.Set("chain_id", strconv.FormatInt(req.ChainID, 10))
	query.Set("from_block", strconv.FormatInt(req.FromBlock, 10))
	query.Set("limit", strconv.Itoa(req.Limit))
	endpoint.RawQuery = query.Encode()

	return endpoint.String(), nil
}

func checkCompletedBlocksResponseStatus(resp *http.Response) error {
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		return nil
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf(
		"request completed blocks failed: status_code=%d body=%s",
		resp.StatusCode,
		strings.TrimSpace(string(body)),
	)
}
