package consensus

import (
	typesproto "github.com/xufeisofly/hotstuff/proto/hotstuff/types"
	"github.com/xufeisofly/hotstuff/types"
)

// SyncInfo holds the highest known QC or TC.
// Generally, if highQC.View > highTC.View, there is no need to include highTC in the SyncInfo.
// However, if highQC.View < highTC.View, we should still include highQC.
// This can also hold an AggregateQC for Fast-Hotstuff.
type SyncInfo struct {
	qc    *types.QuorumCert
	tc    *types.TimeoutCert
	aggQC *types.AggregateQC
}

// NewSyncInfo returns a new SyncInfo struct.
func NewSyncInfo() SyncInfo {
	return SyncInfo{}
}

// WithQC returns a copy of the SyncInfo struct with the given QC.
func (si SyncInfo) WithQC(qc types.QuorumCert) SyncInfo {
	si.qc = new(types.QuorumCert)
	*si.qc = qc
	return si
}

// WithTC returns a copy of the SyncInfo struct with the given TC.
func (si SyncInfo) WithTC(tc types.TimeoutCert) SyncInfo {
	si.tc = new(types.TimeoutCert)
	*si.tc = tc
	return si
}

// WithAggQC returns a copy of the SyncInfo struct with the given AggregateQC.
func (si SyncInfo) WithAggQC(aggQC types.AggregateQC) SyncInfo {
	si.aggQC = new(types.AggregateQC)
	*si.aggQC = aggQC
	return si
}

func (si SyncInfo) QC() *types.QuorumCert {
	return si.qc
}

func (si SyncInfo) TC() *types.TimeoutCert {
	return si.tc
}

func (si SyncInfo) AggQC() *types.AggregateQC {
	return si.aggQC
}

func (si SyncInfo) ToProto() *typesproto.SyncInfo {
	return nil
}
