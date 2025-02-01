package consensus

import "github.com/xufeisofly/hotstuff/types"

type epochInfo struct {
	height     uint64
	validators *types.ValidatorSet

	totalVotingPower  int64
	quorumVotingPower int64
}

func NewEpochInfo(validators *types.ValidatorSet) *epochInfo {
	total := validators.Size()
	return &epochInfo{
		validators:        validators,
		totalVotingPower:  int64(total),
		quorumVotingPower: int64(total*2/3 + 1),
	}
}

func (e *epochInfo) Height() uint64 {
	return e.height
}

func (e *epochInfo) Validators() *types.ValidatorSet {
	return e.validators
}

func (e *epochInfo) TotalVotingPower() int64 {
	return e.totalVotingPower
}

func (e *epochInfo) QuorumVotingPower() int64 {
	return e.quorumVotingPower
}
