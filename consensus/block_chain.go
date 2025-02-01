package consensus

import (
	sm "github.com/xufeisofly/hotstuff/state"
	"github.com/xufeisofly/hotstuff/types"
)

// BlockStatus is status of a block
type BlockStatus int

const (
	BlockStatus_Unknown   BlockStatus = iota // unknown status
	BlockStatus_Proposed                     // block has been proposed
	BlockStatus_Locked                       // block has been locked
	BlockStatus_Committed                    // block has been committed
)

type BlockChain interface {
	// Store a block to the chain
	Store(block *types.Block) error
	// If has block
	Has(blockhash Hash) bool
	// If block and target share the same branch
	Extends(block, target *types.Block) bool
	// Prune from the latest prune view to target block
	PruneTo(targetHash Hash, forkedBlocks []*types.Block) error

	// Get all blocks
	GetAll() []*types.Block

	// Get max view from the chain
	GetMaxView() types.View
}

type wrappedBlock struct {
	block    *types.Block
	status   BlockStatus
	children []*types.Block
	qc       *QuorumCert
}

type blockChain struct {
	startBlock *types.Block // the starting block of the chain
	pruneView  types.View   // latest view that has been pruned

	blocksAtView  map[types.View]*types.Block // mapping from view to block
	wrappedBlocks map[HashStr]*wrappedBlock   // mapping from block hash to wrapped block

	latestCommittedBlock *types.Block
	latestLockedBlock    *types.Block

	blockStore sm.BlockStore
}

func NewBlockChain(blockStore sm.BlockStore) BlockChain {
	return &blockChain{
		blockStore: blockStore,
	}
}
