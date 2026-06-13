package scanner

import "fmt"

type ScanRange struct {
	FromBlock                  int64
	Limit                      int
	ConfirmedTargetBlockNumber int64
}

func PlanScanRange(
	latestCompletedBlock int64,
	confirmationDepth int64,
	lastScannedBlock int64,
	batchLimit int,
) (*ScanRange, bool, error) {
	if latestCompletedBlock < 0 {
		return nil, false, fmt.Errorf("latest_completed_block must be non-negative")
	}
	if confirmationDepth < 0 {
		return nil, false, fmt.Errorf("confirmation_depth must be non-negative")
	}
	if lastScannedBlock < -1 {
		return nil, false, fmt.Errorf("last_scanned_block must be greater than or equal to -1")
	}
	if batchLimit <= 0 {
		return nil, false, fmt.Errorf("batch_limit must be positive")
	}

	confirmedTargetBlockNumber := latestCompletedBlock - confirmationDepth
	if confirmedTargetBlockNumber < 0 {
		return nil, false, nil
	}

	fromBlock := lastScannedBlock + 1
	if fromBlock > confirmedTargetBlockNumber {
		return nil, false, nil
	}

	remaining := confirmedTargetBlockNumber - fromBlock + 1

	limit := int64(batchLimit)
	if remaining < limit {
		limit = remaining
	}

	return &ScanRange{
		FromBlock:                  fromBlock,
		Limit:                      int(limit),
		ConfirmedTargetBlockNumber: confirmedTargetBlockNumber,
	}, true, nil
}
