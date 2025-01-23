package consensus

type CryptoBase interface {
	Sign(message []byte)
	Combine()
	Verify()
}

type Crypto interface {
	CryptoBase

	CreatePartialCert(block)
}
