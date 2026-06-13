package explorer

type SyncStatusRequest struct {
	ChainID int64
}

type SyncStatusResponse struct {
	ChainID              int64                  `json:"chain_id"`
	SyncTarget           string                 `json:"sync_target"`
	LatestCompletedBlock *CompletedBlockSummary `json:"latest_completed_block"`
}

type CompletedBlockSummary struct {
	Number int64  `json:"number"`
	Hash   string `json:"hash"`
}

type ListCompletedBlocksRequest struct {
	ChainID   int64
	FromBlock int64
	Limit     int
}

type ListCompletedBlocksResponse struct {
	ChainID int64            `json:"chain_id"`
	Blocks  []CompletedBlock `json:"blocks"`
}

type CompletedBlock struct {
	Number       int64                  `json:"number"`
	Hash         string                 `json:"hash"`
	ParentHash   string                 `json:"parent_hash"`
	Transactions []CompletedTransaction `json:"transactions"`
}

type CompletedTransaction struct {
	TxHash        string `json:"tx_hash"`
	FromAddress   string `json:"from_address"`
	ToAddress     string `json:"to_address"`
	AmountWei     string `json:"amount_wei"`
	ReceiptStatus int16  `json:"receipt_status"`
}
