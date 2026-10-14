package aria2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"
)

const (
	// snapshotMaxResults caps how many waiting and stopped downloads one
	// snapshot retrieves.
	snapshotMaxResults = 100
	// addURIChunkSize caps how many addUri calls share one multicall request.
	addURIChunkSize = 50
)

// taskKeys limits tell* responses to the fields the UI consumes.
var taskKeys = []string{
	"gid",
	"status",
	"totalLength",
	"completedLength",
	"downloadSpeed",
	"connections",
	"errorCode",
	"errorMessage",
	"files",
	"bittorrent",
}

// Endpoint describes where the daemon listens.
type Endpoint struct {
	URL    string
	Secret string
}

// Client is a JSON-RPC client for a single aria2 daemon.
type Client struct {
	ep   Endpoint
	http *http.Client
	id   atomic.Uint64
}

// NewClient returns a client for ep. The HTTP client keeps connections alive
// because the UI polls once per second for the whole session.
func NewClient(ep Endpoint) *Client {
	return &Client{
		ep: ep,
		http: &http.Client{
			Timeout: requestTimeout,
			Transport: &http.Transport{
				DialContext:         (&net.Dialer{Timeout: dialTimeout}).DialContext,
				MaxIdleConns:        4,
				MaxIdleConnsPerHost: 4,
				IdleConnTimeout:     time.Minute,
			},
		},
	}
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      string `json:"id"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
}

type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *rpcFault       `json:"error"`
}

type rpcFault struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// multicall is one nested call inside a system.multicall request.
type multicall struct {
	MethodName string `json:"methodName"`
	Params     []any  `json:"params"`
}

// params prepends the RPC secret, which aria2 expects as the first argument of
// every method (including each nested call of a multicall).
func (c *Client) params(args ...any) []any {
	params := make([]any, 0, len(args)+1)
	if c.ep.Secret != "" {
		params = append(params, "token:"+c.ep.Secret)
	}
	return append(params, args...)
}

// call performs one JSON-RPC request; out receives the decoded result.
func (c *Client) call(ctx context.Context, method string, params []any, out any) error {
	body, err := json.Marshal(rpcRequest{
		JSONRPC: "2.0",
		ID:      strconv.FormatUint(c.id.Add(1), 10),
		Method:  method,
		Params:  params,
	})
	if err != nil {
		return fmt.Errorf("%s: encode request: %w", method, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.ep.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%s: build request: %w", method, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("%s: read response: %w", method, err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: unexpected http status %s", method, resp.Status)
	}

	var envelope rpcResponse
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("%s: decode response: %w", method, err)
	}
	if envelope.Error != nil {
		return &rpcError{Code: envelope.Error.Code, Message: envelope.Error.Message}
	}
	if out == nil || len(envelope.Result) == 0 {
		return nil
	}
	if err := json.Unmarshal(envelope.Result, out); err != nil {
		return fmt.Errorf("%s: decode result: %w", method, err)
	}
	return nil
}

// multicallResult holds either the single value of a successful nested call or
// the fault that replaced it.
type multicallResult struct {
	value json.RawMessage
	err   error
}

func (r multicallResult) decode(out any) error {
	if r.err != nil {
		return r.err
	}
	if len(r.value) == 0 {
		return errors.New("multicall result is empty")
	}
	return json.Unmarshal(r.value, out)
}

// multicall runs calls in a single system.multicall request.
func (c *Client) multicall(ctx context.Context, calls []multicall) ([]multicallResult, error) {
	var raw []json.RawMessage
	if err := c.call(ctx, "system.multicall", []any{calls}, &raw); err != nil {
		return nil, err
	}
	return parseMulticall(raw)
}

// parseMulticall decodes the response of system.multicall: one entry per
// nested call, either a one element array holding the value or an object with
// faultCode and faultString.
func parseMulticall(raw []json.RawMessage) ([]multicallResult, error) {
	results := make([]multicallResult, len(raw))
	for i, entry := range raw {
		trimmed := bytes.TrimSpace(entry)
		if len(trimmed) == 0 {
			return nil, fmt.Errorf("multicall entry %d is empty", i)
		}
		switch trimmed[0] {
		case '[':
			var values []json.RawMessage
			if err := json.Unmarshal(trimmed, &values); err != nil {
				return nil, fmt.Errorf("multicall entry %d: %w", i, err)
			}
			if len(values) == 0 {
				return nil, fmt.Errorf("multicall entry %d returned no value", i)
			}
			results[i].value = values[0]
		case '{':
			var fault struct {
				Code   int    `json:"faultCode"`
				String string `json:"faultString"`
			}
			if err := json.Unmarshal(trimmed, &fault); err != nil {
				return nil, fmt.Errorf("multicall entry %d: %w", i, err)
			}
			results[i].err = &rpcError{Code: fault.Code, Message: fault.String}
		default:
			return nil, fmt.Errorf("multicall entry %d has an unexpected shape", i)
		}
	}
	return results, nil
}

// Snapshot fetches the download lists and the global statistics in a single
// HTTP request: four nested calls share one system.multicall round trip.
func (c *Client) Snapshot(ctx context.Context) (Snapshot, error) {
	calls := []multicall{
		{MethodName: "aria2.tellActive", Params: c.params(taskKeys)},
		{MethodName: "aria2.tellWaiting", Params: c.params(0, snapshotMaxResults, taskKeys)},
		{MethodName: "aria2.tellStopped", Params: c.params(0, snapshotMaxResults, taskKeys)},
		{MethodName: "aria2.getGlobalStat", Params: c.params()},
	}

	results, err := c.multicall(ctx, calls)
	if err != nil {
		return Snapshot{}, err
	}

	var snap Snapshot
	if err := decodeTasks(results[0], &snap.Active); err != nil {
		return Snapshot{}, err
	}
	if err := decodeTasks(results[1], &snap.Waiting); err != nil {
		return Snapshot{}, err
	}
	if err := decodeTasks(results[2], &snap.Stopped); err != nil {
		return Snapshot{}, err
	}
	var stat rawGlobalStat
	if err := results[3].decode(&stat); err != nil {
		return Snapshot{}, err
	}
	snap.Stat = stat.stat()
	return snap, nil
}

func decodeTasks(result multicallResult, out *[]Task) error {
	var raw []rawTask
	if err := result.decode(&raw); err != nil {
		return err
	}
	tasks := make([]Task, 0, len(raw))
	for _, item := range raw {
		tasks = append(tasks, item.task())
	}
	*out = tasks
	return nil
}

// AddURIs registers one download per URI and returns their GIDs aligned with
// uris: an entry that aria2 rejected holds an empty GID while the remaining
// URIs are still added, and a non-nil error reports the rejected ones.
func (c *Client) AddURIs(ctx context.Context, uris []string, opts map[string]string) ([]string, error) {
	if len(uris) == 0 {
		return nil, nil
	}
	if opts == nil {
		opts = map[string]string{}
	}

	gids := make([]string, len(uris))
	var failures []error

	for start := 0; start < len(uris); start += addURIChunkSize {
		end := min(start+addURIChunkSize, len(uris))
		chunk := uris[start:end]

		calls := make([]multicall, len(chunk))
		for i, uri := range chunk {
			calls[i] = multicall{MethodName: "aria2.addUri", Params: c.params([]string{uri}, opts)}
		}

		results, err := c.multicall(ctx, calls)
		if err != nil {
			for _, uri := range chunk {
				failures = append(failures, fmt.Errorf("%s: %w", uri, err))
			}
			continue
		}
		for i, result := range results {
			var gid string
			if err := result.decode(&gid); err != nil {
				failures = append(failures, fmt.Errorf("%s: %w", chunk[i], err))
				continue
			}
			gids[start+i] = gid
		}
	}

	return gids, errors.Join(failures...)
}

// Pause pauses a download.
func (c *Client) Pause(ctx context.Context, gid string) error {
	return c.call(ctx, "aria2.pause", c.params(gid), nil)
}

// Unpause resumes a paused download.
func (c *Client) Unpause(ctx context.Context, gid string) error {
	return c.call(ctx, "aria2.unpause", c.params(gid), nil)
}

// Remove removes an active, waiting or paused download.
func (c *Client) Remove(ctx context.Context, gid string) error {
	return c.call(ctx, "aria2.remove", c.params(gid), nil)
}

// PurgeResult drops a stopped download from the result list.
func (c *Client) PurgeResult(ctx context.Context, gid string) error {
	return c.call(ctx, "aria2.removeDownloadResult", c.params(gid), nil)
}

// SetMaxConcurrent applies the parallelism limit to the running daemon.
func (c *Client) SetMaxConcurrent(ctx context.Context, n int) error {
	return c.call(ctx, "aria2.changeGlobalOption", c.params(map[string]string{
		"max-concurrent-downloads": strconv.Itoa(n),
	}), nil)
}

// Version returns the daemon version; it doubles as the readiness probe.
func (c *Client) Version(ctx context.Context) (string, error) {
	var result struct {
		Version string `json:"version"`
	}
	if err := c.call(ctx, "aria2.getVersion", c.params(), &result); err != nil {
		return "", err
	}
	if result.Version == "" {
		return "", errors.New("aria2.getVersion returned an empty version")
	}
	return result.Version, nil
}

// Shutdown asks the daemon to exit. aria2 closes the connection while
// answering, so a transport error means the request was accepted and only an
// RPC level failure is reported.
func (c *Client) Shutdown(ctx context.Context) error {
	err := c.call(ctx, "aria2.shutdown", c.params(), nil)
	var rpcErr *rpcError
	if err != nil && !errors.As(err, &rpcErr) {
		return nil
	}
	return err
}
