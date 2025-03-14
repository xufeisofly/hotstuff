package consensus

import (
	"time"

	tcrypto "github.com/xufeisofly/hotstuff/crypto"
	tmevents "github.com/xufeisofly/hotstuff/libs/events"
	"github.com/xufeisofly/hotstuff/libs/math"
	"github.com/xufeisofly/hotstuff/types"
)

type Pacemaker interface {
	AdvanceView(SyncInfo)
	OnLocalTimeout() error
	HandleTimeoutMsg(TimeoutMsg) error
}

type oneShotTimer struct {
	timerDoNotUse *time.Timer
}

func (t oneShotTimer) Stop() bool {
	return t.timerDoNotUse.Stop()
}

type TimeoutMsg struct {
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

type pacemaker struct {
	// pacemaker makes steps based on current state of the peer
	peerState   *PeerState
	crypto      Crypto
	leaderElect LeaderElect
	duration    ViewDuration

	evsw  tmevents.EventSwitch
	timer oneShotTimer
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

func (p *pacemaker) OnLocalTimeout() error { return nil }

func (p *pacemaker) HandleTimeoutMsg(timeoutMsg TimeoutMsg) error { return nil }

func (p *pacemaker) startTimer() {
	view := p.peerState.CurView()

	p.timer = oneShotTimer{time.AfterFunc(p.duration.GetDuration(), func() {
		// fire timeout event after duration
		p.evsw.FireEvent(types.EventViewTimeout, types.EventDataViewTimeout{
			View: view,
		})
	})}
}

func (p *pacemaker) stopTimer() {
	p.timer.Stop()
	// xufeisoflyishere
}
