package aria2

import (
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

// Client timeouts and response limits.
const (
	requestTimeout   = 10 * time.Second
	dialTimeout      = 2 * time.Second
	maxResponseBytes = 8 << 20 // 8 MiB
)

// rpcError is a JSON-RPC fault reported by aria2.
type rpcError struct {
	Code    int
	Message string
}

func (e *rpcError) Error() string {
	return fmt.Sprintf("aria2 error %d: %s", e.Code, e.Message)
}

// isConnectionError reports whether err came from the transport rather than
// from aria2 itself. A daemon that is shutting down closes the connection
// before the reply has been flushed, which is not a failure to report.
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed) {
		return true
	}
	var netErr *net.OpError
	return errors.As(err, &netErr)
}
