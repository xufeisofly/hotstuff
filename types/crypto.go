package types

import (
	"fmt"
	"io"
	"strings"
)

type QuorumSignature interface {
	ToBytes() []byte
	// Participants returns the IDs of replicas who participated in the threshold signature.
	Participants() AddressSet
	IsValid() bool
}

// QuorumCert(QC) is certificate for Block created by a quorum of partial certificates.
type QuorumCert struct {
	signature QuorumSignature
	view      View
	blockID   BlockID
}

func NewQuorumCert(signature QuorumSignature, view View, blockID BlockID) QuorumCert {
	return QuorumCert{signature, view, blockID}
}

func (qc QuorumCert) Signature() QuorumSignature {
	return qc.signature
}

func (qc QuorumCert) View() View {
	return qc.view
}

func (qc QuorumCert) BlockID() BlockID {
	return qc.blockID
}

func (qc QuorumCert) String() string {
	var sb strings.Builder
	if qc.signature != nil {
		_ = writeParticipants(&sb, qc.Signature().Participants())
	}
	return fmt.Sprintf("QC{ hash: %.6s, Addrs: [ %s] }", qc.blockID.Hash, &sb)
}

// AggregateQC is a set of QCs extracted from timeout messages
// and an aggregate signature of the timeout signatures.
// This is used by the Fast-HotStuff consensus protocol.
type AggregateQC struct {
	qcs       map[AddressStr]QuorumCert
	signature QuorumSignature
	view      View
}

func NewAggregateQC(
	qcs map[AddressStr]QuorumCert,
	signature QuorumSignature,
	view View,
) AggregateQC {
	return AggregateQC{qcs, signature, view}
}

func (aggQC AggregateQC) QCs() map[AddressStr]QuorumCert {
	return aggQC.qcs
}

func (aggQC AggregateQC) Signature() QuorumSignature {
	return aggQC.signature
}

func (aggQC AggregateQC) View() View {
	return aggQC.view
}

func (aggQC AggregateQC) String() string {
	var sb strings.Builder
	if aggQC.signature != nil {
		_ = writeParticipants(&sb, aggQC.signature.Participants())
	}
	return fmt.Sprintf("AggQC{ view: %d, Addrs: [ %s] }", aggQC.view, &sb)
}

func writeParticipants(wr io.Writer, participants AddressSet) (err error) {
	participants.RangeWhile(func(addr Address) bool {
		_, err = fmt.Fprintf(wr, "%s ", addr)
		return err == nil
	})
	return err
}

// TimeoutCert (TC) is a certificate created by a quorum of timeout messsages.
type TimeoutCert struct {
	signature QuorumSignature
	view      View
}

func NewTimeoutCert(signature QuorumSignature, view View) TimeoutCert {
	return TimeoutCert{signature, view}
}

func (tc TimeoutCert) Signature() QuorumSignature {
	return tc.signature
}

func (tc TimeoutCert) View() View {
	return tc.view
}

func (tc TimeoutCert) String() string {
	var sb strings.Builder
	if tc.signature != nil {
		_ = writeParticipants(&sb, tc.Signature().Participants())
	}
	return fmt.Sprintf("TC{ view: %d, Addrs: [ %s] }", tc.view, &sb)
}
