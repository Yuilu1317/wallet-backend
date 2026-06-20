package controller

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Yuilu1317/wallet-backend/internal/worker"
	"github.com/gin-gonic/gin"
)

type WorkerRunner interface {
	RunOnce(ctx context.Context) error
	Start(rootCtx context.Context) error
	Stop() error
	Status() worker.RunnerStatus
}

type WorkerController struct {
	rootCtx context.Context

	nativeETHDepositScannerRunner WorkerRunner
	nativeETHDepositCreditRunner  WorkerRunner
}

func NewWorkerController(
	rootCtx context.Context,
	nativeETHDepositScannerRunner WorkerRunner,
	nativeETHDepositCreditRunner WorkerRunner,
) (*WorkerController, error) {
	if rootCtx == nil {
		return nil, fmt.Errorf("root context is nil")
	}
	if nativeETHDepositScannerRunner == nil {
		return nil, fmt.Errorf("native eth deposit scanner runner is required")
	}
	if nativeETHDepositCreditRunner == nil {
		return nil, fmt.Errorf("native eth deposit credit runner is required")
	}
	return &WorkerController{
		rootCtx:                       rootCtx,
		nativeETHDepositScannerRunner: nativeETHDepositScannerRunner,
		nativeETHDepositCreditRunner:  nativeETHDepositCreditRunner,
	}, nil
}

func (c *WorkerController) RunNativeETHDepositScannerOnce(ctx *gin.Context) {
	if err := c.nativeETHDepositScannerRunner.RunOnce(ctx.Request.Context()); err != nil {
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

func (c *WorkerController) StartNativeETHDepositScanner(ctx *gin.Context) {
	if err := c.nativeETHDepositScannerRunner.Start(c.rootCtx); err != nil {
		ctx.JSON(http.StatusConflict, gin.H{
			"error": err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"status": "started",
		"worker": "native_eth_deposit_scanner",
	})
}

func (c *WorkerController) StopNativeETHDepositScanner(ctx *gin.Context) {
	if err := c.nativeETHDepositScannerRunner.Stop(); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"status": "stopping",
		"worker": "native_eth_deposit_scanner",
	})
}

func (c *WorkerController) GetNativeETHDepositScannerStatus(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, c.nativeETHDepositScannerRunner.Status())
}

func (c *WorkerController) RunNativeETHDepositCreditOnce(ctx *gin.Context) {
	if err := c.nativeETHDepositCreditRunner.RunOnce(ctx.Request.Context()); err != nil {
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

func (c *WorkerController) StartNativeETHDepositCredit(ctx *gin.Context) {
	if err := c.nativeETHDepositCreditRunner.Start(c.rootCtx); err != nil {
		ctx.JSON(http.StatusConflict, gin.H{
			"error": err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"status": "started",
		"worker": "native_eth_deposit_credit",
	})
}

func (c *WorkerController) StopNativeETHDepositCredit(ctx *gin.Context) {
	if err := c.nativeETHDepositCreditRunner.Stop(); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"status": "stopping",
		"worker": "native_eth_deposit_credit",
	})
}

func (c *WorkerController) GetNativeETHDepositCreditStatus(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, c.nativeETHDepositCreditRunner.Status())
}
