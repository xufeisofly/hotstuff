package bls12

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
func (priv *PriKey) Public() crypto.PubKey {
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

// GeneratePrivateKey generates a new private key.
func GeneratePrivateKey() (*PriKey, error) {
	// the private key is uniformly random integer such that 0 <= pk < r
	pk, err := rand.Int(rand.Reader, curveOrder)
	if err != nil {
		return nil, fmt.Errorf("bls12: failed to generate private key: %w", err)
	}
	return &PriKey{
		p: pk,
	}, nil
}

// AggregateSignature is a bls12-381 aggregate signature. The participants field contains the IDs of the replicas that
// participated in signature creation. This allows us to build an aggregated public key to verify the signature.
type AggregateSignature struct {
	sig          bls12.PointG2
	participants []crypto.Address // The ids of the replicas who submitted signatures.
}

// ToBytes returns a byte representation of the aggregate signature.
func (agg *AggregateSignature) ToBytes() []byte {
	if agg == nil {
		return nil
	}
	b := bls12.NewG2().ToCompressed(&agg.sig)
	return b
}

// Participants returns the IDs of replicas who participated in the threshold signature.
func (agg AggregateSignature) Participants() []crypto.Address {
	return agg.participants
}

func firstParticipant(participants []crypto.Address) crypto.Address {
	if len(participants) == 0 {
		return crypto.Address{}
	}
	return participants[0]
}

type BlsKeyPair struct {
}

type bls12Base struct {
	mtx tmsync.RWMutex

	// local key pairs
	priKey PriKey
	pubKey PubKey
	proof  ProofOfPossession
}

func New() *bls12Base {
	return &bls12Base{}
}

func (bls *bls12Base) ResetKeyPair() {
	// the private key is uniformly random integer such that 0 <= pk < r
	sk, err := rand.Int(rand.Reader, curveOrder)
	if err != nil {
		panic(err)
	}
	bls.priKey = PriKey{p: sk}
	bls.pubKey = bls.priKey.Public().(PubKey)
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
func (bls *bls12Base) Sign(message []byte) (*bls12.PointG2, error) {
	p, err := bls.coreSign(message, domain)
	if err != nil {
		return nil, fmt.Errorf("bls12: coreSign failed: %w", err)
	}
	return p, nil
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
func (bls *bls12Base) Combine(signatures ...*bls12.PointG2) (combined *bls12.PointG2, err error) {
	if len(signatures) < 2 {
		return nil, fmt.Errorf("bls12: combine failed, sigs num < 2")
	}
	g2 := bls12.NewG2()
	aggG2 := bls12.PointG2{}
	for _, sig := range signatures {
		g2.Add(&aggG2, &aggG2, sig)
	}
	return &aggG2, nil
}

func (bls *bls12Base) AggregateVerify(publicKeys []*PubKey, messages [][]byte, signature *bls12.PointG2) bool {
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

func (bls *bls12Base) FastAggregateVerify(publicKeys []*PubKey, message []byte, signature *bls12.PointG2) bool {
	engine := bls12.NewEngine()
	var aggregate bls12.PointG1
	for _, pk := range publicKeys {
		engine.G1.Add(&aggregate, &aggregate, pk.p)
	}
	return bls.coreVerify(&PubKey{p: &aggregate}, message, signature, domain)
}

func (bls *bls12Base) Verify(pubKey *PubKey, message []byte, signature *bls12.PointG2) bool {
	return bls.coreVerify(pubKey, message, signature, domain)
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
	pubKey := bls.priKey.Public().(PubKey)
	proof, err := bls.coreSign(pubKey.Bytes(), domainPOP)
	if err != nil {
		panic(err)
	}
	return ProofOfPossession{p: proof}
}

func (bls *bls12Base) PopVerify(pubKey *PubKey, proof ProofOfPossession) bool {
	return bls.coreVerify(pubKey, pubKey.Bytes(), proof.p, domainPOP)
}

//-------------------

// Verify verifies the given quorum signature against the message.
func (bls *bls12Base) Verify(signature QuorumSignature, message []byte) bool {
	s, ok := signature.(*AggregateSignature)
	if !ok {
		panic(fmt.Sprintf("cannot verify signature of incompatible type %T (expected %T)", signature, s))
	}

	n := s.Participants().Len()

	if n == 1 {
		id := firstParticipant(s.Participants())
		pk, ok := bls.publicKey(id)
		if !ok {
			bls.logger.Warnf("Missing public key for ID %d", id)
			return false
		}
		return bls.coreVerify(pk, message, &s.sig, domain)
	}

	// else if l > 1:
	pks := make([]*PubKey, 0, n)
	s.Participants().RangeWhile(func(id uint32) bool {
		pk, ok := bls.publicKey(id)
		if ok {
			pks = append(pks, pk)
			return true
		}
		return false
	})
	if len(pks) != n {
		return false
	}
	return bls.fastAggregateVerify(pks, message, &s.sig)
}

// BatchVerify verifies the given quorum signature against the batch of messages.
func (bls *bls12Base) BatchVerify(signature QuorumSignature, batch map[uint32][]byte) bool {
	s, ok := signature.(*AggregateSignature)
	if !ok {
		panic(fmt.Sprintf("cannot verify incompatible signature type %T (expected %T)", signature, s))
	}

	if s.Participants().Len() != len(batch) {
		return false
	}

	pks := make([]*PubKey, 0, len(batch))
	msgs := make([][]byte, 0, len(batch))

	for id, msg := range batch {
		msgs = append(msgs, msg)
		pk, ok := bls.publicKey(id)
		if !ok {
			return false
		}
		pks = append(pks, pk)
	}

	if len(batch) == 1 {
		return bls.coreVerify(pks[0], msgs[0], &s.sig, domain)
	}

	return bls.aggregateVerify(pks, msgs, &s.sig)
}
