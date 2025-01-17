package proxy

import (
	abci "github.com/xufeisofly/hotstuff-core/abci/types"
	"github.com/xufeisofly/hotstuff-core/version"
)

// RequestInfo contains all the information for sending
// the abci.RequestInfo message during handshake with the app.
// It contains only compile-time version information.
var RequestInfo = abci.RequestInfo{
	Version:      version.TMCoreSemVer,
	BlockVersion: version.BlockProtocol,
	P2PVersion:   version.P2PProtocol,
}
