package consensus

import (
	cfg "github.com/xufeisofly/hotstuff/config"
	hscrypto "github.com/xufeisofly/hotstuff/crypto"
	"github.com/xufeisofly/hotstuff/libs/service"
	tmsync "github.com/xufeisofly/hotstuff/libs/sync"
	"github.com/xufeisofly/hotstuff/p2p"
	sm "github.com/xufeisofly/hotstuff/state"
	"github.com/xufeisofly/hotstuff/types"
)

type msgInfo struct {
	Msg    Message `json:"msg"`
	PeerID p2p.ID  `json:"peer_key"`
}

type txNotifier interface {
	TxsAvailable() <-chan struct{}
}

type Consensus struct {
	service.BaseService

	// config details
	config        *cfg.ConsensusConfig
	privValidator types.PrivValidator // for signing votes

	// store blocks and commits
	blockStore sm.BlockStore

	// create and execute blocks
	blockExec *sm.BlockExecutor

	// notify us if txs are available
	txNotifier txNotifier

	// internal state
	mtx   tmsync.Mutex
	state sm.State // State until height-1.
	// privValidator pubkey, memoized for the duration of one block
	// to avoid extra requests to HSM
	privValidatorPubKey hscrypto.PubKey

	// state changes may be triggered by: msgs from peers,
	// msgs from ourself, or by timeouts
	peerMsgQueue     chan msgInfo
	internalMsgQueue chan msgInfo
	timeoutTicker    TimeoutTicker

	Validators *types.ValidatorSet
}
