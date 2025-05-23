package consensus

import (
	"context"
	"fmt"
	"os"
	"testing"

	tmpubsub "github.com/xufeisofly/hotstuff/libs/pubsub"
	"github.com/xufeisofly/hotstuff/types"
)

func TestMain(m *testing.M) {
	config = ResetConfig("consensus_reactor_test")
	// consensusReplayConfig = ResetConfig("consensus_replay_test")
	// configStateTest := ResetConfig("consensus_state_test")
	// configMempoolTest := ResetConfig("consensus_mempool_test")
	// configByzantineTest := ResetConfig("consensus_byzantine_test")
	code := m.Run()
	os.RemoveAll(config.RootDir)
	// os.RemoveAll(consensusReplayConfig.RootDir)
	// os.RemoveAll(configStateTest.RootDir)
	// os.RemoveAll(configMempoolTest.RootDir)
	// os.RemoveAll(configByzantineTest.RootDir)
	os.Exit(code)
}

// 测试 proposer 打包广播 Proposal
func TestPropose(t *testing.T) {
	cs, _ := randConsensus(1)

	proposalCh := subscribe(cs.eventBus, types.EventQueryHsPropose)

	// TODO propose
	si := NewSyncInfo().WithQC(mockQuorumCert(1))
	cs.Propose(&si)
	ensureNewProposal(proposalCh, cs.CurView())
}

// 测试 proposer 选举
func TestProposerSelection(t *testing.T) {}

func subscribe(eventBus *types.EventBus, q tmpubsub.Query) <-chan tmpubsub.Message {
	sub, err := eventBus.Subscribe(context.Background(), testSubscriber, q)
	if err != nil {
		panic(fmt.Sprintf("failed to subscribe %s to %v", testSubscriber, q))
	}
	return sub.Out()
}

func mockQuorumCert(view types.View) types.QuorumCert {
	return types.NewQuorumCert(nil, view, types.BlockID{})
}
