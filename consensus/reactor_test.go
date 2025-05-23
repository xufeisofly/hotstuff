package consensus

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/xufeisofly/hotstuff/libs/log"
	"github.com/xufeisofly/hotstuff/p2p"
	"github.com/xufeisofly/hotstuff/types"
)

func startConsensusNet(t *testing.T, css []*Consensus, n int) (
	[]*Reactor,
	[]types.Subscription,
	[]*types.EventBus,
) {
	reactors := make([]*Reactor, n)
	blocksSubs := make([]types.Subscription, 0)
	eventBuses := make([]*types.EventBus, n)
	for i := 0; i < n; i++ {
		/*logger, err := tmflags.ParseLogLevel("consensus:info,*:error", logger, "info")
		if err != nil {	t.Fatal(err)}*/
		reactors[i] = NewReactor(css[i], true) // so we dont start the consensus states
		reactors[i].SetLogger(css[i].Logger)

		// eventBus is already started with the cs
		eventBuses[i] = css[i].eventBus
		reactors[i].SetEventBus(eventBuses[i])

		blocksSub, err := eventBuses[i].Subscribe(context.Background(), testSubscriber, types.EventQueryNewBlock)
		require.NoError(t, err)
		blocksSubs = append(blocksSubs, blocksSub)

		if css[i].state.LastBlockView == types.ViewBeforeGenesis { // simulate handle initChain in handshake
			if err := css[i].blockExec.Store().Save(css[i].state); err != nil {
				t.Error(err)
			}

		}
	}
	// make connected switches and start all reactors
	p2p.MakeConnectedSwitches(config.P2P, n, func(i int, s *p2p.Switch) *p2p.Switch {
		s.AddReactor("CONSENSUS", reactors[i])
		s.SetLogger(reactors[i].cons.Logger.With("module", "p2p"))
		return s
	}, p2p.Connect2Switches)

	// now that everyone is connected,  start the state machines
	// If we started the state machines before everyone was connected,
	// we'd block when the cs fires NewBlockEvent and the peers are trying to start their reactors
	// TODO: is this still true with new pubsub?
	for i := 0; i < n; i++ {
		s := reactors[i].cons.GetState()
		reactors[i].SwitchToConsensus(s, false)
	}
	return reactors, blocksSubs, eventBuses
}

func stopConsensusNet(logger log.Logger, reactors []*Reactor, eventBuses []*types.EventBus) {
	logger.Info("stopConsensusNet", "n", len(reactors))
	for i, r := range reactors {
		logger.Info("stopConsensusNet: Stopping Reactor", "i", i)
		if err := r.Switch.Stop(); err != nil {
			logger.Error("error trying to stop switch", "error", err)
		}
	}
	for i, b := range eventBuses {
		logger.Info("stopConsensusNet: Stopping eventBus", "i", i)
		if err := b.Stop(); err != nil {
			logger.Error("error trying to stop eventbus", "error", err)
		}
	}
	logger.Info("stopConsensusNet: DONE", "n", len(reactors))
}

// Ensure a testnet makes blocks
func TestReactorBasic(t *testing.T) {
	N := 4
	css, cleanup := randConsensusNet(N, "consensus_reactor_test", newCounter)
	defer cleanup()
	reactors, blocksSubs, eventBuses := startConsensusNet(t, css, N)
	defer stopConsensusNet(log.TestingLogger(), reactors, eventBuses)
	timeoutWaitGroup(t, N, func(j int) {
		<-blocksSubs[j].Out()
	}, css)
}

// Ensure we can process blocks with evidence
func TestReactorWithEvidence(t *testing.T) {}

// Ensure a testnet makes blocks when there are txs
func TestReactorCreatesBlockWhenEmptyBlocksFalse(t *testing.T) {}

func timeoutWaitGroup(t *testing.T, n int, f func(int), css []*Consensus) {
	wg := new(sync.WaitGroup)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(j int) {
			f(j)
			wg.Done()
		}(i)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	// we're running many nodes in-process, possibly in in a virtual machine,
	// and spewing debug messages - making a block could take a while,
	timeout := time.Second * 20

	select {
	case <-done:
	case <-time.After(timeout):
		for i, cs := range css {
			t.Log("#################")
			t.Log("Validator", i)
			t.Log("CurView", cs.pacemaker.CurView())
			t.Log("")
		}
		os.Stdout.Write([]byte("pprof.Lookup('goroutine'):\n"))
		err := pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)
		require.NoError(t, err)
		capture()
		panic("Timed out waiting for all validators to commit a block")
	}
}

func capture() {
	trace := make([]byte, 10240000)
	count := runtime.Stack(trace, true)
	fmt.Printf("Stack of %d bytes: %s\n", count, trace)
}
