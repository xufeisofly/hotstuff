package consensus

import (
	tmcrypto "github.com/xufeisofly/hotstuff/crypto"
	"github.com/xufeisofly/hotstuff/types"
)

type epochInfo struct {
	view       types.View
	validators *types.ValidatorSet

	privValidatorPubKey tmcrypto.PubKey

	totalVotingPower  int64
	quorumVotingPower int64
}

func NewEpochInfo(
	view types.View,
	validators *types.ValidatorSet,
	privPubKey tmcrypto.PubKey,
) *epochInfo {
	total := validators.Size()
	return &epochInfo{
		view:                view,
		validators:          validators,
		totalVotingPower:    int64(total),
		quorumVotingPower:   int64(total*2/3 + 1),
		privValidatorPubKey: privPubKey,
	}
}

func (e *epochInfo) EpochView() types.View {
	return e.view
}

func (e *epochInfo) Validators() *types.ValidatorSet {
	return e.validators
}

func (e *epochInfo) LocalAddress() tmcrypto.Address {
	return e.privValidatorPubKey.Address()
}

func (e *epochInfo) TotalVotingPower() int64 {
	return e.totalVotingPower
}

func (e *epochInfo) QuorumVotingPower() int64 {
	return e.quorumVotingPower
}
