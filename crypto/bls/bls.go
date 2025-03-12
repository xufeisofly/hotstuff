package bls

import (
	"crypto/rand"
	"fmt"

	"math/big"

	bls12 "github.com/kilic/bls12-381"
	"github.com/xufeisofly/hotstuff/crypto"
	tmsync "github.com/xufeisofly/hotstuff/libs/sync"
)

var (
	domain    = []byte("BLS_SIG_BLS12381G2_XMD:SHA-256_SSWU_RO_POP_")
	domainPOP = []byte("BLS_POP_BLS12381G2_XMD:SHA-256_SSWU_RO_POP_")

	curveOrder, _ = new(big.Int).SetString("73eda753299d7d483339d80809a1d80553bda402fffe5bfeffffffff00000001", 16)
)

type PubKey struct {
	p *bls12.PointG1
}

// ToBytes marshals the public key to a byte slice.
func (pub PubKey) Bytes() []byte {
	return bls12.NewG1().ToCompressed(pub.p)
}

func (pub PubKey) Address() crypto.Address {
	return nil
}

func (pub PubKey) VerifySignature(msg []byte, sig []byte) bool {
	return false
}

func (pub PubKey) Equals(pub2 crypto.PubKey) bool {
	return false
}

func (pub PubKey) Type() string {
	return "bls12"
}

// PriKey is a bls12-381 private key.
type PriKey struct {
	p *big.Int
}

// ToBytes marshals the private key to a byte slice.
func (priv PriKey) Bytes() []byte {
	return priv.p.Bytes()
}

// Public returns the public key associated with this private key.
func (priv *PriKey) Public() PubKey {
	p := &bls12.PointG1{}
	// The public key is the secret key multiplied by the generator G1
	return PubKey{p: bls12.NewG1().MulScalarBig(p, &bls12.G1One, priv.p)}
}

type ProofOfPossession struct {
	p *bls12.PointG2
}

func (proof ProofOfPossession) Bytes() []byte {
	return bls12.NewG2().ToCompressed(proof.p)
}

type bls12Base struct {
	mtx tmsync.RWMutex

	// local key pairs
	priKey PriKey
	pubKey PubKey
	proof  ProofOfPossession

	privValidatorPubKey crypto.PubKey
	pubKeyFn            GetPubKeyFn
}

type GetPubKeyFn func(crypto.Address) (crypto.PubKey, bool)

func New(pubKeyFn GetPubKeyFn) *bls12Base {
	b := &bls12Base{
		pubKeyFn: pubKeyFn,
	}
	b.InitKeyPair()
	return b
}

func (bls *bls12Base) InitKeyPair() {
	// the private key is uniformly random integer such that 0 <= pk < r
	sk, err := rand.Int(rand.Reader, curveOrder)
	if err != nil {
		panic(err)
	}
	bls.priKey = PriKey{p: sk}
	bls.pubKey = bls.priKey.Public()
	bls.proof = bls.PopProve()
}

func (bls *bls12Base) PriKey() PriKey {
	return bls.priKey
}

func (bls *bls12Base) PubKey() PubKey {
	return bls.pubKey
}

func (bls *bls12Base) ProofOfPossession() ProofOfPossession {
	return bls.proof
}

// Sign creates a cryptographic signature of the given messsage.
func (bls *bls12Base) Sign(message []byte) (crypto.QuorumSignature, error) {
	p, err := bls.coreSign(message, domain)
	if err != nil {
		return nil, fmt.Errorf("bls12: coreSign failed: %w", err)
	}
	addrSet := crypto.NewAddressSet()
	addrSet.Add(bls.privValidatorPubKey.Address())
	return &AggregateSignature{point: *p}, nil
}

func (bls *bls12Base) coreSign(message []byte, domainTag []byte) (*bls12.PointG2, error) {
	sk := bls.priKey
	g2 := bls12.NewG2()
	point, err := g2.HashToCurve(message, domainTag)
	if err != nil {
		return nil, err
	}
	// multiply the point by the secret key, storing the result in the same point variable
	g2.MulScalarBig(point, point, sk.p)
	return point, nil
}

// Combine combines multiple signatures into a single signature.
func (bls *bls12Base) Combine(signatures ...crypto.QuorumSignature) (combined crypto.QuorumSignature, err error) {
	if len(signatures) < 2 {
		return nil, fmt.Errorf("bls12: combine failed, len(signatures) < 2")
	}

	g2 := bls12.NewG2()
	agg := bls12.PointG2{}
	participants := crypto.NewAddressSet()
	for _, sig1 := range signatures {
		if sig2, ok := sig1.(*AggregateSignature); ok {
			sig2.participants.RangeWhile(func(addr crypto.Address) bool {
				if participants.Contains(addr) {
					err = fmt.Errorf("bls: combine failed, participants not contain addr: %s", string(addr))
					return false
				}
				participants.Add(addr)
				return true
			})
			if err != nil {
				return nil, err
			}
			g2.Add(&agg, &agg, &sig2.point)
		} else {
			panic(fmt.Sprintf("cannot combine incompatible signature type %T (expected %T)", sig1, sig2))
		}
	}
	return &AggregateSignature{point: agg, participants: participants}, nil
}

// Verify verifies the given quorum signature against the message.
func (bls *bls12Base) Verify(signature crypto.QuorumSignature, message []byte) bool {
	s, ok := signature.(*AggregateSignature)
	if !ok {
		panic(fmt.Sprintf("cannot verify signature of incompatible type %T (expected %T)", signature, s))
	}

	n := s.Participants().Len()

	if n == 1 {
		addr := s.Participants().First()
		pk, ok := bls.pubKeyFn(addr)
		if !ok {
			return false
		}
		return bls.coreVerify(pk.(*PubKey), message, &s.point, domain)
	}

	// else if l > 1:
	pks := make([]*PubKey, 0, n)
	s.Participants().RangeWhile(func(addr crypto.Address) bool {
		pk, ok := bls.pubKeyFn(addr)
		if ok {
			pks = append(pks, pk.(*PubKey))
			return true
		}
		return false
	})
	if len(pks) != n {
		return false
	}
	return bls.fastAggregateVerify(pks, message, &s.point)
}

// BatchVerify verifies the given quorum signature against the batch of messages.
func (bls *bls12Base) BatchVerify(signature crypto.QuorumSignature, batch map[crypto.AddressStr][]byte) bool {
	s, ok := signature.(*AggregateSignature)
	if !ok {
		panic(fmt.Sprintf("cannot verify incompatible signature type %T (expected %T)", signature, s))
	}

	if s.Participants().Len() != len(batch) {
		return false
	}

	pks := make([]*PubKey, 0, len(batch))
	msgs := make([][]byte, 0, len(batch))

	for addrStr, msg := range batch {
		msgs = append(msgs, msg)
		pk, ok := bls.pubKeyFn(crypto.Address(addrStr))
		if !ok {
			return false
		}
		pks = append(pks, pk.(*PubKey))
	}

	if len(batch) == 1 {
		return bls.coreVerify(pks[0], msgs[0], &s.point, domain)
	}

	return bls.aggregateVerify(pks, msgs, &s.point)
}

func (bls *bls12Base) aggregateVerify(publicKeys []*PubKey, messages [][]byte, signature *bls12.PointG2) bool {
	set := make(map[string]struct{})
	for _, m := range messages {
		set[string(m)] = struct{}{}
	}

	return len(messages) == len(set) && bls.coreAggregateVerify(publicKeys, messages, signature)
}

func (bls *bls12Base) coreAggregateVerify(publicKeys []*PubKey, messages [][]byte, signature *bls12.PointG2) bool {
	n := len(publicKeys)
	// validate input
	if n != len(messages) {
		return false
	}

	// precondition n >= 1
	if n < 1 {
		return false
	}

	if !bls.subgroupCheck(signature) {
		return false
	}

	engine := bls12.NewEngine()

	for i := 0; i < n; i++ {
		q, err := engine.G2.HashToCurve(messages[i], domain)
		if err != nil {
			return false
		}
		engine.AddPair(publicKeys[i].p, q)
	}

	engine.AddPairInv(&bls12.G1One, signature)
	return engine.Result().IsOne()
}

func (bls *bls12Base) fastAggregateVerify(publicKeys []*PubKey, message []byte, signature *bls12.PointG2) bool {
	engine := bls12.NewEngine()
	var aggregate bls12.PointG1
	for _, pk := range publicKeys {
		engine.G1.Add(&aggregate, &aggregate, pk.p)
	}
	return bls.coreVerify(&PubKey{p: &aggregate}, message, signature, domain)
}

func (bls *bls12Base) coreVerify(pubKey *PubKey, message []byte, signature *bls12.PointG2, domainTag []byte) bool {
	if !bls.subgroupCheck(signature) {
		return false
	}
	g2 := bls12.NewG2()
	messagePoint, err := g2.HashToCurve(message, domainTag)
	if err != nil {
		return false
	}
	engine := bls12.NewEngine()
	engine.AddPairInv(&bls12.G1One, signature)
	engine.AddPair(pubKey.p, messagePoint)
	return engine.Result().IsOne()
}

func (bls *bls12Base) subgroupCheck(point *bls12.PointG2) bool {
	var p bls12.PointG2
	g2 := bls12.NewG2()
	g2.MulScalarBig(&p, point, curveOrder)
	return g2.IsZero(&p)
}

func (bls *bls12Base) PopProve() ProofOfPossession {
	pubKey := bls.priKey.Public()
	proof, err := bls.coreSign(pubKey.Bytes(), domainPOP)
	if err != nil {
		panic(err)
	}
	return ProofOfPossession{p: proof}
}

func (bls *bls12Base) PopVerify(pubKey *PubKey, proof *ProofOfPossession) bool {
	return bls.coreVerify(pubKey, pubKey.Bytes(), proof.p, domainPOP)
}
