package consensus

import (
	"bytes"
	"fmt"
	"runtime/debug"
	"time"

	tmcrypto "github.com/xufeisofly/hotstuff/crypto"
	tmevents "github.com/xufeisofly/hotstuff/libs/events"
	"github.com/xufeisofly/hotstuff/libs/log"
	"github.com/xufeisofly/hotstuff/libs/math"
	"github.com/xufeisofly/hotstuff/libs/service"
	"github.com/xufeisofly/hotstuff/types"
)

type Pacemaker interface {
	HighQC() *types.QuorumCert
	HighTC() *types.TimeoutCert
	CurView() types.View
	LocalAddress() tmcrypto.Address

	AdvanceView(SyncInfo)

	ReceiveMsg(msgInfo)

	SetPrivValidatorPubKey(tmcrypto.PubKey)
	SetProposeFunc(ProposeFunc)
	SetStopVotingFunc(StopVotingFunc)
}

type oneShotTimer struct {
	timerDoNotUse *time.Timer
}

func (t oneShotTimer) Stop() bool {
	return t.timerDoNotUse.Stop()
}

type pacemaker struct {
	service.BaseService

	msgQueue chan msgInfo
	done     chan struct{}

	highQC  *types.QuorumCert
	highTC  *types.TimeoutCert
	curView types.View

	crypto              Crypto
	privValidatorPubKey tmcrypto.PubKey

	leaderElect LeaderElect
	duration    ViewDuration

	timer oneShotTimer

	lastTimeout *TimeoutMessage

	evsw tmevents.EventSwitch

	proposeFunc    ProposeFunc
	stopVotingFunc StopVotingFunc
}

func NewPacemaker(
	c Crypto,
	leaderElect LeaderElect,
	duration ViewDuration,
) Pacemaker {
	genesisTC := types.NewTimeoutCert(nil, types.ViewBeforeGenesis)
	return &pacemaker{
		highQC:      &types.QuorumCertForGenesis,
		highTC:      &genesisTC,
		curView:     types.GenesisView,
		crypto:      c,
		leaderElect: leaderElect,
		duration:    duration,
		timer:       oneShotTimer{time.AfterFunc(0, func() {})},
	}
}

func (p *pacemaker) SetLogger(l log.Logger) {
	p.BaseService.Logger = l
}

func (p *pacemaker) SetProposeFunc(fn ProposeFunc) {
	p.proposeFunc = fn
}

func (p *pacemaker) SetStopVotingFunc(fn StopVotingFunc) {
	p.stopVotingFunc = fn
}

func (p *pacemaker) HighQC() *types.QuorumCert {
	return p.highQC
}

func (p *pacemaker) HighTC() *types.TimeoutCert {
	return p.highTC
}

func (p *pacemaker) CurView() types.View {
	return p.curView
}

func (p *pacemaker) updateHighQC(qc *types.QuorumCert) {
	if p.highQC.View() < qc.View() {
		p.highQC = qc
	}
}

func (p *pacemaker) updateHighTC(tc *types.TimeoutCert) {
	if p.highTC.View() < tc.View() {
		p.highTC = tc
	}
}

func (p *pacemaker) updateCurView(v types.View) {
	p.curView = v
}

func (p *pacemaker) AdvanceView(si SyncInfo) {
	if si.QC() == nil && si.TC() == nil {
		return
	}

	timeout := false
	if si.QC() != nil {
		p.updateHighQC(si.QC())
	}

	if si.TC() != nil {
		timeout = true
		p.updateHighTC(si.TC())
	}

	if si.AggQC() != nil {
		timeout = true

		highQC, ok := p.crypto.VerifyAggregateQC(*si.AggQC())
		if !ok {
			return
		}

		p.updateHighQC(&highQC)
	}

	newView := math.MaxInt64(p.highQC.View(), p.highTC.View()) + 1
	if newView <= p.curView {
		return
	}

	p.stopTimer()
	if !timeout {
		p.duration.ViewSucceeded()
	}

	p.updateCurView(newView)
	p.duration.ViewStarted()
	p.startTimer()

	if bytes.Equal(
		p.leaderElect.GetLeader(newView).Address,
		p.LocalAddress()) {
		p.proposeFunc(&si)
	} else {
		p.evsw.FireEvent(types.EventNewView, &NewViewMessage{si: &si})
	}
}

func (p *pacemaker) OnStart() error {
	go p.receiveRoutine()
	p.startTimer()
	return nil
}

func (p *pacemaker) OnStop() {
	p.stopTimer()
}

func (p *pacemaker) receiveRoutine() {
	onExit := func(p *pacemaker) {
		close(p.done)
	}

	defer func() {
		if r := recover(); r != nil {
			p.Logger.Error("PACEMAKER FAILURE!!!", "err", r, "stack", string(debug.Stack()))
			onExit(p)
		}
	}()

	for {
		var mi msgInfo

		select {
		case mi = <-p.msgQueue:
			p.handleMessage(mi)
		case <-p.Quit():
			onExit(p)
			return
		}
	}
}

func (p *pacemaker) onLocalTimeout() error {
	p.stopTimer()
	p.duration.ViewTimeout()
	defer p.startTimer()

	view := p.curView
	if p.lastTimeout != nil && p.lastTimeout.View == view {
		p.evsw.FireEvent(types.EventViewTimeout, p.lastTimeout)
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
		Sender:          p.LocalAddress(),
		View:            view,
		ViewHash:        viewHash,
		EpochView:       types.View(1),
		HighQC:          p.highQC,
		ViewSignature:   sig,
		HighQCSignature: nil, // TODO highqc signature
	}

	p.lastTimeout = timeoutMsg
	p.stopVotingFunc(view)
	p.evsw.FireEvent(types.EventViewTimeout, timeoutMsg)
	p.handleTimeoutMessage(timeoutMsg)

	return nil
}

func (p *pacemaker) ReceiveMsg(mi msgInfo) {
	select {
	case p.msgQueue <- mi:
	default:
		p.Logger.Debug("internal msg queue is full; using a go-routine")
		go func() { p.msgQueue <- mi }()
	}
}

func (p *pacemaker) SetPrivValidatorPubKey(pk tmcrypto.PubKey) {
	p.privValidatorPubKey = pk
}

func (p *pacemaker) LocalAddress() tmcrypto.Address {
	return p.privValidatorPubKey.Address()
}

func (p *pacemaker) handleMessage(mi msgInfo) {
	msg, _ := mi.Msg, mi.PeerID

	switch msg := msg.(type) {
	case *NewViewMessage:
		p.handleNewViewMessage(msg)
	case *TimeoutMessage:
		p.handleTimeoutMessage(msg)
	default:
		p.Logger.Error("unknown msg type", "type", fmt.Sprintf("%T", msg))
	}
}

func (p *pacemaker) handleNewViewMessage(newViewMsg *NewViewMessage) error {
	p.AdvanceView(*newViewMsg.si)
	return nil
}

func (p *pacemaker) handleTimeoutMessage(timeoutMsg *TimeoutMessage) error {
	curView := p.curView

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
		p.onLocalTimeout()
	})}
}

func (p *pacemaker) stopTimer() {
	p.timer.Stop()
}
