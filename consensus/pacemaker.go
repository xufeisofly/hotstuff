package consensus

import (
	"bytes"
	"time"

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

type pacemaker struct {
	service.BaseService

	crypto      Crypto
	leaderElect LeaderElect
	duration    ViewDuration

	timer oneShotTimer

	lastTimeout *TimeoutMessage
	consensus   *Consensus
}

func NewPacemaker(
	c Crypto,
	leaderElect LeaderElect,
	duration ViewDuration,
) Pacemaker {
	return &pacemaker{
		crypto:      c,
		leaderElect: leaderElect,
		duration:    duration,
		timer:       oneShotTimer{time.AfterFunc(0, func() {})},
	}
}

func (p *pacemaker) SetLogger(l log.Logger) {
	p.BaseService.Logger = l
}

func (p *pacemaker) HighQC() *types.QuorumCert {
	return p.consensus.HighQC()
}

func (p *pacemaker) HighTC() *types.TimeoutCert {
	return p.consensus.HighTC()
}

func (p *pacemaker) UpdateHighQC(qc *types.QuorumCert) {
	if p.HighQC().View() < qc.View() {
		p.consensus.SetHighQC(qc)
	}
}

func (p *pacemaker) UpdateHighTC(tc *types.TimeoutCert) {
	if p.HighTC().View() < tc.View() {
		p.consensus.SetHighTC(tc)
	}
}

func (p *pacemaker) CurView() types.View {
	return p.consensus.CurView()
}

func (p *pacemaker) UpdateCurView(v types.View) {
	p.consensus.SetCurView(v)
}

func (p *pacemaker) AdvanceView(si SyncInfo) {
	if si.QC() == nil && si.TC() == nil {
		return
	}

	timeout := false
	if si.QC() != nil {
		p.UpdateHighQC(si.QC())
	}

	if si.TC() != nil {
		timeout = true
		p.UpdateHighTC(si.TC())
	}

	if si.AggQC() != nil {
		timeout = true

		highQC, ok := p.crypto.VerifyAggregateQC(*si.AggQC())
		if !ok {
			return
		}

		p.UpdateHighQC(&highQC)
	}

	newView := math.MaxInt64(p.HighQC().View(), p.HighTC().View()) + 1
	if newView <= p.CurView() {
		return
	}

	p.stopTimer()
	if !timeout {
		p.duration.ViewSucceeded()
	}

	p.UpdateCurView(newView)
	p.duration.ViewStarted()
	p.startTimer()

	if bytes.Equal(
		p.leaderElect.GetLeader(newView).Address,
		p.peerState.LocalAddress()) {
		p.consensus.Propose(&si)
	} else {
		p.consensus.evsw.FireEvent(types.EventNewView, &NewViewMessage{si: &si})
	}
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

	view := p.CurView()
	if p.lastTimeout != nil && p.lastTimeout.View == view {
		p.consensus.evsw.FireEvent(types.EventViewTimeout, p.lastTimeout)
		return nil
	}

	p.Logger.Debug("OnLocalTimeout", "view", view)
	// make timeout message
	viewHash := types.GetViewHash(view)
	sig, err := p.crypto.Sign(viewHash)
	if err != nil {
		return err
	}

	timeoutMsg := &TimeoutMessage{
		Sender:          p.peerState.LocalAddress(),
		View:            view,
		ViewHash:        viewHash,
		EpochView:       types.View(1),
		HighQC:          p.HighQC(),
		ViewSignature:   sig,
		HighQCSignature: nil, // TODO highqc signature
	}

	p.lastTimeout = timeoutMsg
	p.consensus.StopVoting(view)
	p.consensus.evsw.FireEvent(types.EventViewTimeout, timeoutMsg)
	p.HandleTimeoutMessage(timeoutMsg)

	return nil
}

func (p *pacemaker) HandleNewViewMessage(newViewMsg *NewViewMessage) error {
	p.AdvanceView(*newViewMsg.si)
	return nil
}

func (p *pacemaker) HandleTimeoutMessage(timeoutMsg *TimeoutMessage) error {
	curView := p.CurView()

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
	return nil
}

func (p *pacemaker) startTimer() {
	p.timer = oneShotTimer{time.AfterFunc(p.duration.GetDuration(), func() {
		// fire timeout event after duration
		// trigger OnLocalTimeout
		p.OnLocalTimeout()
	})}
}

func (p *pacemaker) stopTimer() {
	p.timer.Stop()
}
