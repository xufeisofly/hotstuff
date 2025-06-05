package state

import (
	"github.com/xufeisofly/hotstuff/types"
)

//------------------------------------------------------
// blockchain services types
// NOTE: Interfaces used by RPC must be thread safe!
//------------------------------------------------------

//------------------------------------------------------
// blockstore

//go:generate ../scripts/mockery_generate.sh BlockStore

// BlockStore defines the interface used by the ConsensusState.
type BlockStore interface {
	Base() int64
	Height() int64
	Size() int64

	LoadBaseMeta() *types.BlockMeta
	LoadBlockMeta(height int64) *types.BlockMeta
	LoadBlock(height int64) *types.Block

	SaveBlock(block *types.Block, blockParts *types.PartSet, seenCommit *types.Commit)

	PruneBlocks(height int64) (uint64, error)

	LoadBlockByHash(hash []byte) *types.Block
	LoadBlockPart(height int64, index int) *types.Part

	LoadBlockCommit(height int64) *types.Commit
	LoadSeenCommit(height int64) *types.Commit
}

type HsBlockStore interface {
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
	PruneTo(targetHash types.Hash) (forkedBlocks []*types.Block, err error)

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
	GetQuorumCertOf(blockHash types.Hash) *types.QuorumCert
	// Set QC for a block
	SetQuorumCertFor(blockHash types.Hash, qc *types.QuorumCert)

	// Is the chain valid
	IsValid() bool
	// Number of blocks in chain
	Size() int
	String() string

	// Get a block referenced by qc
	QuorumCertRef(block *types.Block) *types.Block
	// Get a block referenced by parent hash
	ParentRef(block *types.Block) *types.Block
}

//-----------------------------------------------------------------------------
// evidence pool

//go:generate ../scripts/mockery_generate.sh EvidencePool

// EvidencePool defines the EvidencePool interface used by State.
type EvidencePool interface {
	PendingEvidence(maxBytes int64) (ev []types.Evidence, size int64)
	AddEvidence(types.Evidence) error
	Update(State, types.EvidenceList)
	CheckEvidence(types.EvidenceList) error
}

// EmptyEvidencePool is an empty implementation of EvidencePool, useful for testing. It also complies
// to the consensus evidence pool interface
type EmptyEvidencePool struct{}

func (EmptyEvidencePool) PendingEvidence(maxBytes int64) (ev []types.Evidence, size int64) {
	return nil, 0
}
func (EmptyEvidencePool) AddEvidence(types.Evidence) error                    { return nil }
func (EmptyEvidencePool) Update(State, types.EvidenceList)                    {}
func (EmptyEvidencePool) CheckEvidence(evList types.EvidenceList) error       { return nil }
func (EmptyEvidencePool) ReportConflictingVotes(voteA, voteB *types.Vote)     {}
func (EmptyEvidencePool) ReportConflictingHsVotes(voteA, voteB *types.HsVote) {}
