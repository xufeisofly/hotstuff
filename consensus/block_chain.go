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
	// Store a block(committed) to database
	Store2Db(block *types.Block) error
	// Get a block by block hash
	Get(blockHash Hash) *types.Block
	// If has block
	Has(blockhash Hash) bool
	// If block and target share the same branch
	Extends(block, target *types.Block) bool
	// Prune from the latest prune view to target block
	PruneTo(targetHash Hash, forkedBlocks []*types.Block) error

	// Get all blocks
	GetAll() []*types.Block
	// Get all verified blocks
	GetAllVerified() []*types.Block
	// Get all blocks ordered by view
	GetOrderedAll() []*types.Block
	// Get all children blocks of a block
	GetRecursiveChildren(blockHash Hash) []*types.Block

	// Get max view from the chain
	GetMaxView() types.View
	// Latest committed block
	LatestCommittedBlock() *types.Block
	// Latest locked block
	LatestLockedBlock() *types.Block
	// Set latest committed block
	SetLatestCommittedBlock(block *types.Block)
	// Set latest locked block
	SetLatestLockedBlock(block *types.Block)

	// Get QC of a block
	GetQuorumCertOf(block *types.Block) *QuorumCert
	// Set QC for a block
	SetQuorumCertFor(block *types.Block, qc *QuorumCert)

	// Is the chain valid
	IsValid() bool
	// Number of blocks in chain
	Size() uint32
	String() string

	// Get a block referenced by qc
	QuorumCertRef(block *types.Block) *types.Block
	// Get a block referenced by parent hash
	ParentRef(block *types.Block) *types.Block
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

var _ BlockChain = (*blockChain)(nil)

func NewBlockChain(blockStore sm.BlockStore) BlockChain {
	return &blockChain{
		startBlock:           nil,
		pruneView:            types.ViewBeforeGenesis,
		blocksAtView:         make(map[types.View]*types.Block),
		wrappedBlocks:        make(map[HashStr]*wrappedBlock),
		latestCommittedBlock: nil,
		latestLockedBlock:    nil,
		blockStore:           blockStore,
	}
}

func (bc *blockChain) Store(block *types.Block) error {
	return nil
}

func (bc *blockChain) Store2Db(block *types.Block) error { return nil }

func (bc *blockChain) Get(blockHash Hash) *types.Block { return nil }

func (bc *blockChain) Has(blockhash Hash) bool { return false }

func (bc *blockChain) Extends(block, target *types.Block) bool { return false }

func (bc *blockChain) PruneTo(targetHash Hash, forkedBlocks []*types.Block) error { return nil }

func (bc *blockChain) GetAll() []*types.Block { return nil }

func (bc *blockChain) GetAllVerified() []*types.Block { return nil }

func (bc *blockChain) GetOrderedAll() []*types.Block { return nil }

func (bc *blockChain) GetRecursiveChildren(blockHash Hash) []*types.Block { return nil }

func (bc *blockChain) GetMaxView() types.View { return 0 }

func (bc *blockChain) LatestCommittedBlock() *types.Block { return nil }

func (bc *blockChain) LatestLockedBlock() *types.Block { return nil }

func (bc *blockChain) SetLatestCommittedBlock(block *types.Block) {}

func (bc *blockChain) SetLatestLockedBlock(block *types.Block) {}

func (bc *blockChain) GetQuorumCertOf(block *types.Block) *QuorumCert { return nil }

func (bc *blockChain) SetQuorumCertFor(block *types.Block, qc *QuorumCert) {}

func (bc *blockChain) IsValid() bool { return false }

func (bc *blockChain) Size() uint32 { return 0 }

func (bc *blockChain) String() string { return "" }

func (bc *blockChain) QuorumCertRef(block *types.Block) *types.Block { return nil }

func (bc *blockChain) ParentRef(block *types.Block) *types.Block { return nil }
