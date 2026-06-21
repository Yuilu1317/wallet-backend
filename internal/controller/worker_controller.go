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
	runners map[string]WorkerRunner
}

func NewWorkerController(
	rootCtx context.Context,
	runners map[string]WorkerRunner,
) (*WorkerController, error) {
	if rootCtx == nil {
		return nil, fmt.Errorf("root context is nil")
	}
	if len(runners) == 0 {
		return nil, fmt.Errorf("runners is empty")
	}
	copied := make(map[string]WorkerRunner, len(runners))
	for name, runner := range runners {
		if name == "" {
			return nil, fmt.Errorf("worker name is empty")
		}
		if runner == nil {
			return nil, fmt.Errorf("runner %s is nil", name)
		}
		copied[name] = runner
	}

	return &WorkerController{
		rootCtx: rootCtx,
		runners: copied,
	}, nil
}

func (c *WorkerController) getRunner(ctx *gin.Context) (WorkerRunner, string, bool) {
	name := ctx.Param("name")

	runner, ok := c.runners[name]
	if !ok {
		ctx.JSON(http.StatusNotFound, gin.H{
			"error":  "worker not found",
			"worker": name,
		})
		return nil, name, false
	}

	return runner, name, true
}

func (c *WorkerController) RunOnce(ctx *gin.Context) {
	runner, name, ok := c.getRunner(ctx)
	if !ok {
		return
	}

	if err := runner.RunOnce(ctx.Request.Context()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error":  err.Error(),
			"worker": name,
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"status": "completed",
		"worker": name,
	})
}

func (c *WorkerController) Start(ctx *gin.Context) {
	runner, name, ok := c.getRunner(ctx)
	if !ok {
		return
	}

	if err := runner.Start(c.rootCtx); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error":  err.Error(),
			"worker": name,
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"status": "started",
		"worker": name,
	})
}

func (c *WorkerController) Stop(ctx *gin.Context) {
	runner, name, ok := c.getRunner(ctx)
	if !ok {
		return
	}

	if err := runner.Stop(); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error":  err.Error(),
			"worker": name,
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"status": "stopping",
		"worker": name,
	})
}

func (c *WorkerController) Status(ctx *gin.Context) {
	result := make(map[string]worker.RunnerStatus, len(c.runners))
	for name, runner := range c.runners {
		result[name] = runner.Status()
	}

	ctx.JSON(http.StatusOK, gin.H{
		"workers": result,
	})
}
