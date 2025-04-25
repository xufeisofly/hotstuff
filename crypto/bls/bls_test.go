package bls_test

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xufeisofly/hotstuff/crypto"
	"github.com/xufeisofly/hotstuff/crypto/bls"
)

type keyData struct {
	priv string
	pub  string
	addr string
}

var blsDataTable = []keyData{
	{
		priv: "6a80a269031cd7b90e4293a0559a91735a135ce96755f54ad83639f83e23b307",
		pub:  "8c235ea38f738c975c5e1bd5510bf01956e0bc35ab3fd81156749feed156d4990a617248011de944284689d063c79564",
	},
}

var addr2BlsPkMap = map[string]string{
	"secAddr...": "8c235ea38f738c975c5e1bd5510bf01956e0bc35ab3fd81156749feed156d4990a617248011de944284689d063c79564",
}

func GetBlsPkByAddr(addr crypto.Address) (crypto.PubKey, bool) {

}

func TestPubKeyBls(t *testing.T) {
	for _, d := range blsDataTable {
		sk, _ := hex.DecodeString(d.priv)
		pk, _ := hex.DecodeString(d.pub)

		priv := bls.NewPriKey(sk)
		assert.Equal(t, priv.Public().Bytes(), pk)
	}
}

func TestSignAndVeriyBls(t *testing.T) {
	blsBase := bls.New(nil)
	message := []byte("hello")
	sig, err := blsBase.Sign(message)
	require.Nil(t, err)

	ok := blsBase.Verify(sig, message)
}
