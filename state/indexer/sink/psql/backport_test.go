package psql

import (
	"github.com/xufeisofly/hotstuff-core/state/indexer"
	"github.com/xufeisofly/hotstuff-core/state/txindex"
)

var (
	_ indexer.BlockIndexer = BackportBlockIndexer{}
	_ txindex.TxIndexer    = BackportTxIndexer{}
)
