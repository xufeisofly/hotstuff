package consensus

import "github.com/xufeisofly/hotstuff/types"

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
