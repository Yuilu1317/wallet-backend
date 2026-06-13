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

const SyncStatusPath = "/internal/wallet/sync-status"

func (c *HTTPClient) GetSyncStatus(
	ctx context.Context,
	chainID int64,
) (*SyncStatusResponse, error) {
	if chainID <= 0 {
		return nil, fmt.Errorf("chain_id must be positive")
	}

	endpoint, err := c.buildSyncStatusURL(chainID)
	if err != nil {
		return nil, fmt.Errorf("build sync status block url: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create sync status block request: %w", err)
	}

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request sync status block: chain_id=%d: %w", chainID, err)
	}
	defer resp.Body.Close()

	if err := checkResponseStatus(resp); err != nil {
		return nil, fmt.Errorf("check sync status block status: %w", err)
	}

	var out SyncStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode sync status block response: %w", err)
	}

	if out.ChainID != chainID {
		return nil, fmt.Errorf("unexpected response chain_id: got=%d expected=%d", out.ChainID, chainID)
	}

	if out.LatestCompletedBlock == nil {
		return nil, fmt.Errorf("latest_completed_block is nil")
	}

	if out.LatestCompletedBlock.Number < 0 {
		return nil, fmt.Errorf("latest_completed_block.number must be non-negative")
	}

	if strings.TrimSpace(out.LatestCompletedBlock.Hash) == "" {
		return nil, fmt.Errorf("latest_completed_block.hash is required")
	}
	return &out, nil
}

func (c *HTTPClient) buildSyncStatusURL(chainID int64) (string, error) {
	endpoint, err := url.Parse(c.baseURL + SyncStatusPath)
	if err != nil {
		return "", fmt.Errorf("parse sync status block url: %w", err)
	}

	query := endpoint.Query()
	query.Set("chain_id", strconv.FormatInt(chainID, 10))
	endpoint.RawQuery = query.Encode()

	return endpoint.String(), nil
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

	if err := checkResponseStatus(resp); err != nil {
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

func checkResponseStatus(resp *http.Response) error {
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		return nil
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf(
		"request failed: status_code=%d body=%s",
		resp.StatusCode,
		strings.TrimSpace(string(body)),
	)
}
