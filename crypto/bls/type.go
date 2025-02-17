package bls

import (
	"fmt"

	bls12 "github.com/kilic/bls12-381"
	"github.com/xufeisofly/hotstuff/types"
)

// AggregateSignature is a bls12-381 aggregate signature. The participants field contains the IDs of the replicas that
// participated in signature creation. This allows us to build an aggregated public key to verify the signature.
type AggregateSignature struct {
	point        bls12.PointG2
	participants types.AddressSet
}

var EmptyAggregateSignature = AggregateSignature{
	point: *bls12.NewG2().Zero(),
}

var _ types.QuorumSignature = (*AggregateSignature)(nil)

// ToBytes returns a byte representation of the aggregate signature.
func (agg *AggregateSignature) ToBytes() []byte {
	if agg == nil {
		return nil
	}
	// Serialize the point
	pointBytes := bls12.NewG2().ToCompressed(&agg.point)
	// Serialize the participants (AddressSet)
	participantsBytes := agg.participants.ToBytes()
	// Concatenate the two byte slices (point and participants)
	return append(pointBytes, participantsBytes...)
}

// Participants returns the IDs of replicas who participated in the threshold signature.
func (agg *AggregateSignature) Participants() types.AddressSet {
	return agg.participants
}

func (agg *AggregateSignature) IsValid() bool {
	return agg != nil && len(agg.participants) > 0
}

func AggregateSignatureFromBytes(data []byte) (*AggregateSignature, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("data is empty")
	}

	// Deserialize the point (assuming point size is fixed, use the point size from your library)
	pointSize := len(bls12.NewG2().ToCompressed(bls12.NewG2().Zero())) // Or a fixed length if defined elsewhere
	if len(data) < pointSize {
		return nil, fmt.Errorf("invalid data length for point")
	}

	// Extract point bytes and deserialize
	pointBytes := data[:pointSize]
	point, err := bls12.NewG2().FromCompressed(pointBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize point: %w", err)
	}

	// Deserialize the participants from the remaining data
	participantsBytes := data[pointSize:]
	participants, err := types.AddressSetFromBytes(participantsBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize participants: %w", err)
	}

	// Return the AggregateSignature object
	return &AggregateSignature{
		point:        *point,
		participants: participants,
	}, nil
}
