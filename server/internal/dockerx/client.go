package dockerx

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
)

// Client wraps the Docker Engine API client. It is optional:
// on dev machines without /var/run/docker.sock all operations
// return ErrUnavailable instead of failing the whole API.
type Client struct {
	mu  sync.RWMutex
	cli *client.Client
	ok  bool
}

var ErrUnavailable = dockerErr("docker unavailable: /var/run/docker.sock not reachable")

type dockerErr string

func (e dockerErr) Error() string { return string(e) }

func New() *Client {
	c := &Client{}
	c.tryConnect()
	return c
}

func (c *Client) tryConnect() {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := cli.Ping(ctx); err != nil {
		// Also try explicit socket path check for clearer errors.
		if _, serr := os.Stat("/var/run/docker.sock"); serr != nil {
			return
		}
		return
	}
	c.mu.Lock()
	c.cli = cli
	c.ok = true
	c.mu.Unlock()
}

func (c *Client) Available() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ok && c.cli != nil
}

// RetryConnect allows the /api/containers endpoint to lazily reconnect
// (useful when Docker starts after ServerHub).
func (c *Client) RetryConnect() {
	if !c.Available() {
		c.tryConnect()
	}
}

func (c *Client) withClient() (*client.Client, error) {
	c.RetryConnect()
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.ok || c.cli == nil {
		return nil, ErrUnavailable
	}
	return c.cli, nil
}

func (c *Client) ListContainers(ctx context.Context) ([]container.Summary, error) {
	cli, err := c.withClient()
	if err != nil {
		return nil, err
	}
	return cli.ContainerList(ctx, container.ListOptions{All: true})
}

func (c *Client) InspectContainer(ctx context.Context, id string) (container.InspectResponse, error) {
	cli, err := c.withClient()
	if err != nil {
		return container.InspectResponse{}, err
	}
	return cli.ContainerInspect(ctx, id)
}

func (c *Client) ListImages(ctx context.Context) ([]image.Summary, error) {
	cli, err := c.withClient()
	if err != nil {
		return nil, err
	}
	return cli.ImageList(ctx, image.ListOptions{All: false})
}

func (c *Client) ListVolumes(ctx context.Context) (volume.ListResponse, error) {
	cli, err := c.withClient()
	if err != nil {
		return volume.ListResponse{}, err
	}
	return cli.VolumeList(ctx, volume.ListOptions{})
}

// Lifecycle operations. All require a reachable Docker daemon and are
// audit-logged by the calling handler. Stop/Restart callers must enforce
// an explicit confirmation gate (high-risk per spec §15).
func (c *Client) StartContainer(ctx context.Context, id string) error {
	cli, err := c.withClient()
	if err != nil {
		return err
	}
	return cli.ContainerStart(ctx, id, container.StartOptions{})
}

func (c *Client) StopContainer(ctx context.Context, id string, timeoutSec int) error {
	cli, err := c.withClient()
	if err != nil {
		return err
	}
	opts := container.StopOptions{}
	if timeoutSec >= 0 {
		opts.Timeout = &timeoutSec
	}
	return cli.ContainerStop(ctx, id, opts)
}

func (c *Client) RestartContainer(ctx context.Context, id string, timeoutSec int) error {
	cli, err := c.withClient()
	if err != nil {
		return err
	}
	// Engine API v1.42+: restart takes StopOptions.
	opts := container.StopOptions{}
	if timeoutSec >= 0 {
		opts.Timeout = &timeoutSec
	}
	return cli.ContainerRestart(ctx, id, opts)
}

// Raw exposes the underlying engine client for logs/stats streaming.
func (c *Client) Raw() (*client.Client, error) {
	return c.withClient()
}
