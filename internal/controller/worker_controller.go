package controller

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Worker interface {
	RunOnce(ctx context.Context) error
}

type WorkerController struct {
	nativeETHDepositScannerWorker Worker
	nativeETHDepositCreditWorker  Worker
}

func NewWorkerController(
	nativeETHDepositScannerWorker Worker,
	nativeETHDepositCreditWorker Worker,
) (*WorkerController, error) {
	if nativeETHDepositScannerWorker == nil {
		return nil, fmt.Errorf("native eth deposit scanner worker is required")
	}
	if nativeETHDepositCreditWorker == nil {
		return nil, fmt.Errorf("native eth deposit credit worker is required")
	}
	return &WorkerController{
		nativeETHDepositScannerWorker: nativeETHDepositScannerWorker,
		nativeETHDepositCreditWorker:  nativeETHDepositCreditWorker,
	}, nil
}

func (c *WorkerController) RunNativeETHDepositScannerOnce(ctx *gin.Context) {
	if err := c.nativeETHDepositScannerWorker.RunOnce(ctx.Request.Context()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"status": "completed",
		"worker": "native_eth_deposit_scanner",
	})
}

func (c *WorkerController) RunNativeETHDepositCreditOnce(ctx *gin.Context) {
	if err := c.nativeETHDepositCreditWorker.RunOnce(ctx.Request.Context()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"status": "completed",
		"worker": "native_eth_deposit_credit",
	})
}
