package consensus

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"

	"github.com/xufeisofly/hotstuff/types"
)

type LeaderElect interface {
	GetLeader() *types.Validator
}

type leaderElect struct {
	blockchain Blockchain
	epochInfo  *epochInfo
}

func (l *leaderElect) GetLeader() *types.Validator {
	qc := QuorumCertBeforeGenesis()
	committedBlock := l.blockchain.LatestCommittedBlock()
	if committedBlock != nil {
		qc = *committedBlock.SelfCommit.CommitQC
	}

	var buf bytes.Buffer
	buf.Write(qc.Signature().ToBytes())
	hash := sha256.Sum256(buf.Bytes())

	var result int
	buf2 := bytes.NewReader(hash[:])
	binary.Read(buf2, binary.BigEndian, &result)

	leaderIdx := result % len(l.epochInfo.Validators().Validators)
	_, val := l.epochInfo.Validators().GetByIndex(int32(leaderIdx))
	return val
}
