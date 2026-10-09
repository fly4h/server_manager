//go:build !windows

package dockerapi

import (
	"context"
	"fmt"
	"net"
)

// dialNamedPipe is a no-op outside Windows: the Docker Engine there is reached
// through a unix socket instead. Keeping the symbol lets client.go stay portable.
func dialNamedPipe(_ context.Context, pipe string) (net.Conn, error) {
	return nil, fmt.Errorf("named pipe %q is only supported on Windows; use unix:// or tcp://", pipe)
}
