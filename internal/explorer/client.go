package explorer

import "context"

// Client defines the wallet-facing contract for reading completed chain data
// from the block explorer service.
type Client interface {
	ListCompletedBlocks(
		ctx context.Context,
		req ListCompletedBlocksRequest,
	) (*ListCompletedBlocksResponse, error)
}
