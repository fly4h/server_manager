package dockerapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// Client talks to the Docker Engine REST API over a unix socket,
// Windows named pipe, or tcp — whatever DOCKER_HOST says.
type Client struct {
	http *http.Client
	base string
}

// New builds a client from a docker host URL:
//
//	unix:///var/run/docker.sock
//	npipe:////./pipe/docker_engine
//	tcp://127.0.0.1:2375
func New(host string) (*Client, error) {
	var (
		transport *http.Transport
		base      string
	)
	switch {
	case host == "" || strings.HasPrefix(host, "unix://"):
		socket := "/var/run/docker.sock"
		if s := strings.TrimPrefix(host, "unix://"); s != "" {
			socket = s
		}
		transport = &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", socket)
			},
		}
		base = "http://docker"
	case strings.HasPrefix(host, "npipe://"):
		pipe := `\\.\pipe\` + strings.TrimPrefix(strings.TrimPrefix(host, "npipe://"), `\\.\pipe\`)
		transport = &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return dialNamedPipe(ctx, pipe)
			},
		}
		base = "http://docker"
	case strings.HasPrefix(host, "tcp://"):
		transport = &http.Transport{}
		base = "http://" + strings.TrimPrefix(host, "tcp://")
	default:
		return nil, fmt.Errorf("unsupported docker host %q", host)
	}
	return &Client{http: &http.Client{Transport: transport, Timeout: 30 * time.Second}, base: base}, nil
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rdr)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024))
		return &APIError{Status: resp.StatusCode, Body: string(b)}
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// APIError surfaces Docker daemon error responses.
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string { return fmt.Sprintf("docker api %d: %s", e.Status, e.Body) }

// ---- containers ----

// Container is the subset of /containers/json we surface.
type Container struct {
	ID     string   `json:"Id"`
	Names  []string `json:"Names"`
	Image  string   `json:"Image"`
	State  string   `json:"State"`
	Status string   `json:"Status"`
}

func (c *Container) Name() string {
	if len(c.Names) > 0 {
		return strings.TrimPrefix(c.Names[0], "/")
	}
	return c.ID[:12]
}

func (c *Client) ListContainers(ctx context.Context, all bool) ([]Container, error) {
	q := ""
	if all {
		q = "?all=1"
	}
	var out []Container
	err := c.do(ctx, http.MethodGet, "/containers/json"+q, nil, &out)
	return out, err
}

func (c *Client) ContainerAction(ctx context.Context, id, action string) error {
	return c.do(ctx, http.MethodPost, "/containers/"+id+"/"+action, nil, nil)
}

func (c *Client) RemoveContainer(ctx context.Context, id string, force bool) error {
	q := ""
	if force {
		q = "?force=1"
	}
	return c.do(ctx, http.MethodDelete, "/containers/"+id+q, nil, nil)
}

// InspectContainer returns raw JSON so the frontend can render details freely.
func (c *Client) InspectContainer(ctx context.Context, id string) (json.RawMessage, error) {
	var out json.RawMessage
	err := c.do(ctx, http.MethodGet, "/containers/"+id+"/json", nil, &out)
	return out, err
}

// ContainerLogs returns demultiplexed log text (stdout+stderr merged, newest last).
func (c *Client) ContainerLogs(ctx context.Context, id string, tail int) (string, error) {
	if tail <= 0 {
		tail = 200
	}
	// plain text mode via t=0 keeps the multiplexed framing; we demux manually.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/containers/%s/logs?stdout=1&stderr=1&tail=%d&t=1", c.base, id, tail), nil)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024))
		return "", &APIError{Status: resp.StatusCode, Body: string(b)}
	}
	return demuxDockerStream(resp.Body)
}

// demuxDockerStream strips the 8-byte stream header frames Docker uses.
func demuxDockerStream(r io.Reader) (string, error) {
	var out strings.Builder
	hdr := make([]byte, 8)
	for {
		if _, err := io.ReadFull(r, hdr); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			return out.String(), err
		}
		size := int64(hdr[4])<<24 | int64(hdr[5])<<16 | int64(hdr[6])<<8 | int64(hdr[7])
		if _, err := io.CopyN(&out, r, size); err != nil {
			break
		}
	}
	return out.String(), nil
}

// ---- images / volumes / networks ----

type Image struct {
	ID       string   `json:"Id"`
	RepoTags []string `json:"RepoTags"`
	Size     int64    `json:"Size"`
	Created  int64    `json:"Created"`
}

func (i *Image) Tag() string {
	if len(i.RepoTags) > 0 {
		return i.RepoTags[0]
	}
	return "<none>"
}

type volumeList struct {
	Volumes []map[string]any `json:"Volumes"`
}
type networkList struct {
	Networks []map[string]any `json:"Networks"`
}

func (c *Client) ListImages(ctx context.Context) ([]Image, error) {
	var out []Image
	err := c.do(ctx, http.MethodGet, "/images/json", nil, &out)
	return out, err
}

func (c *Client) ListVolumes(ctx context.Context) ([]map[string]any, error) {
	var out volumeList
	if err := c.do(ctx, http.MethodGet, "/volumes", nil, &out); err != nil {
		return nil, err
	}
	return out.Volumes, nil
}

func (c *Client) ListNetworks(ctx context.Context) ([]map[string]any, error) {
	var out networkList
	if err := c.do(ctx, http.MethodGet, "/networks", nil, &out); err != nil {
		return nil, err
	}
	return out.Networks, nil
}

func (c *Client) RemoveImage(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/images/"+id, nil, nil)
}

func (c *Client) RemoveVolume(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, "/volumes/"+name, nil, nil)
}

func (c *Client) Ping(ctx context.Context) error {
	return c.do(ctx, http.MethodGet, "/_ping", nil, nil)
}
