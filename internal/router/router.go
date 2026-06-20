package router

import (
	"github.com/Yuilu1317/wallet-backend/internal/controller"
	"github.com/gin-gonic/gin"
)

func RegisterWorkerRoutes(r *gin.Engine, workerController *controller.WorkerController) {
	group := r.Group("/admin/workers")

	// native ETH deposit scanner
	group.POST(
		"/native-eth-deposit-scanner/run-once",
		workerController.RunNativeETHDepositScannerOnce,
	)
	group.POST(
		"/native-eth-deposit-scanner/start",
		workerController.StartNativeETHDepositScanner,
	)
	group.POST(
		"/native-eth-deposit-scanner/stop",
		workerController.StopNativeETHDepositScanner,
	)
	group.GET(
		"/native-eth-deposit-scanner/status",
		workerController.GetNativeETHDepositScannerStatus,
	)

	// native ETH deposit credit
	group.POST(
		"/native-eth-deposit-credit/run-once",
		workerController.RunNativeETHDepositCreditOnce,
	)
	group.POST(
		"/native-eth-deposit-credit/start",
		workerController.StartNativeETHDepositCredit,
	)
	group.POST(
		"/native-eth-deposit-credit/stop",
		workerController.StopNativeETHDepositCredit,
	)
	group.GET(
		"/native-eth-deposit-credit/status",
		workerController.GetNativeETHDepositCreditStatus,
	)
}
