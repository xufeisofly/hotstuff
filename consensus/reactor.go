package consensus

import (
	"fmt"

	"github.com/gogo/protobuf/proto"
	tmsync "github.com/xufeisofly/hotstuff/libs/sync"
	"github.com/xufeisofly/hotstuff/p2p"
	tmcons "github.com/xufeisofly/hotstuff/proto/hotstuff/consensus"
	sm "github.com/xufeisofly/hotstuff/state"
	"github.com/xufeisofly/hotstuff/types"
)

type Reactor struct {
	p2p.BaseReactor

	conS *State

	mtx      tmsync.RWMutex
	waitSync bool
	eventBus *types.EventBus

	Metrics *Metrics
}

type ReactorOption func(*Reactor)

func NewReactor(consensusState *State, waitSync bool, options ...ReactorOption) *Reactor {
	conR := &Reactor{
		conS:     consensusState,
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
	// TODO subscribe broadcast events

	if !conR.WaitSync() {
		err := conR.conS.Start()
		if err != nil {
			return err
		}
	}

	return nil
}

func (conR *Reactor) OnStop() {
	// TODO unsubscribe broadcast events
	if err := conR.conS.Stop(); err != nil {
		conR.Logger.Error("Error stopping consensus state", "err", err)
	}
	if !conR.WaitSync() {
		conR.conS.Wait()
	}
}

// No idea how this function works.
func (conR *Reactor) SwitchToConsensus(state sm.State, skipWAL bool) {
	return
}

func (conR *Reactor) GetChannels() []*p2p.ChannelDescriptor {
	return []*p2p.ChannelDescriptor{}
}

func (conR *Reactor) InitPeer(peer p2p.Peer) p2p.Peer {
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

func (conR *Reactor) ReceiveEnvelope(e p2p.Envelope) {
	// TODO Apply by e.ChannelID and msg.Type
	// distribute msg to different funcs

	// TODO put sig to cs.{channels}, use the state receive routine to handle msgs
	// like this: cs.peerMsgQueue <- msgInfo{msg, e.Src.ID()}
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

func (conR *Reactor) SetEventBus(b *types.EventBus) {
	conR.eventBus = b
	conR.conS.SetEventBus(b)
}

// WaitSync returns whether the consensus reactor is waiting for state/fast sync.
func (conR *Reactor) WaitSync() bool {
	conR.mtx.RLock()
	defer conR.mtx.RUnlock()
	return conR.waitSync
}

func (conR *Reactor) String() string {
	return "ConsensusReactor"
}

// StringIndented returns an indented string representation of the Reactor
func (conR *Reactor) StringIndented(indent string) string {
	s := "ConsensusReactor{\n"
	s += indent + "  " + conR.conS.StringIndented(indent+"  ") + "\n"
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
