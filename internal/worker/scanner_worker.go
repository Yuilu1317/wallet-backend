package worker

import (
	"context"
	"fmt"
)

type NativeETHDepositScanner interface {
	ScanOnce(ctx context.Context) error
}

type ScannerWorker struct {
	nativeETHDepositScanner NativeETHDepositScanner
}

func NewNativeETHDepositScannerWorker(nativeETHDepositScanner NativeETHDepositScanner) (*ScannerWorker, error) {
	if nativeETHDepositScanner == nil {
		return nil, fmt.Errorf("native eth deposit scanner is required")
	}

	return &ScannerWorker{nativeETHDepositScanner: nativeETHDepositScanner}, nil
}

func (w *ScannerWorker) RunOnce(ctx context.Context) error {
	if err := w.nativeETHDepositScanner.ScanOnce(ctx); err != nil {
		return fmt.Errorf("run native eth deposit scanner once: %w", err)
	}
	return nil
}
