package store

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"

	"github.com/xufeisofly/hotstuff/crypto/bls"
	"github.com/xufeisofly/hotstuff/libs/log"
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

type wrappedBlock struct {
	block    *types.Block
	status   BlockStatus
	children []*types.Block
	selfQC   *types.QuorumCert
}

func (wb *wrappedBlock) addChild(block *types.Block) {
	wb.children = append(wb.children, block)
}

func (wb *wrappedBlock) setStatus(s BlockStatus) {
	wb.status = s
}

func (wb *wrappedBlock) setSelfQC(qc *types.QuorumCert) {
	wb.selfQC = qc
}

type HsBlockStore struct {
	startBlock *types.Block // the starting block of the chain
	pruneView  types.View   // latest view that has been pruned

	blocksAtView  map[types.View]*types.Block // mapping from view to block
	wrappedBlocks map[string]*wrappedBlock    // mapping from block hash to wrapped block

	latestCommittedBlock *types.Block
	latestLockedBlock    *types.Block

	blockStore *BlockStore

	logger log.Logger
}

func NewHsBlockStore(blockStore *BlockStore) *HsBlockStore {
	return &HsBlockStore{
		startBlock:           nil,
		pruneView:            types.ViewBeforeGenesis,
		blocksAtView:         make(map[types.View]*types.Block),
		wrappedBlocks:        make(map[string]*wrappedBlock),
		latestCommittedBlock: nil,
		latestLockedBlock:    nil,
		blockStore:           blockStore,
		logger:               log.NewNopLogger(),
	}
}

func (bc *HsBlockStore) SetLogger(l log.Logger) {
	bc.logger = l
}

func (bc *HsBlockStore) Store(block *types.Block) error {
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
	if bytes.Equal(bc.startBlock.ParentHash(), block.Hash()) {
		bc.addWrappedBlock(wrap(block))
		bc.blocksAtView[block.View] = block
		bc.addChild(bc.startBlock)
		bc.SetQuorumCertFor(bc.startBlock.QuorumCert.BlockID().Hash, bc.startBlock.QuorumCert)
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

func (bc *HsBlockStore) Store2Db(block *types.Block) error {
	// TODO blockStore.SaveBlock need to be modified
	return nil
}

func (bc *HsBlockStore) Get(blockHash types.Hash) *types.Block {
	v, ok := bc.wrappedBlocks[string(blockHash)]
	if ok && v != nil {
		return v.block
	}
	return nil
}

func (bc *HsBlockStore) Has(blockHash types.Hash) bool {
	v, ok := bc.wrappedBlocks[string(blockHash)]
	return ok && v != nil
}

func (bc *HsBlockStore) Extends(block, target *types.Block) bool {
	cur := block
	for cur != nil && cur.View > target.View {
		parent := bc.ParentRef(cur)
		if parent == nil {
			break
		}
		cur = parent
	}
	return bytes.Equal(cur.Hash(), target.Hash())
}

func (bc *HsBlockStore) PruneTo(targetHash types.Hash) (forkedBlocks []*types.Block, err error) {
	forkedBlocks = make([]*types.Block, 0)
	cur := bc.Get(targetHash)
	if cur == nil {
		return forkedBlocks, ErrNotFoundBlock{Hash: targetHash}
	}

	targetBlock := cur
	targetView := cur.View
	// target view has already been pruned to
	if bc.pruneView >= targetView {
		return forkedBlocks, nil
	}

	// get all block hashes in the same branch of target block
	canonicalHashes := make(map[string]struct{})
	canonicalHashes[string(cur.Hash())] = struct{}{}
	for cur.View > bc.pruneView {
		cur = bc.ParentRef(cur)
		if cur != nil {
			canonicalHashes[string(cur.Hash())] = struct{}{}
			continue
		}
		return forkedBlocks, ErrNotFoundBlock{Hash: cur.Hash()}
	}

	startBlock, ok := bc.blocksAtView[bc.pruneView]
	if startBlock == nil || !ok {
		return forkedBlocks, ErrNotFoundBlock{View: bc.pruneView}
	}

	// prune all branch from start to target except canonical branch
	bc.pruneToTarget(startBlock.Hash(), targetHash, canonicalHashes, &forkedBlocks)
	bc.pruneView = targetView
	bc.startBlock = targetBlock

	return forkedBlocks, nil
}

func (bc *HsBlockStore) GetAll() []*types.Block {
	var all []*types.Block
	for _, wrapped := range bc.wrappedBlocks {
		all = append(all, wrapped.block)
	}
	return all
}

func (bc *HsBlockStore) GetAllVerified() []*types.Block {
	var all []*types.Block
	for _, wrapped := range bc.wrappedBlocks {
		if wrapped.block.QuorumCert != nil {
			all = append(all, wrapped.block)
		}
	}
	return all
}

func (bc *HsBlockStore) GetOrderedAll() []*types.Block {
	blocks := bc.GetAll()
	sort.Slice(blocks, func(i, j int) bool {
		return blocks[i].View < blocks[j].View
	})
	return blocks
}

// GetRecursiveChildren gets all children of a block hash recursively
func (bc *HsBlockStore) GetRecursiveChildren(blockHash types.Hash) []*types.Block {
	var all []*types.Block

	levelChildren := bc.getChildren(blockHash)
	if len(levelChildren) == 0 {
		return []*types.Block{}
	}

	for _, levelChild := range levelChildren {
		all = append(all, levelChild)
		all = append(all, bc.GetRecursiveChildren(levelChild.Hash())...)
	}

	return all
}

func (bc *HsBlockStore) GetMaxView() types.View {
	var maxView types.View = 0
	for view := range bc.blocksAtView {
		if view > maxView {
			maxView = view
		}
	}
	return maxView
}

func (bc *HsBlockStore) LatestCommittedBlock() *types.Block {
	return bc.latestCommittedBlock
}

func (bc *HsBlockStore) LatestLockedBlock() *types.Block {
	return bc.latestLockedBlock
}

func (bc *HsBlockStore) SetLatestCommittedBlock(block *types.Block) {
	// omit old block
	if bc.latestCommittedBlock != nil && bc.latestCommittedBlock.View >= block.View {
		return
	}

	if block.SelfCommit == nil {
		panic(fmt.Sprintf("block must have a CommitQC, view: %d", block.View))
	}

	bc.latestCommittedBlock = block
	if wrapped, ok := bc.wrappedBlocks[string(block.Hash())]; ok {
		wrapped.setStatus(BlockStatus_Committed)
	}
}

func (bc *HsBlockStore) SetLatestLockedBlock(block *types.Block) {
	if wrapped, ok := bc.wrappedBlocks[string(block.Hash())]; ok {
		if wrapped.status != BlockStatus_Committed {
			bc.latestLockedBlock = block
			wrapped.setStatus(BlockStatus_Locked)
		}
	}
}

func (bc *HsBlockStore) GetQuorumCertOf(blockHash types.Hash) *types.QuorumCert {
	if wrapped, ok := bc.wrappedBlocks[string(blockHash)]; ok {
		return wrapped.selfQC
	}
	return nil
}

func (bc *HsBlockStore) SetQuorumCertFor(blockHash types.Hash, qc *types.QuorumCert) {
	if wrapped, ok := bc.wrappedBlocks[string(blockHash)]; ok {
		wrapped.setSelfQC(qc)
	}
}

func (bc *HsBlockStore) IsValid() bool {
	if bc.Size() == 0 {
		return false
	}
	// a valid chain has only one node which is the start node
	// that don't has a parent
	num := 0
	for _, wrapped := range bc.wrappedBlocks {
		if wrapped.block == nil {
			continue
		}
		parent := bc.ParentRef(wrapped.block)
		if parent == nil {
			num++
		}
	}
	return num == 1
}

func (bc *HsBlockStore) Size() int {
	return len(bc.wrappedBlocks)
}

func (bc *HsBlockStore) String() string {
	blocks := bc.GetOrderedAll()

	var ret string
	for _, block := range blocks {
		ret += "," + strconv.Itoa(int(block.View))
	}
	return ret
}

func (bc *HsBlockStore) QuorumCertRef(block *types.Block) *types.Block {
	if block == nil || block.QuorumCert == nil {
		return nil
	}
	return bc.Get(block.QuorumCert.BlockID().Hash)
}

func (bc *HsBlockStore) ParentRef(block *types.Block) *types.Block {
	if block == nil {
		return nil
	}
	return bc.Get(block.ParentHash())
}

func wrap(block *types.Block) *wrappedBlock {
	return &wrappedBlock{
		block: block,
	}
}

func (bc *HsBlockStore) addWrappedBlock(wb *wrappedBlock) {
	if wb.block.Hash() == nil {
		panic("wb block hash is nil")
	}

	v, ok := bc.wrappedBlocks[wb.block.Hash().String()]
	if ok && v != nil {
		bc.logger.Debug("block exsits",
			"view", wb.block.View,
			"parent hash", wb.block.ParentHash(),
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
		"parent hash", wb.block.ParentHash(),
		"tx size", len(wb.block.Data.Txs),
	)
}

func (bc *HsBlockStore) addChild(b *types.Block) {
	if bc.latestCommittedBlock != nil && b.View <= bc.latestCommittedBlock.View {
		return
	}

	parentBlockHashStr := b.ParentHash().String()
	_, ok := bc.wrappedBlocks[parentBlockHashStr]
	if !ok {
		bc.wrappedBlocks[parentBlockHashStr] = wrap(nil)
		bc.logger.Debug("add nil parent block", "view", b.View)
	}

	bc.wrappedBlocks[parentBlockHashStr].addChild(b)
}

func (bc *HsBlockStore) pruneToTarget(
	startHash types.Hash,
	targetHash types.Hash,
	canonicalHashes map[string]struct{},
	forkedBlocks *[]*types.Block,
) {
	if bytes.Equal(startHash, targetHash) {
		return
	}

	children := bc.getChildren(startHash)
	if len(children) == 0 {
		return
	}

	for _, child := range children {
		// delete the block not in the branch of hashes
		if _, ok := canonicalHashes[string(child.Hash())]; !ok {
			bc.deleteBlock(child)
			*forkedBlocks = append(*forkedBlocks, child)
		}
		bc.pruneToTarget(child.Hash(), targetHash, canonicalHashes, forkedBlocks)
	}
}

func (bc *HsBlockStore) getChildren(blockHash types.Hash) []*types.Block {
	v, ok := bc.wrappedBlocks[string(blockHash)]
	if !ok {
		return []*types.Block{}
	}
	return v.children
}

func (bc *HsBlockStore) deleteBlock(block *types.Block) error {
	blockHash := block.Hash()
	wrappedParent := bc.wrappedBlocks[string(block.ParentHash())]
	if wrappedParent != nil {
		// delete the block from parent children array
		for i, child := range wrappedParent.children {
			if bytes.Equal(child.Hash(), blockHash) {
				wrappedParent.children = append(wrappedParent.children[:i], wrappedParent.children[i+1:]...)
			}
		}
	}

	blockAtView := bc.blocksAtView[block.View]
	if bytes.Equal(blockAtView.Hash(), blockHash) {
		delete(bc.blocksAtView, block.View)
	}

	delete(bc.wrappedBlocks, string(blockHash))
	return nil
}

func QuorumCertBeforeGenesis() types.QuorumCert {
	return types.NewQuorumCert(
		&bls.EmptyAggregateSignature,
		types.ViewBeforeGenesis,
		types.BlockID{},
	)
}

func QuorumCertOfGenesis(genesisBlockID types.BlockID) types.QuorumCert {
	return types.NewQuorumCert(
		&bls.EmptyAggregateSignature,
		types.GenesisView,
		genesisBlockID,
	)
}
