package consensus

import (
	"errors"
	"fmt"
	"sync"

	"github.com/gogo/protobuf/proto"
	tmevents "github.com/xufeisofly/hotstuff/libs/events"
	tmjson "github.com/xufeisofly/hotstuff/libs/json"
	"github.com/xufeisofly/hotstuff/libs/log"
	tmsync "github.com/xufeisofly/hotstuff/libs/sync"
	"github.com/xufeisofly/hotstuff/p2p"
	tmcons "github.com/xufeisofly/hotstuff/proto/hotstuff/consensus"
	sm "github.com/xufeisofly/hotstuff/state"
	"github.com/xufeisofly/hotstuff/types"
)

const (
	StateChannel       = byte(0x20)
	DataChannel        = byte(0x21)
	VoteChannel        = byte(0x22)
	VoteSetBitsChannel = byte(0x23)

	maxMsgSize = 1048576 // 1MB; NOTE/TODO: keep in sync with types.PartSet sizes.

	blocksToContributeToBecomeGoodPeer = 10000
	votesToContributeToBecomeGoodPeer  = 10000
)

type Reactor struct {
	p2p.BaseReactor

	cons      *Consensus
	pacemaker Pacemaker

	mtx      tmsync.RWMutex
	waitSync bool
	eventBus *types.EventBus

	Metrics *Metrics
}

type ReactorOption func(*Reactor)

func NewReactor(consensus *Consensus, waitSync bool, options ...ReactorOption) *Reactor {
	conR := &Reactor{
		cons:     consensus,
		waitSync: waitSync,
		Metrics:  NopMetrics(),
	}
	conR.BaseReactor = *p2p.NewBaseReactor("Consensus", conR)

	for _, option := range options {
		option(conR)
	}

	return conR
}

func (conR *Reactor) OnStart() error {
	conR.Logger.Info("Reactor ", "waitSync", conR.WaitSync())

	// TODO Start stats goroutine
	conR.subscribeEvents()

	if !conR.WaitSync() {
		err := conR.cons.Start()
		if err != nil {
			return err
		}
	}

	return nil
}

func (conR *Reactor) OnStop() {
	conR.unsubscribeEvents()
	if err := conR.cons.Stop(); err != nil {
		conR.Logger.Error("Error stopping consensus state", "err", err)
	}
	if !conR.WaitSync() {
		conR.cons.Wait()
	}
}

// No idea how this function works.
func (conR *Reactor) SwitchToConsensus(state sm.State, skipWAL bool) {
	return
}

func (conR *Reactor) GetChannels() []*p2p.ChannelDescriptor {
	return []*p2p.ChannelDescriptor{
		{
			ID:                  DataChannel,
			Priority:            10,
			SendQueueCapacity:   100,
			RecvBufferCapacity:  50 * 4096,
			RecvMessageCapacity: maxMsgSize,
			MessageType:         &tmcons.Message{},
		},
	}
}

func (conR *Reactor) InitPeer(peer p2p.Peer) p2p.Peer {
	peerState := NewPeerState(peer).SetLogger(conR.Logger)
	peer.Set(types.PeerStateKey, peerState)
	return peer
}

func (conR *Reactor) AddPeer(peer p2p.Peer) {
	if !conR.IsRunning() {
		return
	}

	// TODO gossip routine for block parts
	// TODO gossip routine for votes

	if !conR.WaitSync() {
		// TODO
	}
}

func (conR *Reactor) RemovePeer(peer p2p.Peer, reason interface{}) {
	if !conR.IsRunning() {
		return
	}
}

// getValidatorPeer get validator's peer for p2p messaging
func (conR *Reactor) getValidatorPeer(v types.Validator) p2p.Peer {
	peerID := p2p.PubKeyToID(v.PubKey)
	return conR.Switch.Peers().Get(peerID)
}

func (conR *Reactor) ReceiveEnvelope(e p2p.Envelope) {
	if !conR.IsRunning() {
		conR.Logger.Debug("Receive", "src", e.Src, "chId", e.ChannelID)
		return
	}
	m := e.Message
	if wm, ok := m.(p2p.Wrapper); ok {
		m = wm.Wrap()
	}
	msg, err := MsgFromProto(m.(*tmcons.Message))
	if err != nil {
		conR.Logger.Error("Error decoding message", "src", e.Src, "chId", e.ChannelID, "err", err)
		conR.Switch.StopPeerForError(e.Src, err)
		return
	}

	if err = msg.ValidateBasic(); err != nil {
		conR.Logger.Error("Peer sent us invalid msg", "peer", e.Src, "msg", e.Message, "err", err)
		conR.Switch.StopPeerForError(e.Src, err)
		return
	}

	conR.Logger.Debug("Receive", "src", e.Src, "chId", e.ChannelID, "msg", msg)

	switch e.ChannelID {
	case DataChannel:
		if conR.WaitSync() {
			conR.Logger.Info("Ignoring message received during sync", "msg", msg)
			return
		}
		switch msg := msg.(type) {
		case *ProposalMessage, *VoteMessage:
			conR.cons.ReceiveMsg(msgInfo{msg, e.Src.ID()})
		case *NewViewMessage, *TimeoutMessage:
			conR.pacemaker.ReceiveMsg(msgInfo{msg, e.Src.ID()})
		}
	}
}

func (conR *Reactor) Receive(chID byte, peer p2p.Peer, msgBytes []byte) {
	msg := &tmcons.Message{}
	err := proto.Unmarshal(msgBytes, msg)
	if err != nil {
		panic(err)
	}
	uw, err := msg.Unwrap()
	if err != nil {
		panic(err)
	}
	conR.ReceiveEnvelope(p2p.Envelope{
		ChannelID: chID,
		Src:       peer,
		Message:   uw,
	})
}

// SetEventBus sets event bus.
func (conR *Reactor) SetEventBus(b *types.EventBus) {
	conR.eventBus = b
	conR.cons.SetEventBus(b)
}

// WaitSync returns whether the consensus reactor is waiting for state/fast sync.
func (conR *Reactor) WaitSync() bool {
	conR.mtx.RLock()
	defer conR.mtx.RUnlock()
	return conR.waitSync
}

// subscribeToBroadcastEvents subscribes for new round steps and votes
// using internal pubsub defined on state to broadcast
// them to peers upon receiving.
func (conR *Reactor) subscribeEvents() {
	const subscriber = "consensus-reactor"
	if err := conR.cons.evsw.AddListenerForEvent(subscriber, types.EventPropose,
		func(data tmevents.EventData) {
			conR.broadcastProposalMessage(data.(*ProposalMessage))
		}); err != nil {
		conR.Logger.Error("Error adding listener for events", "err", err)
	}

	if err := conR.cons.evsw.AddListenerForEvent(subscriber, types.EventVote,
		func(data tmevents.EventData) {
			conR.sendVoteMessage(data.(*VoteMessage))
		}); err != nil {
		conR.Logger.Error("Error adding listener for events", "err", err)
	}

	if err := conR.cons.evsw.AddListenerForEvent(subscriber, types.EventNewView,
		func(data tmevents.EventData) {
			conR.broadcastNewViewMessage(data.(*NewViewMessage))
		}); err != nil {
		conR.Logger.Error("Error adding listener for events", "err", err)
	}

	if err := conR.cons.evsw.AddListenerForEvent(subscriber, types.EventViewTimeout,
		func(data tmevents.EventData) {
			conR.broadcastTimeoutMessage(data.(*TimeoutMessage))
		}); err != nil {
		conR.Logger.Error("Error adding listener for events", "err", err)
	}
}

func (conR *Reactor) unsubscribeEvents() {
	const subscriber = "consensus-reactor"
	conR.cons.evsw.RemoveListener(subscriber)
}

func (conR *Reactor) broadcastProposalMessage(proposalMsg *ProposalMessage) {
	// TODO actual block data should not use broadcast but gossip
	// otherwise it will spend too much bandwidth

	conR.Switch.BroadcastEnvelope(p2p.Envelope{
		ChannelID: DataChannel,
		Message: &tmcons.ProposalMessage{
			Proposal: *proposalMsg.Proposal.ToProto(),
		},
	})
}

func (conR *Reactor) sendVoteMessage(voteMsg *VoteMessage) {
	// TODO get the next proposer
	nextProposer := conR.getValidatorPeer(*conR.cons.leaderElect.GetLeader(voteMsg.Vote.View))
	logger := conR.Logger.With("peer", nextProposer)

	p2p.SendEnvelopeShim(nextProposer, p2p.Envelope{
		ChannelID: DataChannel,
		Message: &tmcons.VoteMessage{
			Vote: *voteMsg.Vote.ToProto(),
		},
	}, logger)
}

func (conR *Reactor) broadcastNewViewMessage(newViewMsg *NewViewMessage) {
	conR.Switch.BroadcastEnvelope(p2p.Envelope{
		ChannelID: DataChannel,
		Message: &tmcons.NewViewMessage{
			SyncInfo: *newViewMsg.si.ToProto(),
		},
	})
}

func (conR *Reactor) broadcastTimeoutMessage(timeoutMsg *TimeoutMessage) {
	conR.Switch.BroadcastEnvelope(p2p.Envelope{
		ChannelID: DataChannel,
		Message:   timeoutMsg.ToProto(),
	})
}

func (conR *Reactor) String() string {
	return "ConsensusReactor"
}

// StringIndented returns an indented string representation of the Reactor
func (conR *Reactor) StringIndented(indent string) string {
	s := "ConsensusReactor{\n"
	s += indent + "  " + conR.cons.StringIndented(indent+"  ") + "\n"
	for _, peer := range conR.Switch.Peers().List() {
		ps, ok := peer.Get(types.PeerStateKey).(*PeerState)
		if !ok {
			panic(fmt.Sprintf("Peer %v has no state", peer))
		}
		s += indent + "  " + ps.StringIndented(indent+"  ") + "\n"
	}
	s += indent + "}"
	return s
}

// ReactorMetrics sets the metrics
func ReactorMetrics(metrics *Metrics) ReactorOption {
	return func(conR *Reactor) { conR.Metrics = metrics }
}

//-----------------------------------------------------------------------------

var (
	ErrPeerStateHeightRegression = errors.New("error peer state height regression")
	ErrPeerStateInvalidStartTime = errors.New("error peer state invalid startTime")
)

// PeerState contains the known state of a peer, including its connection and
// threadsafe access to its PeerRoundState.
// NOTE: THIS GETS DUMPED WITH rpc/core/consensus.go.
// Be mindful of what you Expose.
type PeerState struct {
	peer   p2p.Peer
	logger log.Logger

	mtx sync.Mutex // NOTE: Modify below using setters, never directly.

	Stats *peerStateStats `json:"stats"` // Exposed.
}

// peerStateStats holds internal statistics for a peer.
type peerStateStats struct {
	Votes      int `json:"votes"`
	BlockParts int `json:"block_parts"`
}

func (pss peerStateStats) String() string {
	return fmt.Sprintf("peerStateStats{votes: %d, blockParts: %d}",
		pss.Votes, pss.BlockParts)
}

// NewPeerState returns a new PeerState for the given Peer
func NewPeerState(peer p2p.Peer) *PeerState {
	return &PeerState{
		peer:   peer,
		logger: log.NewNopLogger(),
		Stats:  &peerStateStats{},
	}
}

// SetLogger allows to set a logger on the peer state. Returns the peer state
// itself.
func (ps *PeerState) SetLogger(logger log.Logger) *PeerState {
	ps.logger = logger
	return ps
}

// ToJSON returns a json of PeerState.
func (ps *PeerState) ToJSON() ([]byte, error) {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	return tmjson.Marshal(ps)
}

func (ps *PeerState) String() string {
	return ps.StringIndented("")
}

// StringIndented returns a string representation of the PeerState
func (ps *PeerState) StringIndented(indent string) string {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()
	return fmt.Sprintf(`PeerState{
%s  Key        %v
%s  Stats      %v
%s}`,
		indent, ps.peer.ID(),
		indent, ps.Stats,
		indent)
}
