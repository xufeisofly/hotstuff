package types

type QuorumCert struct {
	BlockData *BlockData
}

type BlockData struct {
	Height int64
}
