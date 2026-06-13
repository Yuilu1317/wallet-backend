package scanner

import (
	"strings"
	"testing"
)

func TestPlanScanRange(t *testing.T) {
	tests := []struct {
		name                 string
		latestCompletedBlock int64
		confirmationDepth    int64
		lastScannedBlock     int64
		batchSize            int
		wantShouldScan       bool
		wantFromBlock        int64
		wantLimit            int
		wantConfirmedTarget  int64
		wantErr              string
	}{
		{
			name:                 "full batch when scanner is far behind confirmed target",
			latestCompletedBlock: 200,
			confirmationDepth:    12,
			lastScannedBlock:     99,
			batchSize:            10,
			wantShouldScan:       true,
			wantFromBlock:        100,
			wantLimit:            10,
			wantConfirmedTarget:  188,
		},
		{
			name:                 "limit is capped by confirmed target",
			latestCompletedBlock: 105,
			confirmationDepth:    3,
			lastScannedBlock:     99,
			batchSize:            10,
			wantShouldScan:       true,
			wantFromBlock:        100,
			wantLimit:            3,
			wantConfirmedTarget:  102,
		},
		{
			name:                 "single block remaining",
			latestCompletedBlock: 80,
			confirmationDepth:    12,
			lastScannedBlock:     67,
			batchSize:            10,
			wantShouldScan:       true,
			wantFromBlock:        68,
			wantLimit:            1,
			wantConfirmedTarget:  68,
		},
		{
			name:                 "no scan when cursor has reached confirmed target",
			latestCompletedBlock: 80,
			confirmationDepth:    12,
			lastScannedBlock:     68,
			batchSize:            10,
			wantShouldScan:       false,
		},
		{
			name:                 "no scan when confirmed target is negative",
			latestCompletedBlock: 5,
			confirmationDepth:    12,
			lastScannedBlock:     0,
			batchSize:            10,
			wantShouldScan:       false,
		},
		{
			name:                 "zero confirmation depth scans up to latest completed block",
			latestCompletedBlock: 100,
			confirmationDepth:    0,
			lastScannedBlock:     99,
			batchSize:            10,
			wantShouldScan:       true,
			wantFromBlock:        100,
			wantLimit:            1,
			wantConfirmedTarget:  100,
		},
		{
			name:                 "last scanned block can be minus one",
			latestCompletedBlock: 5,
			confirmationDepth:    0,
			lastScannedBlock:     -1,
			batchSize:            3,
			wantShouldScan:       true,
			wantFromBlock:        0,
			wantLimit:            3,
			wantConfirmedTarget:  5,
		},
		{
			name:                 "latest completed block must be non-negative",
			latestCompletedBlock: -1,
			confirmationDepth:    0,
			lastScannedBlock:     0,
			batchSize:            10,
			wantErr:              "latest_completed_block must be non-negative",
		},
		{
			name:                 "confirmation depth must be non-negative",
			latestCompletedBlock: 100,
			confirmationDepth:    -1,
			lastScannedBlock:     0,
			batchSize:            10,
			wantErr:              "confirmation_depth must be non-negative",
		},
		{
			name:                 "last scanned block must be greater than or equal to minus one",
			latestCompletedBlock: 100,
			confirmationDepth:    0,
			lastScannedBlock:     -2,
			batchSize:            10,
			wantErr:              "last_scanned_block must be greater than or equal to -1",
		},
		{
			name:                 "batch size must be positive",
			latestCompletedBlock: 100,
			confirmationDepth:    0,
			lastScannedBlock:     0,
			batchSize:            0,
			wantErr:              "batch_limit must be positive",
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			got, shouldScan, err := PlanScanRange(
				tt.latestCompletedBlock,
				tt.confirmationDepth,
				tt.lastScannedBlock,
				tt.batchSize,
			)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("expected error, got nil")
				}

				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error to contain %q, got %q", tt.wantErr, err.Error())
				}

				if got != nil {
					t.Fatalf("expected nil scan range, got %+v", got)
				}

				if shouldScan {
					t.Fatal("expected shouldScan=false when error occurs")
				}

				return
			}

			if err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}

			if shouldScan != tt.wantShouldScan {
				t.Fatalf("expected shouldScan=%v, got %v", tt.wantShouldScan, shouldScan)
			}

			if !tt.wantShouldScan {
				if got != nil {
					t.Fatalf("expected nil scan range, got %+v", got)
				}
				return
			}

			if got == nil {
				t.Fatal("expected scan range, got nil")
			}

			if got.FromBlock != tt.wantFromBlock {
				t.Fatalf("expected from block %d, got %d", tt.wantFromBlock, got.FromBlock)
			}

			if got.Limit != tt.wantLimit {
				t.Fatalf("expected limit %d, got %d", tt.wantLimit, got.Limit)
			}

			if got.ConfirmedTargetBlockNumber != tt.wantConfirmedTarget {
				t.Fatalf("expected confirmed target block %d, got %d", tt.wantConfirmedTarget, got.ConfirmedTargetBlockNumber)
			}
		})
	}
}
