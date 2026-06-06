package explorer

type ListCompletedBlocksRequest struct {
	ChainID   int64
	FromBlock int64
	Limit     int
}

type ListCompletedBlocksResponse struct {
	ChainID int64
	Blocks  []CompletedBlock
}

type CompletedBlock struct {
	Number       int64
	Hash         string
	ParentHash   string
	Transactions []CompletedTransaction
}

type CompletedTransaction struct {
	TxHash        string
	FromAddress   string
	ToAddress     string
	AmountWei     string
	ReceiptStatus int16
}
