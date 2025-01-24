package consensus

import (
	"github.com/xufeisofly/hotstuff/libs/log"
	"github.com/xufeisofly/hotstuff/types"
)

type CryptoBase interface {
	Sign(message []byte) (types.QuorumSignature, error)
	Combine(signatures ...types.QuorumSignature) (types.QuorumSignature, error)
	Verify(signature types.QuorumSignature, message []byte) bool
	BatchVerify(signature types.QuorumSignature, batch map[types.AddressStr][]byte) bool
}

type Crypto interface {
	CryptoBase

	CreatePartialCert(block *types.Block) (PartialCert, error)
	CreateQuorumCert(block *types.Block, partialCerts []PartialCert) (QuorumCert, error)
	CreateTimeoutCert(view types.View, timeouts []TimeoutMsg) (TimeoutCert, error)
	CreateAggregateQC(view types.View, timeouts []TimeoutMsg) (AggregateQC, error)

	VerifyPartialCert(cert PartialCert) bool
	VerifyQuorumCert(qc QuorumCert) bool
	VerifyTimeoutCert(tc TimeoutCert) bool
	VerifyAggregateQC(aggQC AggregateQC) (highQC QuorumCert, ok bool)
}

type crypto struct {
	CryptoBase

	logger    log.Logger
	epochInfo epochInfo
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

func (c *crypto) CreatePartialCert(block *types.Block) (PartialCert, error) {
	blockHash := block.Hash()
	sig, err := c.Sign(blockHash)
	if err != nil {
		return PartialCert{}, err
	}
	return NewPartialCert(sig, blockHash), nil
}

func (c *crypto) CreateQuorumCert(block *types.Block, partialCerts []PartialCert) (QuorumCert, error) {
	sigs := make([]types.QuorumSignature, 0, len(partialCerts))
	for _, partialCert := range partialCerts {
		sigs = append(sigs, partialCert.Signature())
	}
	sig, err := c.Combine(sigs...)
	if err != nil {
		return QuorumCert{}, err
	}
	return NewQuorumCert(sig, block.View, block.Hash()), nil
}

func (c *crypto) CreateTimeoutCert(view types.View, timeouts []TimeoutMsg) (TimeoutCert, error) {
	return TimeoutCert{}, nil
}

func (c *crypto) CreateAggregateQC(view types.View, timeouts []TimeoutMsg) (AggregateQC, error) {
	return AggregateQC{}, nil
}

func (c *crypto) VerifyPartialCert(cert PartialCert) bool {
	return c.Verify(cert.Signature(), cert.BlockHash())
}

func (c *crypto) VerifyQuorumCert(qc QuorumCert) bool {
	if qc.Signature().Participants().Len() < int(c.epochInfo.QuorumVotingPower()) {
		return false
	}
	return c.Verify(qc.Signature(), qc.BlockHash())
}

func (c *crypto) VerifyTimeoutCert(tc TimeoutCert) bool {
	return false
}

func (c *crypto) VerifyAggregateQC(aggQC AggregateQC) (highQC QuorumCert, ok bool) {
	return QuorumCert{}, false
}
