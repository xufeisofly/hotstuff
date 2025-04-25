package consensus

import (
	"fmt"

	tcrypto "github.com/xufeisofly/hotstuff/crypto"
	tmjson "github.com/xufeisofly/hotstuff/libs/json"
	"github.com/xufeisofly/hotstuff/p2p"
	tmcons "github.com/xufeisofly/hotstuff/proto/hotstuff/consensus"
	typesproto "github.com/xufeisofly/hotstuff/proto/hotstuff/types"
	"github.com/xufeisofly/hotstuff/types"
)

type ProposeFunc func(*SyncInfo) error
type StopVotingFunc func(types.View)

// SyncInfo holds the highest known QC or TC.
// Generally, if highQC.View > highTC.View, there is no need to include highTC in the SyncInfo.
// However, if highQC.View < highTC.View, we should still include highQC.
// This can also hold an AggregateQC for Fast-Hotstuff.
type SyncInfo struct {
	qc    *types.QuorumCert
	tc    *types.TimeoutCert
	aggQC *types.AggregateQC
}

// NewSyncInfo returns a new SyncInfo struct.
func NewSyncInfo() SyncInfo {
	return SyncInfo{}
}

// WithQC returns a copy of the SyncInfo struct with the given QC.
func (si SyncInfo) WithQC(qc types.QuorumCert) SyncInfo {
	si.qc = new(types.QuorumCert)
	*si.qc = qc
	return si
}

// WithTC returns a copy of the SyncInfo struct with the given TC.
func (si SyncInfo) WithTC(tc types.TimeoutCert) SyncInfo {
	si.tc = new(types.TimeoutCert)
	*si.tc = tc
	return si
}

// WithAggQC returns a copy of the SyncInfo struct with the given AggregateQC.
func (si SyncInfo) WithAggQC(aggQC types.AggregateQC) SyncInfo {
	si.aggQC = new(types.AggregateQC)
	*si.aggQC = aggQC
	return si
}

func (si SyncInfo) QC() *types.QuorumCert {
	return si.qc
}

func (si SyncInfo) TC() *types.TimeoutCert {
	return si.tc
}

func (si SyncInfo) AggQC() *types.AggregateQC {
	return si.aggQC
}

func (si SyncInfo) ToProto() *typesproto.SyncInfo {
	return nil
}

//-----------------------------------------------------------------------------
// Messages

// Message is a message that can be sent and received on the Reactor
type msgInfo struct {
	Msg    Message `json:"msg"`
	PeerID p2p.ID  `json:"peer_key"`
}

type Message interface {
	ValidateBasic() error
}

func init() {
	tmjson.RegisterType(&ProposalMessage{}, "hotstuff/Proposal")
	// tmjson.RegisterType(&BlockPartMessage{}, "tendermint/BlockPart")
	tmjson.RegisterType(&VoteMessage{}, "hotstuff/Vote")
	tmjson.RegisterType(&NewViewMessage{}, "hotstuff/NewView")
	tmjson.RegisterType(&TimeoutMessage{}, "hotstuff/ViewTimeout")
}

//-------------------------------------

// ProposalMessage is sent when a new block is proposed.
type ProposalMessage struct {
	Proposal *types.HsProposal
}

// ValidateBasic performs basic validation.
func (m *ProposalMessage) ValidateBasic() error {
	return m.Proposal.ValidateBasic()
}

// String returns a string representation.
func (m *ProposalMessage) String() string {
	return fmt.Sprintf("[Proposal %v]", m.Proposal)
}

//-------------------------------------

// VoteMessage is sent when voting for a proposal (or lack thereof).
type VoteMessage struct {
	Vote *types.HsVote
}

// ValidateBasic performs basic validation.
func (m *VoteMessage) ValidateBasic() error {
	return m.Vote.ValidateBasic()
}

// String returns a string representation.
func (m *VoteMessage) String() string {
	return fmt.Sprintf("[Vote %v]", m.Vote)
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

func (tmsg *TimeoutMessage) ValidateBasic() error {
	return nil
}

func (tmsg *TimeoutMessage) ToProto() *tmcons.TimeoutMessage {
	return &tmcons.TimeoutMessage{
		Sender:          tmsg.Sender,
		View:            tmsg.View,
		ViewHash:        tmsg.ViewHash,
		EpochView:       tmsg.EpochView,
		HighQc:          tmsg.HighQC.ToProto(),
		ViewSignature:   tmsg.ViewSignature.ToBytes(),
		HighQcSignature: tmsg.HighQCSignature.ToBytes(),
	}
}

type NewViewMessage struct {
	si *SyncInfo
}

func (nvmsg *NewViewMessage) ValidateBasic() error {
	return nil
}
