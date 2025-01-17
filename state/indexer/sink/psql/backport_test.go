package psql

import (
	"github.com/xufeisofly/hotstuff/state/indexer"
	"github.com/xufeisofly/hotstuff/state/txindex"
)

var (
	_ indexer.BlockIndexer = BackportBlockIndexer{}
	_ txindex.TxIndexer    = BackportTxIndexer{}
)
