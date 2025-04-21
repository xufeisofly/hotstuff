package consensus

import (
	"sync"

	tcrypto "github.com/xufeisofly/hotstuff/crypto"
	"github.com/xufeisofly/hotstuff/libs/log"
	"github.com/xufeisofly/hotstuff/types"
)

type CryptoBase interface {
	Sign(message []byte) (tcrypto.QuorumSignature, error)
	Combine(signatures ...tcrypto.QuorumSignature) (tcrypto.QuorumSignature, error)
	Verify(signature tcrypto.QuorumSignature, message []byte) bool
	BatchVerify(signature tcrypto.QuorumSignature, batch map[types.AddressStr][]byte) bool
}

type Crypto interface {
	CryptoBase

	CollectPartialSignature(
		view types.View,
		msgHash []byte,
		partSig tcrypto.QuorumSignature,
	) (aggSig tcrypto.QuorumSignature, ok bool)

	VerifyQuorumCert(qc types.QuorumCert) bool
	VerifyTimeoutCert(tc types.TimeoutCert) bool
	VerifyAggregateQC(aggQC types.AggregateQC) (highQC types.QuorumCert, ok bool)
}

type crypto struct {
	CryptoBase

	logger    log.Logger
	epochInfo epochInfo
	// partial signatures collection for one view
	sigCollect *sigCollect
}

var _ Crypto = (*crypto)(nil)

func NewCrypto(cryptoBase CryptoBase) Crypto {
	return &crypto{
		CryptoBase: cryptoBase,
	}
}

func (c *crypto) SetEpochInfo(e epochInfo) {
	c.epochInfo = e
}

func (c *crypto) SetLogger(l log.Logger) {
	c.logger = l
}

func (c *crypto) CollectPartialSignature(
	view types.View,
	msgHash []byte,
	partSig tcrypto.QuorumSignature,
) (aggSig tcrypto.QuorumSignature, ok bool) {
	if ok := c.Verify(partSig, msgHash); !ok {
		return nil, false
	}

	// handling an old view vote
	if c.sigCollect != nil && c.sigCollect.view > view {
		return nil, false
	}

	// handling a new view vote
	if c.sigCollect == nil || c.sigCollect.view < view {
		c.sigCollect = getSigCollect()
		c.sigCollect.setView(view)
	}

	// handling a reduntant vote
	if c.sigCollect.handled {
		item := c.sigCollect.getItem(msgHash)
		if item.aggSig != nil && item.aggSig.IsValid() {
			return item.aggSig, true
		}
		// If aggSig is invalid, reset sigCollect to be unhandled
		c.sigCollect.setHandled(false)
	}

	if !partSig.IsValid() {
		return nil, false
	}

	addr := partSig.Participants().First()
	item := c.sigCollect.getItem(msgHash)
	item.addPartialSig(addr, partSig)

	// return if valid voting power is not enough
	if item.validVotingPower() < c.epochInfo.QuorumVotingPower() {
		return nil, false
	}

	// combine partial signatures to an aggregated signature
	partSigs := make([]tcrypto.QuorumSignature, 0, len(item.partSigs))
	for _, partSig := range item.partSigs {
		partSigs = append(partSigs, partSig)
	}
	aggSig, err := c.Combine(partSigs...)
	if err != nil {
		panic(err)
	}

	item.setAggSig(aggSig)
	c.sigCollect.setHandled(true)

	return aggSig, true
}

func (c *crypto) VerifyQuorumCert(qc types.QuorumCert) bool {
	if qc.Signature().Participants().Len() < int(c.epochInfo.QuorumVotingPower()) {
		return false
	}
	return c.Verify(qc.Signature(), qc.BlockID().Hash)
}

func (c *crypto) VerifyTimeoutCert(tc types.TimeoutCert) bool {
	return false
}

func (c *crypto) VerifyAggregateQC(aggQC types.AggregateQC) (highQC types.QuorumCert, ok bool) {
	return types.QuorumCert{}, false
}

// partial signature collection for one view
type sigCollect struct {
	view     types.View
	collects map[string]*sigCollectItem
	handled  bool
}

func (sc *sigCollect) setView(view types.View) {
	sc.view = view
}

func (sc *sigCollect) setHandled(handled bool) {
	sc.handled = handled
}

func (sc *sigCollect) addItem(item *sigCollectItem) {
	sc.collects[item.msgHash.String()] = item
}

func (sc *sigCollect) getItem(msgHash types.Hash) *sigCollectItem {
	item, ok := sc.collects[msgHash.String()]
	if ok {
		return item
	}

	item = newSigCollectItem(msgHash)
	sc.addItem(item)
	return item
}

// partial sigatures collection for one msg hash in a view
type sigCollectItem struct {
	msgHash  types.Hash
	partSigs map[types.AddressStr]tcrypto.QuorumSignature
	aggSig   tcrypto.QuorumSignature
}

func newSigCollectItem(msgHash types.Hash) *sigCollectItem {
	return &sigCollectItem{
		msgHash:  msgHash,
		partSigs: make(map[types.AddressStr]tcrypto.QuorumSignature),
		aggSig:   nil,
	}
}

func (scItem *sigCollectItem) addPartialSig(addr types.Address, partSig tcrypto.QuorumSignature) {
	scItem.partSigs[types.AddressStr(addr)] = partSig
}

// valid partial signature count
func (scItem *sigCollectItem) validVotingPower() int64 {
	// TODO calculate voting power
	return int64(len(scItem.partSigs))
}

func (scItem *sigCollectItem) setAggSig(aggSig tcrypto.QuorumSignature) {
	scItem.aggSig = aggSig
}

// sigCollectPool avoids frequent memory allocation
var sigCollectPool = sync.Pool{
	New: func() interface{} {
		return &sigCollect{
			collects: make(map[string]*sigCollectItem),
			handled:  false,
			view:     0,
		}
	},
}

func getSigCollect() *sigCollect {
	return sigCollectPool.Get().(*sigCollect)
}

func putSigCollect(sc *sigCollect) {
	sc.collects = make(map[string]*sigCollectItem)
	sc.handled = false
	sc.view = 0
	sigCollectPool.Put(sc)
}
