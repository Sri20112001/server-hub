package handlers

import (
	"database/sql"

	"github.com/docker/docker/client"
	"serverhub/internal/dockerx"
)

func (h *ContainerHandler) rawClient() (*client.Client, error) {
	// h.Docker is *dockerx.Client; expose raw engine client.
	type rawer interface {
		Raw() (*client.Client, error)
	}
	if r, ok := interface{}(h.Docker).(rawer); ok {
		return r.Raw()
	}
	return nil, dockerx.ErrUnavailable
}

var _ = sql.ErrNoRows

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
