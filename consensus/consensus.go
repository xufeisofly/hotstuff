package consensus

import (
	"bytes"
	"errors"
	"fmt"
	"runtime/debug"
	"time"

	cfg "github.com/xufeisofly/hotstuff/config"
	tmcrypto "github.com/xufeisofly/hotstuff/crypto"
	tmevents "github.com/xufeisofly/hotstuff/libs/events"
	"github.com/xufeisofly/hotstuff/libs/log"
	"github.com/xufeisofly/hotstuff/libs/service"
	tmsync "github.com/xufeisofly/hotstuff/libs/sync"
	"github.com/xufeisofly/hotstuff/p2p"
	sm "github.com/xufeisofly/hotstuff/state"
	"github.com/xufeisofly/hotstuff/types"
)

type txNotifier interface {
	TxsAvailable() <-chan struct{}
}

type evidencePool interface {
	// reports conflicting votes to the evidence pool to be processed into evidence
	ReportConflictingHsVotes(voteA, voteB *types.HsVote)
}

type Consensus struct {
	service.BaseService

	// config details
	config        *cfg.HsConsensusConfig
	privValidator types.PrivValidator

	// crypto to encrypt and decrypt
	crypto    Crypto
	pacemaker Pacemaker

	// store blocks and commits
	blockchain Blockchain // 使用 Blockchain 作为存储，不要使用 tendermint 的 BlockStore，结构不同

	// create and execute blocks
	blockExec   *sm.BlockExecutor
	leaderElect LeaderElect

	lastVoteView types.View

	// notify us if txs sare available
	txNotifier txNotifier

	// add evidence to the pool
	// when it's detected
	evpool evidencePool

	// internal state
	mtx tmsync.RWMutex

	state sm.State // State until view-1.
	// privValidator pubkey, memoized for the duration of one block
	// to avoid extra requests to HSM

	// state changes may be triggered by: msgs from peers,
	msgQueue chan msgInfo
	// msgs from ourself, or by timeouts
	// internalMsgQueue chan msgInfo
	// timeoutTicker    TimeoutTicker

	// we use eventBus to trigger msg broadcasts in the reactor,
	// and to notify external subscribers, eg. through a websocket
	eventBus *types.EventBus

	// closed when we finish shutting down
	done chan struct{}

	// synchronous pubsub between consensus state and reactor.
	// state only emits EventNewRoundStep and EventVote
	evsw tmevents.EventSwitch

	// for reporting metrics
	metrics *Metrics
}

type ConsensusOption func(*Consensus)

func NewConsensus(
	config *cfg.HsConsensusConfig,
	state sm.State,
	blockExec *sm.BlockExecutor,
	blockchain Blockchain,
	txNotifier txNotifier,
	evpool evidencePool,
	pacemaker Pacemaker,
	options ...ConsensusOption,
) *Consensus {
	cs := &Consensus{}
	cs.BaseService = *service.NewBaseService(nil, "Consensus", cs)
	cs.pacemaker = pacemaker

	for _, opt := range options {
		opt(cs)
	}

	return cs
}

func (cs *Consensus) SetLogger(l log.Logger) {
	cs.BaseService.Logger = l
}

// SetEventBus sets event bus.
func (cs *Consensus) SetEventBus(b *types.EventBus) {
	cs.eventBus = b
	cs.blockExec.SetEventBus(b)
}

func (cs *Consensus) StringIndented(indent string) string {
	return ""
}

func (cs *Consensus) String() string {
	return "Consensus"
}

func (cs *Consensus) CurView() types.View {
	return cs.pacemaker.CurView()
}

// GetState returns a copy of the chain state.
func (cs *Consensus) GetState() sm.State {
	cs.mtx.RLock()
	defer cs.mtx.RUnlock()
	return cs.state.Copy()
}

func (cs *Consensus) OnStart() error {
	if err := cs.evsw.Start(); err != nil {
		return err
	}
	go cs.receiveRoutine()
	return nil
}

func (cs *Consensus) OnStop() {
	if err := cs.evsw.Stop(); err != nil {
		cs.Logger.Error("failed trying to stop eventSwitch", "error", err)
	}
}

// GetValidators returns a copy of the current validators.
func (cs *Consensus) GetValidators() (int64, []*types.Validator) {
	cs.mtx.RLock()
	defer cs.mtx.RUnlock()
	return cs.state.LastBlockHeight, cs.state.Validators.Copy().Validators
}

// SetPrivValidator sets the private validator account for signing votes. It
// immediately requests pubkey and caches it.
func (cs *Consensus) SetPrivValidator(priv types.PrivValidator) {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	cs.privValidator = priv

	if err := cs.updatePrivValidatorPubKey(); err != nil {
		cs.Logger.Error("failed to get private validator pubkey", "err", err)
	}
}

func (cs *Consensus) updatePrivValidatorPubKey() error {
	if cs.privValidator == nil {
		return nil
	}

	pubKey, err := cs.privValidator.GetPubKey()
	if err != nil {
		return err
	}
	cs.pacemaker.SetPrivValidatorPubKey(pubKey)
	return nil
}

func (cs *Consensus) LocalAddress() tmcrypto.Address {
	return cs.pacemaker.LocalAddress()
}

func (cs *Consensus) Propose(syncInfo *SyncInfo) error {
	proposal, err := cs.createProposal(syncInfo)
	if err != nil {
		return err
	}

	proposeEvent := types.EventDataHsCompleteProposal{
		View:    proposal.Block.View,
		BlockID: proposal.Block.ID(),
	}
	if cs.eventBus != nil {
		if err := cs.eventBus.PublishEventHsCompleteProposal(proposeEvent); err != nil {
			cs.Logger.Error("failed publishing new propose", "err", err)
		}
	}

	// broadcast proposal msg
	cs.evsw.FireEvent(types.EventPropose, &ProposalMessage{
		Proposal: proposal,
	})
	return nil
}

func (cs *Consensus) createProposal(syncInfo *SyncInfo) (*types.HsProposal, error) {
	proposerAddr := cs.LocalAddress()

	block, _ := cs.blockExec.HsCreateProposalBlock(cs.pacemaker.CurView(), cs.state, cs.pacemaker.HighQC(), proposerAddr)

	return types.NewHsProposal(
		types.View(1),
		block,
		syncInfo.TC(),
	), nil
}

func (cs *Consensus) handleProposalMessage(msg *ProposalMessage, peerID p2p.ID) error {
	block := msg.Proposal.Block
	if !cs.verifyQC(block.QuorumCert) {
		return errors.New("verify quorum cert failed")
	}

	// verify leader
	if !bytes.Equal(
		block.ProposerAddress,
		cs.leaderElect.GetLeader(block.View).Address) {
		return errors.New("verify leader failed")
	}

	if !cs.checkVoteRule(block) {
		return errors.New("check vote rule failed")
	}

	cs.blockExec.ValidateBlock(cs.state, block)

	// accept block
	cs.blockchain.Store(block)

	if b := cs.getBlockToCommit(block); b != nil {
		cs.commit(b)
	}

	cs.pacemaker.AdvanceView(NewSyncInfo().WithQC(*block.QuorumCert))

	// has vote
	if cs.lastVoteView >= block.View {
		cs.Logger.Info("block view too old", "view", block.View)
		return fmt.Errorf("invalid proposal view: %v", block.View)
	}

	// vote
	sig, err := cs.crypto.Sign(block.Hash())
	if err != nil {
		cs.Logger.Error("failed to sign block", "view", block.View, "err", err)
		return err
	}

	cs.StopVoting(block.View)

	cs.evsw.FireEvent(types.EventVote, &VoteMessage{
		Vote: &types.HsVote{
			View:             block.View,
			BlockID:          block.ID(),
			ValidatorAddress: cs.LocalAddress(),
			EpochView:        types.View(1),
			Timestamp:        time.Now(),
			Signature:        sig,
		},
	})

	return nil
}

func (cs *Consensus) checkVoteRule(block *types.Block) bool {
	qcBlock := cs.blockchain.QuorumCertRef(block)
	if qcBlock != nil && qcBlock.View > cs.blockchain.LatestLockedBlock().View {
		return true
	} else if cs.blockchain.Extends(block, cs.blockchain.LatestLockedBlock()) {
		return true
	}
	return false
}

func (cs *Consensus) getBlockToCommit(block *types.Block) *types.Block {
	block1 := cs.blockchain.QuorumCertRef(block)
	if block1 == nil {
		return nil
	}

	block2 := cs.blockchain.QuorumCertRef(block1)
	if block2 == nil {
		return nil
	}

	// update latest locked block
	if block2.View > cs.blockchain.LatestLockedBlock().View {
		cs.blockchain.SetLatestLockedBlock(block2)
	}

	block3 := cs.blockchain.QuorumCertRef(block2)
	if block3 == nil {
		return nil
	}

	if block2.HashesTo(block1.ParentHash()) && block3.HashesTo(block2.ParentHash()) {
		return block3
	}

	return nil
}

func (cs *Consensus) commit(block *types.Block) {
	err := cs.commitInner(block)
	if err != nil {
		cs.Logger.Error("failed to commit", "error", err)
		return
	}

	forkedBlocks, err := cs.blockchain.PruneTo(block.Hash())
	if err != nil {
		cs.Logger.Error("prune failed", "blockHash", block.Hash(), "err", err)
		return
	}
	// TODO return txs of forkedBlocks to mempool
	cs.Logger.Debug("forkedBlocks", "blocks size", len(forkedBlocks))
	return
}

func (cs *Consensus) commitInner(block *types.Block) error {
	if cs.blockchain.LatestCommittedBlock().View >= block.View {
		return nil
	}
	if parent := cs.blockchain.ParentRef(block); parent != nil {
		err := cs.commitInner(parent)
		if err != nil {
			return err
		}
	} else {
		return fmt.Errorf("failed to locate block: %s", block.ParentHash())
	}
	cs.Logger.Debug("committing block", "block", block)
	_, _, err := cs.blockExec.ApplyBlock(cs.state.Copy(), block.ID(), block)
	return err
}

func (cs *Consensus) verifyQC(qc *types.QuorumCert) bool {
	if qc.View() <= cs.pacemaker.HighQC().View() {
		return true
	}

	return cs.crypto.VerifyQuorumCert(*qc)
}

func (cs *Consensus) verifyTC(tc *types.TimeoutCert) bool {
	// an old timeout cert is treated as verified by default
	if tc.View() <= cs.pacemaker.HighTC().View() {
		return true
	}

	return cs.crypto.VerifyTimeoutCert(*tc)
}

func (cs *Consensus) handleVoteMessage(msg *VoteMessage, peerID p2p.ID) {
	blockHash := msg.Vote.BlockID.Hash
	block := cs.blockchain.Get(blockHash)
	if block == nil {
		// TODO synchronizing block from neighbor node
		return
	}

	if block.View <= cs.pacemaker.HighQC().View() {
		return
	}

	aggSig, ok := cs.crypto.CollectPartialSignature(
		block.View,
		blockHash,
		msg.Vote.Signature)
	if !ok || !aggSig.IsValid() {
		return
	}

	qc := types.NewQuorumCert(aggSig, block.View, block.ID())
	si := NewSyncInfo().WithQC(qc)
	cs.evsw.FireEvent(types.EventNewView, &NewViewMessage{si: &si})
}

func (cs *Consensus) receiveRoutine() {
	onExit := func(cs *Consensus) {
		close(cs.done)
	}

	defer func() {
		if r := recover(); r != nil {
			cs.Logger.Error("CONSENSUS FAILURE!!!", "err", r, "stack", string(debug.Stack()))
			onExit(cs)
		}
	}()

	for {
		var mi msgInfo

		select {
		case mi = <-cs.msgQueue:
			cs.handleMessage(mi)
		case <-cs.Quit():
			onExit(cs)
			return
		}
	}
}

func (cs *Consensus) handleMessage(mi msgInfo) {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	msg, peerID := mi.Msg, mi.PeerID

	switch msg := msg.(type) {
	case *ProposalMessage:
		cs.handleProposalMessage(msg, peerID)
	case *VoteMessage:
		cs.handleVoteMessage(msg, peerID)
	default:
		cs.Logger.Error("unknown msg type", "type", fmt.Sprintf("%T", msg))
	}
}

func (cs *Consensus) ReceiveMsg(mi msgInfo) {
	select {
	case cs.msgQueue <- mi:
	default:
		cs.Logger.Debug("internal msg queue is full; using a go-routine")
		go func() { cs.msgQueue <- mi }()
	}
}

func (cs *Consensus) StopVoting(view types.View) {
	cs.lastVoteView = view
}
