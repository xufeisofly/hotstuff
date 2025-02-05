package consensus

import (
	"bytes"

	"github.com/xufeisofly/hotstuff/libs/log"
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
	Get(blockHash types.Hash) *types.Block
	// If has block
	Has(blockhash types.Hash) bool
	// If block and target share the same branch
	Extends(block, target *types.Block) bool
	// Prune from the latest prune view to target block
	PruneTo(targetHash types.Hash, forkedBlocks []*types.Block) error

	// Get all blocks
	GetAll() []*types.Block
	// Get all verified blocks
	GetAllVerified() []*types.Block
	// Get all blocks ordered by view
	GetOrderedAll() []*types.Block
	// Get all children blocks of a block
	GetRecursiveChildren(blockHash types.Hash) []*types.Block

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
	GetQuorumCertOf(block *types.Block) *types.QuorumCert
	// Set QC for a block
	SetQuorumCertFor(blockHash types.Hash, qc *types.QuorumCert)

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
	qc       *types.QuorumCert
}

type blockChain struct {
	startBlock *types.Block // the starting block of the chain
	pruneView  types.View   // latest view that has been pruned

	blocksAtView  map[types.View]*types.Block // mapping from view to block
	wrappedBlocks map[string]*wrappedBlock    // mapping from block hash to wrapped block

	latestCommittedBlock *types.Block
	latestLockedBlock    *types.Block

	blockStore sm.BlockStore

	logger log.Logger
}

var _ BlockChain = (*blockChain)(nil)

func newBlockChain(blockStore sm.BlockStore, l log.Logger) BlockChain {
	return &blockChain{
		startBlock:           nil,
		pruneView:            types.ViewBeforeGenesis,
		blocksAtView:         make(map[types.View]*types.Block),
		wrappedBlocks:        make(map[string]*wrappedBlock),
		latestCommittedBlock: nil,
		latestLockedBlock:    nil,
		blockStore:           blockStore,
		logger:               l,
	}
}

func (bc *blockChain) Store(block *types.Block) error {
	if block.View <= bc.pruneView {
		return nil
	}

	if bc.Has(block.Hash()) {
		bc.logger.Debug("block already stored",
			"hash", block.Hash(),
			"view", block.View,
		)
		return nil
	}

	// no block before
	if bc.startBlock == nil {
		bc.startBlock = block
		bc.addWrappedBlock(wrap(block))
		bc.blocksAtView[block.View] = block
		bc.pruneView = block.View
		return nil
	}

	// parent of start block can be stored
	if bytes.Equal(bc.startBlock.LastBlockID.Hash, block.Hash()) {
		bc.addWrappedBlock(wrap(block))
		bc.blocksAtView[block.View] = block
		bc.addChild(bc.startBlock)
		bc.SetQuorumCertFor(bc.startBlock.QuorumCert.BlockHash(), bc.startBlock.QuorumCert)
		bc.startBlock = block
		return nil
	}

	// parent block must exsit
	if bc.ParentRef(block) == nil {
		bc.logger.Error("lack of parent block",
			"hash", block.Hash(),
			"view", block.View,
		)
		return ErrNotFoundParentBlock{View: block.View, Hash: block.Hash()}
	}

	// qc referenced block must exist
	if block.QuorumCert != nil && bc.QuorumCertRef(block) == nil {
		bc.logger.Error("lack of qc ref block",
			"hash", block.Hash(),
			"view", block.View,
		)
		return ErrNotFoundQcRefBlock{View: block.View, Hash: block.Hash()}
	}

	bc.addWrappedBlock(wrap(block))
	bc.blocksAtView[block.View] = block
	bc.addChild(block)

	return nil
}

func (bc *blockChain) Store2Db(block *types.Block) error { return nil }

func (bc *blockChain) Get(blockHash types.Hash) *types.Block {
	v, ok := bc.wrappedBlocks[blockHash.String()]
	if ok && v != nil {
		return v.block
	}
	return nil
}

func (bc *blockChain) Has(blockhash types.Hash) bool { return false }

func (bc *blockChain) Extends(block, target *types.Block) bool { return false }

func (bc *blockChain) PruneTo(targetHash types.Hash, forkedBlocks []*types.Block) error { return nil }

func (bc *blockChain) GetAll() []*types.Block { return nil }

func (bc *blockChain) GetAllVerified() []*types.Block { return nil }

func (bc *blockChain) GetOrderedAll() []*types.Block { return nil }

func (bc *blockChain) GetRecursiveChildren(blockHash types.Hash) []*types.Block { return nil }

func (bc *blockChain) GetMaxView() types.View { return 0 }

func (bc *blockChain) LatestCommittedBlock() *types.Block { return nil }

func (bc *blockChain) LatestLockedBlock() *types.Block { return nil }

func (bc *blockChain) SetLatestCommittedBlock(block *types.Block) {}

func (bc *blockChain) SetLatestLockedBlock(block *types.Block) {}

func (bc *blockChain) GetQuorumCertOf(block *types.Block) *types.QuorumCert { return nil }

func (bc *blockChain) SetQuorumCertFor(blockHash types.Hash, qc *types.QuorumCert) {}

func (bc *blockChain) IsValid() bool { return false }

func (bc *blockChain) Size() uint32 { return 0 }

func (bc *blockChain) String() string { return "" }

func (bc *blockChain) QuorumCertRef(block *types.Block) *types.Block { return nil }

func (bc *blockChain) ParentRef(block *types.Block) *types.Block { return nil }

func wrap(block *types.Block) *wrappedBlock {
	return &wrappedBlock{
		block: block,
	}
}

func (bc *blockChain) addWrappedBlock(wb *wrappedBlock) {
	if wb.block.Hash() == nil {
		panic("wb block hash is nil")
	}

	v, ok := bc.wrappedBlocks[wb.block.Hash().String()]
	if ok && v != nil {
		bc.logger.Debug("block exsits",
			"view", wb.block.View,
			"parent hash", wb.block.LastBlockID.Hash,
			"tx size", len(wb.block.Data.Txs),
		)
		return
	}

	if ok {
		wb.children = v.children
	}

	bc.wrappedBlocks[wb.block.Hash().String()] = wb

	bc.logger.Debug("success add block",
		"view", wb.block.View,
		"parent hash", wb.block.LastBlockID.Hash,
		"tx size", len(wb.block.Data.Txs),
	)
}

func (bc *blockChain) addChild(b *types.Block) {
	if bc.latestCommittedBlock != nil && b.View <= bc.latestCommittedBlock.View {
		return
	}

	parentBlockHashStr := b.LastBlockID.Hash.String()
	_, ok := bc.wrappedBlocks[parentBlockHashStr]
	if !ok {
		bc.wrappedBlocks[parentBlockHashStr] = wrap(nil)
		bc.logger.Debug("add nil parent block", "view", b.View)
	}

	bc.wrappedBlocks[parentBlockHashStr].children = append(
		bc.wrappedBlocks[parentBlockHashStr].children, b)
}
