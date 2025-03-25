package consensus

import (
	"bytes"
	"time"

	tcrypto "github.com/xufeisofly/hotstuff/crypto"
	tmevents "github.com/xufeisofly/hotstuff/libs/events"
	"github.com/xufeisofly/hotstuff/libs/log"
	"github.com/xufeisofly/hotstuff/libs/math"
	"github.com/xufeisofly/hotstuff/libs/service"
	"github.com/xufeisofly/hotstuff/types"
)

type Pacemaker interface {
	AdvanceView(SyncInfo)
	OnLocalTimeout() error
	HandleTimeoutMessage(*TimeoutMessage) error
	HandleNewViewMessage(*NewViewMessage) error
}

type oneShotTimer struct {
	timerDoNotUse *time.Timer
}

func (t oneShotTimer) Stop() bool {
	return t.timerDoNotUse.Stop()
}

type TimeoutMessage struct {
	Sender    types.Address
	View      types.View
	ViewHash  types.Hash
	EpochView types.View
	HighQC    *types.QuorumCert
	// signature of view hash
	ViewSignature tcrypto.QuorumSignature
	// signature of high qc
	HighQCSignature tcrypto.QuorumSignature
}

type NewViewMessage struct {
	si *SyncInfo
}

type pacemaker struct {
	service.BaseService
	// pacemaker makes steps based on current state of the peer
	peerState   *PeerState
	crypto      Crypto
	leaderElect LeaderElect
	duration    ViewDuration

	evsw  tmevents.EventSwitch
	timer oneShotTimer

	lastTimeout *TimeoutMessage
	consensus   *Consensus
}

func NewPacemaker(
	ps *PeerState,
	c Crypto,
	leaderElect LeaderElect,
	duration ViewDuration,
) Pacemaker {
	return &pacemaker{
		peerState:   ps,
		crypto:      c,
		leaderElect: leaderElect,
		duration:    duration,

		timer: oneShotTimer{time.AfterFunc(0, func() {})},
	}
}

func (p *pacemaker) SetLogger(l log.Logger) {
	p.BaseService.Logger = l
}

func (p *pacemaker) AdvanceView(si SyncInfo) {
	if si.QC() == nil && si.TC() == nil {
		return
	}

	timeout := false
	if si.QC() != nil {
		p.peerState.UpdateHighQC(si.QC())
	}

	if si.TC() != nil {
		timeout = true
		p.peerState.UpdateHighTC(si.TC())
	}

	if si.AggQC() != nil {
		timeout = true

		highQC, ok := p.crypto.VerifyAggregateQC(*si.AggQC())
		if !ok {
			return
		}

		p.peerState.UpdateHighQC(&highQC)
	}

	newView := math.MaxInt64(p.peerState.HighQC().View(), p.peerState.HighTC().View()) + 1
	if newView <= p.peerState.CurView() {
		return
	}

	p.stopTimer()
	if !timeout {
		p.duration.ViewSucceeded()
	}

	p.peerState.UpdateCurView(newView)
	p.duration.ViewStarted()
	p.startTimer()
}

func (p *pacemaker) OnStart() error {
	p.startTimer()
	return nil
}

func (p *pacemaker) OnStop() {
	p.stopTimer()
}

func (p *pacemaker) OnLocalTimeout() error {
	p.stopTimer()
	p.duration.ViewTimeout()
	defer p.startTimer()

	view := p.peerState.CurView()
	if p.lastTimeout != nil && p.lastTimeout.View == view {
		return p.broadcastTimeoutMessage(p.lastTimeout)
	}

	p.Logger.Debug("OnLocalTimeout", "view", view)
	// make timeout message
	viewHash := types.GetViewHash(view)
	sig, err := p.crypto.Sign(viewHash)
	if err != nil {
		return err
	}

	timeoutMsg := &TimeoutMessage{
		Sender:          p.peerState.epochInfo.LocalAddress(),
		View:            view,
		ViewHash:        viewHash,
		EpochView:       p.peerState.CurEpochView(),
		HighQC:          p.peerState.HighQC(),
		ViewSignature:   sig,
		HighQCSignature: nil, // TODO highqc signature
	}

	p.lastTimeout = timeoutMsg
	p.consensus.StopVoting(view)
	p.broadcastTimeoutMessage(timeoutMsg)
	p.HandleTimeoutMessage(timeoutMsg)

	return nil
}

func (p *pacemaker) HandleNewViewMessage(newViewMsg *NewViewMessage) error {
	p.AdvanceView(*newViewMsg.si)
	return nil
}

func (p *pacemaker) HandleTimeoutMessage(timeoutMsg *TimeoutMessage) error {
	curView := p.peerState.CurView()

	si := NewSyncInfo().WithQC(*timeoutMsg.HighQC)
	p.AdvanceView(si)

	aggSig, ok := p.crypto.CollectPartialSignature(
		curView,
		timeoutMsg.ViewHash,
		timeoutMsg.ViewSignature)
	if !ok || !aggSig.IsValid() {
		return nil
	}

	tc := types.NewTimeoutCert(aggSig, curView)
	si = si.WithTC(tc)
	p.AdvanceView(si)

	if bytes.Equal(
		p.leaderElect.GetLeader().Address,
		p.peerState.epochInfo.LocalAddress()) {
		p.consensus.Propose(&si)
	} else {
		p.broadcastNewViewMessage(&NewViewMessage{si: &si})
	}
	return nil
}

func (p *pacemaker) startTimer() {
	p.timer = oneShotTimer{time.AfterFunc(p.duration.GetDuration(), func() {
		// fire timeout event after duration
		// trigger OnLocalTimeout
		p.OnLocalTimeout()
	})}
}

func (p *pacemaker) broadcastTimeoutMessage(msg *TimeoutMessage) error {
	p.evsw.FireEvent(types.EventViewTimeout, msg)
	return nil
}

func (p *pacemaker) broadcastNewViewMessage(msg *NewViewMessage) error {
	p.evsw.FireEvent(types.EventNewView, msg)
	return nil
}

func (p *pacemaker) stopTimer() {
	p.timer.Stop()
}
