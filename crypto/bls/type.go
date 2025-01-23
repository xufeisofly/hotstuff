package bls

import (
	bls12 "github.com/kilic/bls12-381"
	"github.com/xufeisofly/hotstuff/types"
)

// AggregateSignature is a bls12-381 aggregate signature. The participants field contains the IDs of the replicas that
// participated in signature creation. This allows us to build an aggregated public key to verify the signature.
type AggregateSignature struct {
	point        bls12.PointG2
	participants types.AddressSet
}

var _ types.QuorumSignature = (*AggregateSignature)(nil)

// ToBytes returns a byte representation of the aggregate signature.
func (agg *AggregateSignature) ToBytes() []byte {
	if agg == nil {
		return nil
	}
	return bls12.NewG2().ToCompressed(&agg.point)
}

// Participants returns the IDs of replicas who participated in the threshold signature.
func (agg *AggregateSignature) Participants() types.AddressSet {
	return agg.participants
}
