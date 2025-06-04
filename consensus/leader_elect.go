package consensus

import (
	"fmt"
	"math/rand"
	"sort"

	wr "github.com/mroth/weightedrand"
	tcrypto "github.com/xufeisofly/hotstuff/crypto"
	"github.com/xufeisofly/hotstuff/libs/log"
	sm "github.com/xufeisofly/hotstuff/state"
	"github.com/xufeisofly/hotstuff/types"
)

type LeaderElect interface {
	GetLeader(types.View) *types.Validator
}

type leaderElect struct {
	blockchain Blockchain
	state      sm.State
	logger     log.Logger
}

func NewLeaderElect(blockchain Blockchain, state sm.State, l log.Logger) LeaderElect {
	return &leaderElect{
		blockchain: blockchain,
		state:      state,
		logger:     l,
	}
}

func (l *leaderElect) GetLeader(view types.View) *types.Validator {
	qc := QuorumCertBeforeGenesis()
	committedBlock := l.blockchain.LatestCommittedBlock()
	if committedBlock != nil {
		qc = *committedBlock.SelfCommit.CommitQC
	}

	// 对于创始 QC，默认使用 0 号 validator 作为 proposer
	if qc.View() == types.ViewBeforeGenesis || qc.View() == types.GenesisView {
		_, val := l.state.HsValidators.GetByIndex(0)
		return val
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

	fmt.Println("=======", weights)

	chooser, err := wr.NewChooser(weights...)
	if err != nil {
		l.logger.Error("weightedrand failed", "err", err)
	}
	fmt.Println("=======2", chooser)

	seed := int64(view)
	rnd := rand.New(rand.NewSource(seed))

	leaderAddrStr := chooser.PickSource(rnd).(string)
	_, val := l.state.HsValidators.GetByAddress([]byte(leaderAddrStr))
	return val
}
