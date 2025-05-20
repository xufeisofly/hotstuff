package consensus

import (
	"context"
	"fmt"
	"testing"

	tmpubsub "github.com/xufeisofly/hotstuff/libs/pubsub"
	"github.com/xufeisofly/hotstuff/types"
)

// 测试 proposer 打包广播 Proposal
func TestPropose(t *testing.T) {
	cs, _ := randConsensus(1)

	proposalCh := subscribe(cs.eventBus, types.EventQueryHsPropose)

	// TODO propose
	ensureNewProposal(proposalCh, cs.CurView())
	// xufeisoflyishere
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
