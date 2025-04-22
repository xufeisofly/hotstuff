package consensus

import (
	"math/rand"
	"sort"

	wr "github.com/mroth/weightedrand"
	tcrypto "github.com/xufeisofly/hotstuff/crypto"
	"github.com/xufeisofly/hotstuff/libs/log"
	"github.com/xufeisofly/hotstuff/types"
)

type LeaderElect interface {
	GetLeader(types.View) *types.Validator
}

type leaderElect struct {
	blockchain Blockchain
	epochInfo  *EpochInfo
	logger     log.Logger
}

func NewLeaderElect(blockchain Blockchain, epochInfo *EpochInfo, l log.Logger) LeaderElect {
	return &leaderElect{
		blockchain: blockchain,
		epochInfo:  epochInfo,
		logger:     l,
	}
}

func (l *leaderElect) GetLeader(view types.View) *types.Validator {
	qc := QuorumCertBeforeGenesis()
	committedBlock := l.blockchain.LatestCommittedBlock()
	if committedBlock != nil {
		qc = *committedBlock.SelfCommit.CommitQC
	}

	voters := qc.Signature().Participants()
	weights := make([]wr.Choice, 0, len(voters))

	voters.ForEach(func(addr tcrypto.Address) {
		weights = append(weights, wr.Choice{
			Item:   addr.String(),
			Weight: 1,
		})
	})

	sort.Slice(weights, func(i, j int) bool {
		return weights[i].Item.(string)[0] > weights[j].Item.(string)[0]
	})

	chooser, err := wr.NewChooser(weights...)
	if err != nil {
		l.logger.Error("weightedrand failed", "err", err)
	}

	seed := int64(view)
	rnd := rand.New(rand.NewSource(seed))

	leaderAddrStr := chooser.PickSource(rnd).(string)
	_, val := l.epochInfo.Validators().Copy().GetByAddress([]byte(leaderAddrStr))
	return val
}
