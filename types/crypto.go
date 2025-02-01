package types

type QuorumSignature interface {
	ToBytes() []byte
	// Participants returns the IDs of replicas who participated in the threshold signature.
	Participants() AddressSet
	IsValid() bool
}
