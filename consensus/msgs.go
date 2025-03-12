package consensus

import (
	"errors"
	"fmt"

	tmcons "github.com/xufeisofly/hotstuff/proto/hotstuff/consensus"
	"github.com/xufeisofly/hotstuff/types"
)

// MsgFromProto takes a consensus proto message and returns the native go type
func MsgFromProto(p *tmcons.Message) (Message, error) {
	if p == nil {
		return nil, errors.New("consensus: nil message")
	}
	var pb Message
	um, err := p.Unwrap()
	if err != nil {
		return nil, err
	}

	switch msg := um.(type) {
	case *tmcons.Proposal:
		pbP, err := types.HsProposalFromProto(&msg.Proposal)
		if err != nil {
			return nil, fmt.Errorf("proposal msg to proto error: %w", err)
		}

		pb = &ProposalMessage{
			Proposal: pbP,
		}
	case *tmcons.Vote:
		vote, err := types.VoteFromProto(msg.Vote)
		if err != nil {
			return nil, fmt.Errorf("vote msg to proto error: %w", err)
		}

		pb = &VoteMessage{
			Vote: vote,
		}
	default:
		return nil, fmt.Errorf("consensus: message not recognized: %T", msg)
	}

	if err := pb.ValidateBasic(); err != nil {
		return nil, err
	}

	return pb, nil
}
