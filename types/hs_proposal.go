package types

import (
	"fmt"
	"time"

	tmbytes "github.com/xufeisofly/hotstuff/libs/bytes"
	tmtime "github.com/xufeisofly/hotstuff/types/time"
)

// HsProposal proposal structure for hotstuff
type HsProposal struct {
	EpochView View `json:"epoch_view"`
	Block     *Block
	// TC
	TimeoutCert *TimeoutCert
	Timestamp   time.Time `json:"timestamp"`
	Signature   []byte    `json:"signature"`
}

func NewHsProposal(epochView View, block *Block, tc *TimeoutCert) *HsProposal {
	return &HsProposal{
		EpochView:   epochView,
		Block:       block,
		TimeoutCert: tc,
		Timestamp:   tmtime.Now(),
	}
}

func (p *HsProposal) ValidateBasic() error {
	return nil
}

func (p *HsProposal) String() string {
	return fmt.Sprintf("Proposal{%v (%v) %X @ %s}",
		p.Block.View,
		p.Block.Hash(),
		tmbytes.Fingerprint(p.Signature),
		CanonicalTime(p.Timestamp))
}

func (p *HsProposal) ToProto() interface{} {
	return nil
}

func HsProposalFromProto(pp interface{}) (*HsProposal, error) {
	return nil, nil
}
