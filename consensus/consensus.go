package consensus

import (
	"bytes"
	"errors"
	"fmt"
	"runtime/debug"
	"time"

	cfg "github.com/xufeisofly/hotstuff/config"
	tmevents "github.com/xufeisofly/hotstuff/libs/events"
	"github.com/xufeisofly/hotstuff/libs/log"
	"github.com/xufeisofly/hotstuff/libs/service"
	tmsync "github.com/xufeisofly/hotstuff/libs/sync"
	"github.com/xufeisofly/hotstuff/p2p"
	sm "github.com/xufeisofly/hotstuff/state"
	"github.com/xufeisofly/hotstuff/types"
)

type msgInfo struct {
	Msg    Message `json:"msg"`
	PeerID p2p.ID  `json:"peer_key"`
}

type txNotifier interface {
	TxsAvailable() <-chan struct{}
}

type Consensus struct {
	service.BaseService

	// config details
	config *cfg.ConsensusConfig

	// crypto to encrypt and decrypt
	crypto Crypto

	// store blocks and commits
	blockchain Blockchain // 使用 Blockchain 作为存储，不要使用 tendermint 的 BlockStore，结构不同

	// create and execute blocks
	blockExec *sm.BlockExecutor

	// epoch info
	epochInfo *epochInfo

	pacemaker   Pacemaker
	leaderElect LeaderElect

	lastVoteView types.View

	// notify us if txs sare available
	txNotifier txNotifier

	// internal state
	mtx tmsync.Mutex

	peerState *PeerState

	state sm.State // State until view-1.
	// privValidator pubkey, memoized for the duration of one block
	// to avoid extra requests to HSM

	// state changes may be triggered by: msgs from peers,
	msgQueue chan msgInfo
	// msgs from ourself, or by timeouts
	// internalMsgQueue chan msgInfo
	// timeoutTicker    TimeoutTicker

	// closed when we finish shutting down
	done chan struct{}

	// synchronous pubsub between consensus state and reactor.
	// state only emits EventNewRoundStep and EventVote
	evsw tmevents.EventSwitch

	// for reporting metrics
	metrics *Metrics
}

type ConsensusOption func(*Consensus)

func NewConsensus(options ...ConsensusOption) *Consensus {
	cs := &Consensus{}

	for _, opt := range options {
		opt(cs)
	}

	return cs
}

func (cs *Consensus) SetLogger(l log.Logger) {
	cs.BaseService.Logger = l
}

func (cs *Consensus) StringIndented(indent string) string {
	return ""
}

func (cs *Consensus) String() string {
	return "Consensus"
}

func (cs *Consensus) OnStart() error {
	return nil
}

func (cs *Consensus) OnStop() {}

func (cs *Consensus) Propose(syncInfo *SyncInfo) error {
	proposal, err := cs.createProposal(syncInfo)
	if err != nil {
		return err
	}

	// broadcast proposal msg
	cs.evsw.FireEvent(types.EventPropose, &ProposalMessage{
		Proposal: proposal,
	})
	return nil
}

func (cs *Consensus) createProposal(syncInfo *SyncInfo) (*types.HsProposal, error) {
	proposerAddr := cs.epochInfo.LocalAddress()

	block, _ := cs.blockExec.HsCreateProposalBlock(cs.peerState.CurView(), cs.state, cs.peerState.HighQC(), proposerAddr)

	return types.NewHsProposal(
		cs.peerState.CurEpochView(),
		block,
		syncInfo.TC(),
	), nil
}

func (cs *Consensus) HandleProposalMessage(msg *ProposalMessage, peerID p2p.ID) error {
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
		// xufeisoflyishere
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
			ValidatorAddress: cs.epochInfo.LocalAddress(),
			EpochView:        cs.epochInfo.EpochView(),
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
	if qc.View() <= cs.peerState.HighQC().View() {
		return true
	}

	return cs.crypto.VerifyQuorumCert(*qc)
}

func (cs *Consensus) verifyTC(tc *types.TimeoutCert) bool {
	// an old timeout cert is treated as verified by default
	if tc.View() <= cs.peerState.HighTC().View() {
		return true
	}

	return cs.crypto.VerifyTimeoutCert(*tc)
}

func (cs *Consensus) handleVoteMsg(msg *VoteMessage, peerID p2p.ID) {
}

func (cs *Consensus) receiveRoutine() {
	onExit := func(cs *Consensus) {
		close(cs.done)
	}

	defer func() {
		if r := recover(); r != nil {
			cs.Logger.Error("CONSENSUS FAILURE!!!", "err", r, "stack", string(debug.Stack()))
			// stop gracefully
			//
			// NOTE: We most probably shouldn't be running any further when there is
			// some unexpected panic. Some unknown error happened, and so we don't
			// know if that will result in the validator signing an invalid thing. It
			// might be worthwhile to explore a mechanism for manual resuming via
			// some console or secure RPC system, but for now, halting the chain upon
			// unexpected consensus bugs sounds like the better option.
			onExit(cs)
		}
	}()

	for {
		var mi msgInfo

		select {
		case mi = <-cs.msgQueue:
			cs.handleMsg(mi)
		case <-cs.Quit():
			onExit(cs)
			return
		}
	}
}

func (cs *Consensus) handleMsg(mi msgInfo) {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()

	msg, peerID := mi.Msg, mi.PeerID

	switch msg := msg.(type) {
	case *ProposalMessage:
		cs.HandleProposalMessage(msg, peerID)
	case *VoteMessage:
		cs.handleVoteMsg(msg, peerID)
	default:
		cs.Logger.Error("unknown msg type", "type", fmt.Sprintf("%T", msg))
	}
}

func (cs *Consensus) sendMsg(mi msgInfo) {
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
