package consensus

import (
	"github.com/xufeisofly/hotstuff/types"
)

type EpochInfo struct {
	view       types.View
	validators *types.ValidatorSet

	totalVotingPower  int64
	quorumVotingPower int64
}

func NewEpochInfo(
	view types.View,
	validators *types.ValidatorSet,
) *EpochInfo {
	total := validators.Size()
	return &EpochInfo{
		view:              view,
		validators:        validators,
		totalVotingPower:  int64(total),
		quorumVotingPower: int64(total*2/3 + 1),
	}
}

func (e *EpochInfo) EpochView() types.View {
	return e.view
}

func (e *EpochInfo) Validators() *types.ValidatorSet {
	return e.validators
}

func (e *EpochInfo) TotalVotingPower() int64 {
	return e.totalVotingPower
}

func (e *EpochInfo) QuorumVotingPower() int64 {
	return e.quorumVotingPower
}
