package explorer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewHTTPClient(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		timeout time.Duration
		wantErr string
	}{
		{
			name:    "empty base url",
			baseURL: "",
			timeout: time.Second,
			wantErr: "explorer base_url is required",
		},
		{
			name:    "base url without scheme",
			baseURL: "localhost:8080",
			timeout: time.Second,
			wantErr: "explorer base_url must include scheme and host",
		},
		{
			name:    "zero timeout",
			baseURL: "http://localhost:8080",
			timeout: 0,
			wantErr: "explorer timeout must be positive",
		},
		{
			name:    "negative timeout",
			baseURL: "http://localhost:8080",
			timeout: -time.Second,
			wantErr: "explorer timeout must be positive",
		},
		{
			name:    "valid config",
			baseURL: "http://localhost:8080/",
			timeout: 5 * time.Second,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			client, err := NewHTTPClient(tt.baseURL, tt.timeout)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("expected error, got nil")
				}

				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error to contain %q, got %q", tt.wantErr, err.Error())
				}

				if client != nil {
					t.Fatalf("expected nil client, got %+v", client)
				}

				return
			}

			if err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}

			if client == nil {
				t.Fatal("expected client, got nil")
			}

			if client.baseURL != "http://localhost:8080" {
				t.Fatalf("expected trimmed baseURL, got %q", client.baseURL)
			}

			if client.client == nil {
				t.Fatal("expected http client, got nil")
			}

			if client.client.Timeout != tt.timeout {
				t.Fatalf("expected timeout %v, got %v", tt.timeout, client.client.Timeout)
			}
		})
	}
}

func TestHTTPClient_ListCompletedBlocks_ValidatesRequest(t *testing.T) {
	client, err := NewHTTPClient("http://localhost:8080", time.Second)
	if err != nil {
		t.Fatalf("new http client: %v", err)
	}

	tests := []struct {
		name    string
		req     ListCompletedBlocksRequest
		wantErr string
	}{
		{
			name: "invalid chain id",
			req: ListCompletedBlocksRequest{
				ChainID:   0,
				FromBlock: 100,
				Limit:     10,
			},
			wantErr: "chain_id must be positive",
		},
		{
			name: "negative from block",
			req: ListCompletedBlocksRequest{
				ChainID:   11155111,
				FromBlock: -1,
				Limit:     10,
			},
			wantErr: "from_block must be non-negative",
		},
		{
			name: "invalid limit",
			req: ListCompletedBlocksRequest{
				ChainID:   11155111,
				FromBlock: 100,
				Limit:     0,
			},
			wantErr: "limit must be positive",
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			resp, err := client.ListCompletedBlocks(context.Background(), tt.req)
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			if resp != nil {
				t.Fatalf("expected nil response, got %+v", resp)
			}

			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error to contain %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestHTTPClient_ListCompletedBlocks_Success(t *testing.T) {
	var called bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true

		if r.Method != http.MethodGet {
			t.Errorf("expected method GET, got %s", r.Method)
		}

		if r.URL.Path != completedBlocksPath {
			t.Errorf("expected path %s, got %s", completedBlocksPath, r.URL.Path)
		}

		query := r.URL.Query()
		if got := query.Get("chain_id"); got != "11155111" {
			t.Errorf("expected chain_id=11155111, got %q", got)
		}

		if got := query.Get("from_block"); got != "11008017" {
			t.Errorf("expected from_block=11008017, got %q", got)
		}

		if got := query.Get("limit"); got != "10" {
			t.Errorf("expected limit=10, got %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"chain_id": 11155111,
			"blocks": [
				{
					"number": 11008017,
					"hash": "0xblock",
					"parent_hash": "0xparent",
					"transactions": [
						{
							"tx_hash": "0xtx",
							"from_address": "0xfrom",
							"to_address": "0xto",
							"amount_wei": "100",
							"receipt_status": 1
						}
					]
				}
			]
		}`))
	}))
	defer server.Close()

	client, err := NewHTTPClient(server.URL, time.Second)
	if err != nil {
		t.Fatalf("new http client: %v", err)
	}

	resp, err := client.ListCompletedBlocks(context.Background(), ListCompletedBlocksRequest{
		ChainID:   11155111,
		FromBlock: 11008017,
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("list completed blocks: %v", err)
	}

	if !called {
		t.Fatal("expected server to be called")
	}

	if resp == nil {
		t.Fatal("expected response, got nil")
	}

	if resp.ChainID != 11155111 {
		t.Fatalf("expected chain id 11155111, got %d", resp.ChainID)
	}

	if len(resp.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(resp.Blocks))
	}

	block := resp.Blocks[0]
	if block.Number != 11008017 {
		t.Fatalf("expected block number 11008017, got %d", block.Number)
	}

	if block.Hash != "0xblock" {
		t.Fatalf("expected block hash 0xblock, got %q", block.Hash)
	}

	if block.ParentHash != "0xparent" {
		t.Fatalf("expected parent hash 0xparent, got %q", block.ParentHash)
	}

	if len(block.Transactions) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(block.Transactions))
	}

	tx := block.Transactions[0]
	if tx.TxHash != "0xtx" {
		t.Fatalf("expected tx hash 0xtx, got %q", tx.TxHash)
	}

	if tx.FromAddress != "0xfrom" {
		t.Fatalf("expected from address 0xfrom, got %q", tx.FromAddress)
	}

	if tx.ToAddress != "0xto" {
		t.Fatalf("expected to address 0xto, got %q", tx.ToAddress)
	}

	if tx.AmountWei != "100" {
		t.Fatalf("expected amount wei 100, got %q", tx.AmountWei)
	}

	if tx.ReceiptStatus != 1 {
		t.Fatalf("expected receipt status 1, got %d", tx.ReceiptStatus)
	}
}

func TestHTTPClient_ListCompletedBlocks_Non2xxReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer server.Close()

	client, err := NewHTTPClient(server.URL, time.Second)
	if err != nil {
		t.Fatalf("new http client: %v", err)
	}

	resp, err := client.ListCompletedBlocks(context.Background(), ListCompletedBlocksRequest{
		ChainID:   11155111,
		FromBlock: 11008017,
		Limit:     10,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if resp != nil {
		t.Fatalf("expected nil response, got %+v", resp)
	}

	if !strings.Contains(err.Error(), "status_code=400") {
		t.Fatalf("expected status code error, got %q", err.Error())
	}

	if !strings.Contains(err.Error(), "bad request") {
		t.Fatalf("expected response body in error, got %q", err.Error())
	}
}

func TestHTTPClient_ListCompletedBlocks_InvalidJSONReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer server.Close()

	client, err := NewHTTPClient(server.URL, time.Second)
	if err != nil {
		t.Fatalf("new http client: %v", err)
	}

	resp, err := client.ListCompletedBlocks(context.Background(), ListCompletedBlocksRequest{
		ChainID:   11155111,
		FromBlock: 11008017,
		Limit:     10,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if resp != nil {
		t.Fatalf("expected nil response, got %+v", resp)
	}

	if !strings.Contains(err.Error(), "decode completed blocks response") {
		t.Fatalf("expected decode error, got %q", err.Error())
	}
}

func TestHTTPClient_ListCompletedBlocks_UnexpectedChainIDReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"chain_id": 1,
			"blocks": []
		}`))
	}))
	defer server.Close()

	client, err := NewHTTPClient(server.URL, time.Second)
	if err != nil {
		t.Fatalf("new http client: %v", err)
	}

	resp, err := client.ListCompletedBlocks(context.Background(), ListCompletedBlocksRequest{
		ChainID:   11155111,
		FromBlock: 11008017,
		Limit:     10,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if resp != nil {
		t.Fatalf("expected nil response, got %+v", resp)
	}

	if !strings.Contains(err.Error(), "unexpected response chain_id") {
		t.Fatalf("expected unexpected chain_id error, got %q", err.Error())
	}
}

func TestHTTPClient_GetSyncStatus_ValidatesChainID(t *testing.T) {
	client, err := NewHTTPClient("http://localhost:8080", time.Second)
	if err != nil {
		t.Fatalf("new http client: %v", err)
	}

	tests := []struct {
		name    string
		chainID int64
		wantErr string
	}{
		{
			name:    "zero chain id",
			chainID: 0,
			wantErr: "chain_id must be positive",
		},
		{
			name:    "negative chain id",
			chainID: -1,
			wantErr: "chain_id must be positive",
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			resp, err := client.GetSyncStatus(context.Background(), tt.chainID)
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			if resp != nil {
				t.Fatalf("expected nil response, got %+v", resp)
			}

			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error to contain %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestHTTPClient_GetSyncStatus_Success(t *testing.T) {
	var called bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true

		if r.Method != http.MethodGet {
			t.Errorf("expected method GET, got %s", r.Method)
		}

		if r.URL.Path != SyncStatusPath {
			t.Errorf("expected path %s, got %s", SyncStatusPath, r.URL.Path)
		}

		query := r.URL.Query()
		if got := query.Get("chain_id"); got != "11155111" {
			t.Errorf("expected chain_id=11155111, got %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"chain_id": 11155111,
			"sync_target": "safe",
			"latest_completed_block": {
				"number": 11008017,
				"hash": "0xblock"
			}
		}`))
	}))
	defer server.Close()

	client, err := NewHTTPClient(server.URL, time.Second)
	if err != nil {
		t.Fatalf("new http client: %v", err)
	}

	resp, err := client.GetSyncStatus(context.Background(), 11155111)
	if err != nil {
		t.Fatalf("get sync status: %v", err)
	}

	if !called {
		t.Fatal("expected server to be called")
	}

	if resp == nil {
		t.Fatal("expected response, got nil")
	}

	if resp.ChainID != 11155111 {
		t.Fatalf("expected chain id 11155111, got %d", resp.ChainID)
	}

	if resp.SyncTarget != "safe" {
		t.Fatalf("expected sync target safe, got %q", resp.SyncTarget)
	}

	if resp.LatestCompletedBlock == nil {
		t.Fatal("expected latest completed block, got nil")
	}

	if resp.LatestCompletedBlock.Number != 11008017 {
		t.Fatalf("expected latest completed block number 11008017, got %d", resp.LatestCompletedBlock.Number)
	}

	if resp.LatestCompletedBlock.Hash != "0xblock" {
		t.Fatalf("expected latest completed block hash 0xblock, got %q", resp.LatestCompletedBlock.Hash)
	}
}

func TestHTTPClient_GetSyncStatus_Non2xxReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"explorer unavailable"}`))
	}))
	defer server.Close()

	client, err := NewHTTPClient(server.URL, time.Second)
	if err != nil {
		t.Fatalf("new http client: %v", err)
	}

	resp, err := client.GetSyncStatus(context.Background(), 11155111)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if resp != nil {
		t.Fatalf("expected nil response, got %+v", resp)
	}

	if !strings.Contains(err.Error(), "status_code=503") {
		t.Fatalf("expected status code error, got %q", err.Error())
	}

	if !strings.Contains(err.Error(), "explorer unavailable") {
		t.Fatalf("expected response body in error, got %q", err.Error())
	}
}

func TestHTTPClient_GetSyncStatus_InvalidJSONReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer server.Close()

	client, err := NewHTTPClient(server.URL, time.Second)
	if err != nil {
		t.Fatalf("new http client: %v", err)
	}

	resp, err := client.GetSyncStatus(context.Background(), 11155111)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if resp != nil {
		t.Fatalf("expected nil response, got %+v", resp)
	}

	if !strings.Contains(err.Error(), "decode sync status") {
		t.Fatalf("expected decode error, got %q", err.Error())
	}
}

func TestHTTPClient_GetSyncStatus_UnexpectedChainIDReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"chain_id": 1,
			"sync_target": "safe",
			"latest_completed_block": {
				"number": 11008017,
				"hash": "0xblock"
			}
		}`))
	}))
	defer server.Close()

	client, err := NewHTTPClient(server.URL, time.Second)
	if err != nil {
		t.Fatalf("new http client: %v", err)
	}

	resp, err := client.GetSyncStatus(context.Background(), 11155111)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if resp != nil {
		t.Fatalf("expected nil response, got %+v", resp)
	}

	if !strings.Contains(err.Error(), "unexpected response chain_id") {
		t.Fatalf("expected unexpected chain_id error, got %q", err.Error())
	}
}

func TestHTTPClient_GetSyncStatus_NilLatestCompletedBlockReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"chain_id": 11155111,
			"sync_target": "safe",
			"latest_completed_block": null
		}`))
	}))
	defer server.Close()

	client, err := NewHTTPClient(server.URL, time.Second)
	if err != nil {
		t.Fatalf("new http client: %v", err)
	}

	resp, err := client.GetSyncStatus(context.Background(), 11155111)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if resp != nil {
		t.Fatalf("expected nil response, got %+v", resp)
	}

	if !strings.Contains(err.Error(), "latest_completed_block is nil") {
		t.Fatalf("expected nil latest_completed_block error, got %q", err.Error())
	}
}

func TestHTTPClient_GetSyncStatus_NegativeLatestCompletedBlockNumberReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"chain_id": 11155111,
			"sync_target": "safe",
			"latest_completed_block": {
				"number": -1,
				"hash": "0xblock"
			}
		}`))
	}))
	defer server.Close()

	client, err := NewHTTPClient(server.URL, time.Second)
	if err != nil {
		t.Fatalf("new http client: %v", err)
	}

	resp, err := client.GetSyncStatus(context.Background(), 11155111)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if resp != nil {
		t.Fatalf("expected nil response, got %+v", resp)
	}

	if !strings.Contains(err.Error(), "latest_completed_block.number must be non-negative") {
		t.Fatalf("expected negative block number error, got %q", err.Error())
	}
}

func TestHTTPClient_GetSyncStatus_EmptyLatestCompletedBlockHashReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"chain_id": 11155111,
			"sync_target": "safe",
			"latest_completed_block": {
				"number": 11008017,
				"hash": "   "
			}
		}`))
	}))
	defer server.Close()

	client, err := NewHTTPClient(server.URL, time.Second)
	if err != nil {
		t.Fatalf("new http client: %v", err)
	}

	resp, err := client.GetSyncStatus(context.Background(), 11155111)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if resp != nil {
		t.Fatalf("expected nil response, got %+v", resp)
	}

	if !strings.Contains(err.Error(), "latest_completed_block.hash is required") {
		t.Fatalf("expected empty hash error, got %q", err.Error())
	}
}
