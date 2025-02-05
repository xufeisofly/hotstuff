package consensus

import (
	"fmt"

	"github.com/xufeisofly/hotstuff/types"
)

type ErrExpiredView struct {
	View types.View
}

func (err ErrExpiredView) Error() string {
	return fmt.Sprintf("view %d has been expired", err.View)
}

// qc referenced block not found
type ErrNotFoundQcRefBlock struct {
	Hash types.Hash
	View types.View
}

func (err ErrNotFoundQcRefBlock) Error() string {
	return fmt.Sprintf("qc ref block not found, view: %d, hash: %s", err.View, err.Hash)
}

// parent block not found
type ErrNotFoundParentBlock struct {
	Hash types.Hash
	View types.View
}

func (err ErrNotFoundParentBlock) Error() string {
	return fmt.Sprintf("parent block not found, view: %d, hash: %s", err.View, err.Hash)
}
