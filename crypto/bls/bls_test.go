package bls_test

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xufeisofly/hotstuff/crypto"
	"github.com/xufeisofly/hotstuff/crypto/bls"
)

type keyData struct {
	priv  string
	pub   string
	proof string
	addr  string
	msg   string
}

var blsDataTable = []keyData{
	{
		priv:  "6a80a269031cd7b90e4293a0559a91735a135ce96755f54ad83639f83e23b307",
		pub:   "8c235ea38f738c975c5e1bd5510bf01956e0bc35ab3fd81156749feed156d4990a617248011de944284689d063c79564",
		proof: "8c4c17fdf0a82871cd10df59d9095f641c0771b10e4075444fa560dd566d8494be763abb6957a2a596d7044fb15fb443145c78de75a9bddbf94bc07833b90a792a6bbd3a21c10a1c0cae03411603a6df9e7fe97dd8d6ab4a003b62b0a8ae7675",
		addr:  LOCAL_ADDRESS,
		msg:   "msg1",
	}, {
		priv:  "6a8d0ae5c85b6773a7918eb314c3375b5515bc5466fa3de86bfcc209fba52277",
		pub:   "a02db22fc5a47dc0cfd41d03b5ac3ed35f8dcd4b80da0c96fcb4756f0c08d8b899a1d6bca222e5eb4872ad77e20c5266",
		proof: "abc697047c50b5c8c18c03100e1275b9248c9dbbda66c97de4bb809e278f09be9919d77a38db8dd9379426802a22c24b00baa1d688c7f56c41122621cdcaad21e95b7cddc00567666691ff30ed10384959a22e7fbfd535d814b2df36f18b1890",
		addr:  LOCAL_ADDRESS2,
		msg:   "msg2",
	}, {
		priv:  "c2dcb22c4ba6e9d9c6e51e1f3149a5b097a408b39c88ca260e74a74a95cbcf",
		pub:   "8488a75ee88055a20f0ae84117586f2cc86b162d042f836370f9952121ebe50228f1d258ee75693e72f2f4325e2c0fa0",
		proof: "9529469fccd35c658e458d0719d55d9c1dd8b6696892391375d08c8fded6909ca0f2bc310b0afb1888720e8d621839b4135470272d70747617dc6c1c86b8e245fe40728fd3250cbc411e218d5bd4259d8100b613b8d09fa5226d38349ee422ae",
		addr:  LOCAL_ADDRESS3,
		msg:   "msg3",
	}, {
		priv:  "624ebf0d05ac07bb0144bddbb0e9e6bc2dc936027336112e2a22eeaa3b669d6d",
		pub:   "8f0e06fe7071c20382678c29d3a67e940cc9fffa928e61d0ba36ba8ab1c088218cce61b96b6a4d541ecc3a56ec5bd933",
		proof: "9906eba4fb56ace2f582d1202444da3d9b81669ab114a0b37e14e3508175abdacb264a715cab3009d95556bf4a19d47e1849894402991510e079ca13efa0df0092c5bc98b8fcc7d7948bd494e0108526e8b812230da5d7fccaabfcc619f8be89",
		addr:  LOCAL_ADDRESS4,
		msg:   "msg4",
	},
}

const (
	LOCAL_ADDRESS  = "1CKZ9Nx4zgds8tU7nJHotKSDr4a9bYJCa3"
	LOCAL_ADDRESS2 = "1CKZ9Nx4zgds8tU7nJHotKSDr4a9bYJCa4"
	LOCAL_ADDRESS3 = "1CKZ9Nx4zgds8tU7nJHotKSDr4a9bYJCa5"
	LOCAL_ADDRESS4 = "1CKZ9Nx4zgds8tU7nJHotKSDr4a9bYJCa6"
)

func getRandomKeypair() {
	b := bls.New(nil, nil, nil)
	fmt.Println(hex.EncodeToString(b.PriKey().Bytes()))
	fmt.Println(hex.EncodeToString(b.PubKey().Bytes()))
	fmt.Println(hex.EncodeToString(b.ProofOfPossession().Bytes()))
}

func GetBlsPkByAddr(addr crypto.Address) (crypto.PubKey, bool) {
	for _, d := range blsDataTable {
		if addr.Equal(crypto.Address(d.addr)) {
			blsPk, err := hex.DecodeString(d.pub)
			if err != nil {
				return nil, false
			}
			return bls.NewPubKey(blsPk), true
		}
	}
	return nil, false
}

func TestPubKeyBls(t *testing.T) {
	for _, d := range blsDataTable {
		sk, _ := hex.DecodeString(d.priv)
		pk, _ := hex.DecodeString(d.pub)

		priv := bls.NewPriKey(sk)
		assert.Equal(t, priv.Public().Bytes(), pk)
	}
}

func TestSingleSignAndVeriyBls(t *testing.T) {
	message := []byte("hello")

	for _, d := range blsDataTable {
		sk, _ := hex.DecodeString(d.priv)
		blsSk := bls.NewPriKey(sk)
		blsBase := bls.New(blsSk, GetBlsPkByAddr, crypto.Address(d.addr))
		sig, err := blsBase.Sign(message)
		require.Nil(t, err)
		ok := blsBase.Verify(sig, message)
		assert.True(t, ok)
	}
}

func TestAggregateSignAndVerifyBls(t *testing.T) {
	message := []byte("hello")
	var sigs []crypto.QuorumSignature
	for _, d := range blsDataTable {
		sk, _ := hex.DecodeString(d.priv)
		blsSk := bls.NewPriKey(sk)
		blsBase := bls.New(blsSk, GetBlsPkByAddr, crypto.Address(d.addr))
		sig, err := blsBase.Sign(message)
		require.Nil(t, err)
		sigs = append(sigs, sig)
	}

	blsBase := bls.New(nil, GetBlsPkByAddr, nil)
	aggSig, err := blsBase.Combine(sigs...)
	require.Nil(t, err)

	ok := blsBase.Verify(aggSig, message)
	assert.True(t, ok)
}

func TestAggregateSignAndBatchVerifyBls(t *testing.T) {
	batch := make(map[crypto.AddressStr][]byte)
	var sigs []crypto.QuorumSignature
	for _, d := range blsDataTable {
		sk, _ := hex.DecodeString(d.priv)
		blsSk := bls.NewPriKey(sk)
		blsBase := bls.New(blsSk, GetBlsPkByAddr, crypto.Address(d.addr))
		sig, err := blsBase.Sign([]byte(d.msg))
		require.Nil(t, err)

		sigs = append(sigs, sig)
		batch[d.addr] = []byte(d.msg)
	}

	blsBase := bls.New(nil, GetBlsPkByAddr, nil)
	aggSig, err := blsBase.Combine(sigs...)
	require.Nil(t, err)

	ok := blsBase.BatchVerify(aggSig, batch)
	assert.True(t, ok)
}

func TestPopProveAndVerify(t *testing.T) {
	for _, d := range blsDataTable {
		blsBase := bls.New(nil, nil, nil)
		pk, _ := hex.DecodeString(d.pub)
		blsPk := bls.NewPubKey(pk)
		proof, _ := hex.DecodeString(d.proof)
		ok := blsBase.PopVerify(blsPk, bls.NewProofOfPossession(proof))
		assert.True(t, ok)
	}
}
