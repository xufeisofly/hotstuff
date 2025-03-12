package crypto

import (
	"github.com/xufeisofly/hotstuff/crypto/tmhash"
	"github.com/xufeisofly/hotstuff/libs/bytes"
)

const (
	// AddressSize is the size of a pubkey address.
	AddressSize = tmhash.TruncatedSize
)

// An address is a []byte, but hex-encoded even in JSON.
// []byte leaves us the option to change the address length.
// Use an alias so Unmarshal methods (with ptr receivers) are available too.
type Address = bytes.HexBytes
type AddressStr = string

func AddressHash(bz []byte) Address {
	return Address(tmhash.SumTruncated(bz))
}

type PubKey interface {
	Address() Address
	Bytes() []byte
	VerifySignature(msg []byte, sig []byte) bool
	Equals(PubKey) bool
	Type() string
}

type PrivKey interface {
	Bytes() []byte
	Sign(msg []byte) ([]byte, error)
	PubKey() PubKey
	Equals(PrivKey) bool
	Type() string
}

type Symmetric interface {
	Keygen() []byte
	Encrypt(plaintext []byte, secret []byte) (ciphertext []byte)
	Decrypt(ciphertext []byte, secret []byte) (plaintext []byte, err error)
}

type AddressSet map[string]struct{}

func NewAddressSet() AddressSet {
	return make(AddressSet)
}

func (addrSet AddressSet) Add(addr Address) {
	addrSet[string(addr)] = struct{}{}
}

func (addrSet AddressSet) Len() int {
	return len(addrSet)
}

func (addrSet AddressSet) Contains(addr Address) bool {
	_, ok := addrSet[string(addr)]
	return ok
}

// RangeWhile calls f for each addr in the set until f returns false.
func (addrSet AddressSet) RangeWhile(f func(Address) bool) {
	for addrStr := range addrSet {
		if !f(Address(addrStr)) {
			return
		}
	}
}

func (addrSet AddressSet) ForEach(f func(Address)) {
	addrSet.RangeWhile(func(addr Address) bool {
		f(addr)
		return true
	})
}

func (addrSet AddressSet) First() Address {
	if len(addrSet) == 0 {
		return Address{}
	}

	var ret Address
	addrSet.RangeWhile(func(addr Address) bool {
		ret = addr
		return false
	})
	return ret
}

func (addrSet AddressSet) ToBytes() []byte {
	var result []byte
	for address := range addrSet {
		// Assuming UTF-8 encoding for the address string.
		addressBytes := []byte(address)
		result = append(result, addressBytes...)
	}
	return result
}

func AddressSetFromBytes(data []byte) (AddressSet, error) {
	addressSet := make(AddressSet)
	for len(data) > 0 {
		addressLength := len(data)
		address := string(data[:addressLength])
		addressSet[address] = struct{}{}
		data = data[addressLength:]
	}
	return addressSet, nil
}

type QuorumSignature interface {
	ToBytes() []byte
	// Participants returns the IDs of replicas who participated in the threshold signature.
	Participants() AddressSet
	IsValid() bool
}
