//go:build windows

package dockerapi

import (
	"context"
	"net"

	"github.com/Microsoft/go-winio"
)

// dialNamedPipe connects to the Docker Engine over the Windows named pipe
// (npipe:////./pipe/docker_engine).
func dialNamedPipe(ctx context.Context, pipe string) (net.Conn, error) {
	return winio.DialPipeContext(ctx, pipe)
}
