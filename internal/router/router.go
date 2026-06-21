package router

import (
	"github.com/Yuilu1317/wallet-backend/internal/controller"
	"github.com/gin-gonic/gin"
)

func RegisterWorkerRoutes(engine *gin.Engine, workerController *controller.WorkerController) {
	admin := engine.Group("/admin")

	workers := admin.Group("/workers")
	{
		workers.POST("/:name/run-once", workerController.RunOnce)
		workers.POST("/:name/start", workerController.Start)
		workers.POST("/:name/stop", workerController.Stop)

		workers.GET("/status", workerController.Status)
	}
}
