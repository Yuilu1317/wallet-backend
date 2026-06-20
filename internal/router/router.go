package router

import (
	"github.com/Yuilu1317/wallet-backend/internal/controller"
	"github.com/gin-gonic/gin"
)

func RegisterWorkerRoutes(r *gin.Engine, controller *controller.WorkerController) {
	group := r.Group("/admin/workers")

	group.POST("/native-eth-deposit-scanner/run-once", controller.RunNativeETHDepositScannerOnce)
	group.POST("/native-eth-deposit-credit/run-once", controller.RunNativeETHDepositCreditOnce)
}
