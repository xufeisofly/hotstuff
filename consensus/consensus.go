package consensus

import (
	"fmt"
	"runtime/debug"

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
	blockchain Blockchain

	// create and execute blocks
	blockExec *sm.BlockExecutor

	// epoch info
	epochInfo *epochInfo

	// pacemaker Pacemaker
	// leaderRotation LeaderRotation

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

func (cs *Consensus) propose(syncInfo *SyncInfo) error {
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

func (cs *Consensus) handleProposalMsg(msg *ProposalMessage, peerID p2p.ID) {
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
		cs.handleProposalMsg(msg, peerID)
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
