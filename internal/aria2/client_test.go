package aria2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// ─── fake JSON-RPC endpoint ──────────────────────────────────────────────────

// recordedCall is one dispatched JSON-RPC method with its decoded params.
type recordedCall struct {
	method string
	params []any
}

// fakeRPC is an httptest backed aria2 JSON-RPC endpoint.
type fakeRPC struct {
	server *httptest.Server

	mu           sync.Mutex
	calls        []recordedCall
	httpRequests int
	handlers     map[string]func(params []any) (any, error)
	// dropResults truncates multicall responses, to model a malformed reply.
	dropResults int
}

func newFakeRPC(t *testing.T) *fakeRPC {
	t.Helper()

	fake := &fakeRPC{handlers: map[string]func(params []any) (any, error){}}
	fake.server = httptest.NewServer(http.HandlerFunc(fake.handle))
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *fakeRPC) endpoint(secret string) Endpoint {
	return Endpoint{URL: f.server.URL, Secret: secret}
}

// on registers the result of one method; a returned error becomes a fault.
func (f *fakeRPC) on(method string, handler func(params []any) (any, error)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.handlers[method] = handler
}

func (f *fakeRPC) onResult(method string, result any) {
	f.on(method, func([]any) (any, error) { return result, nil })
}

func (f *fakeRPC) recorded() []recordedCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]recordedCall(nil), f.calls...)
}

func (f *fakeRPC) callsFor(method string) []recordedCall {
	var found []recordedCall
	for _, call := range f.recorded() {
		if call.method == method {
			found = append(found, call)
		}
	}
	return found
}

func (f *fakeRPC) requestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.httpRequests
}

func (f *fakeRPC) dropMulticallResults(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dropResults = n
}

func (f *fakeRPC) record(method string, params []any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, recordedCall{method: method, params: params})
}

func (f *fakeRPC) invoke(method string, params []any) (any, error) {
	f.mu.Lock()
	handler := f.handlers[method]
	f.mu.Unlock()

	if handler == nil {
		return nil, fmt.Errorf("no fake handler for %s", method)
	}
	return handler(params)
}

func (f *fakeRPC) handle(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var request struct {
		ID     any               `json:"id"`
		Method string            `json:"method"`
		Params []json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	f.mu.Lock()
	f.httpRequests++
	f.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")

	if request.Method == "system.multicall" {
		var nested []struct {
			MethodName string            `json:"methodName"`
			Params     []json.RawMessage `json:"params"`
		}
		if len(request.Params) > 0 {
			_ = json.Unmarshal(request.Params[0], &nested)
		}

		entries := make([]any, 0, len(nested))
		for _, call := range nested {
			params := decodeParams(call.Params)
			f.record(call.MethodName, params)
			value, err := f.invoke(call.MethodName, params)
			if err != nil {
				code, message := faultFrom(err)
				entries = append(entries, map[string]any{"faultCode": code, "faultString": message})
				continue
			}
			entries = append(entries, []any{value})
		}

		f.mu.Lock()
		drop := f.dropResults
		f.mu.Unlock()
		if drop > 0 && drop < len(entries) {
			entries = entries[:len(entries)-drop]
		}

		writeJSON(w, map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": entries})
		return
	}

	params := decodeParams(request.Params)
	f.record(request.Method, params)
	value, err := f.invoke(request.Method, params)
	if err != nil {
		code, message := faultFrom(err)
		writeJSON(w, map[string]any{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"error":   map[string]any{"code": code, "message": message},
		})
		return
	}
	writeJSON(w, map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": value})
}

// faultFrom renders a handler error as aria2 would: an rpcError keeps its own
// code and message, anything else becomes a generic fault.
func faultFrom(err error) (int, string) {
	if rpcFault, ok := errors.AsType[*rpcError](err); ok {
		return rpcFault.Code, rpcFault.Message
	}
	return -32000, err.Error()
}

func decodeParams(raw []json.RawMessage) []any {
	params := make([]any, 0, len(raw))
	for _, item := range raw {
		var value any
		if err := json.Unmarshal(item, &value); err != nil {
			value = string(item)
		}
		params = append(params, value)
	}
	return params
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(payload)
}

// ─── fixtures ────────────────────────────────────────────────────────────────

func activeTaskJSON() map[string]any {
	return map[string]any{
		"gid":             "a1",
		"status":          "active",
		"totalLength":     "1048576",
		"completedLength": "524288",
		"downloadSpeed":   "131072",
		"connections":     "8",
		"errorCode":       "0",
		"errorMessage":    "",
		"files": []map[string]any{
			{"path": "/downloads/ubuntu.iso", "uris": []map[string]any{{"uri": "http://example.org/ubuntu.iso"}}},
		},
	}
}

func stoppedTaskJSON() map[string]any {
	return map[string]any{
		"gid":             "s1",
		"status":          "error",
		"totalLength":     "2048",
		"completedLength": "1024",
		"downloadSpeed":   "0",
		"connections":     "0",
		"errorCode":       "3",
		"errorMessage":    "Resource not found",
		"files": []map[string]any{
			{"path": "/downloads/broken.bin", "uris": []map[string]any{{"uri": "http://example.org/broken.bin"}}},
		},
	}
}

func globalStatJSON() map[string]any {
	return map[string]any{
		"downloadSpeed": "131072",
		"uploadSpeed":   "4096",
		"numActive":     "1",
		"numWaiting":    "2",
		"numStopped":    "3",
	}
}

// serveSnapshot registers the canned responses of one polling round.
func (f *fakeRPC) serveSnapshot(active, waiting, stopped []map[string]any) {
	f.onResult("aria2.tellActive", active)
	f.onResult("aria2.tellWaiting", waiting)
	f.onResult("aria2.tellStopped", stopped)
	f.onResult("aria2.getGlobalStat", globalStatJSON())
}

// ─── tests ───────────────────────────────────────────────────────────────────

func TestSnapshotParsesTypedTasksAndSendsToken(t *testing.T) {
	fake := newFakeRPC(t)
	fake.serveSnapshot([]map[string]any{activeTaskJSON()}, []map[string]any{}, []map[string]any{stoppedTaskJSON()})

	client := NewClient(fake.endpoint("s3cret"))
	snapshot, err := client.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}

	if len(snapshot.Active) != 1 {
		t.Fatalf("len(Active) = %d, want 1", len(snapshot.Active))
	}
	active := snapshot.Active[0]
	if active.GID != "a1" || active.Name != "ubuntu.iso" || active.Status != StatusActive {
		t.Errorf("Active[0] = %+v, want gid a1, name ubuntu.iso, status active", active)
	}
	if active.Total != 1048576 || active.Completed != 524288 || active.Speed != 131072 || active.Connections != 8 {
		t.Errorf("Active[0] numbers = %+v, want the parsed string values", active)
	}

	if len(snapshot.Stopped) != 1 {
		t.Fatalf("len(Stopped) = %d, want 1", len(snapshot.Stopped))
	}
	if got := snapshot.Stopped[0]; got.Status != StatusError || got.ErrorCode != 3 || got.ErrorMessage != "Resource not found" {
		t.Errorf("Stopped[0] = %+v, want the error details", got)
	}

	want := GlobalStat{DownloadSpeed: 131072, UploadSpeed: 4096, NumActive: 1, NumWaiting: 2, NumStopped: 3}
	if snapshot.Stat != want {
		t.Errorf("Stat = %+v, want %+v", snapshot.Stat, want)
	}

	for _, call := range fake.recorded() {
		if len(call.params) == 0 || call.params[0] != "token:s3cret" {
			t.Errorf("%s params = %v, want the token as the first argument", call.method, call.params)
		}
	}
}

func TestSnapshotUsesOneRequestForFourCalls(t *testing.T) {
	fake := newFakeRPC(t)
	fake.serveSnapshot([]map[string]any{}, []map[string]any{}, []map[string]any{})

	client := NewClient(fake.endpoint("secret"))
	if _, err := client.Snapshot(context.Background()); err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}

	if got := fake.requestCount(); got != 1 {
		t.Errorf("http requests = %d, want 1 multicall request", got)
	}
	for _, method := range []string{"aria2.tellActive", "aria2.tellWaiting", "aria2.tellStopped", "aria2.getGlobalStat"} {
		if len(fake.callsFor(method)) != 1 {
			t.Errorf("%s calls = %d, want 1", method, len(fake.callsFor(method)))
		}
	}

	waiting := fake.callsFor("aria2.tellWaiting")
	if len(waiting) == 1 {
		// token, offset, count, keys
		if len(waiting[0].params) != 4 {
			t.Fatalf("tellWaiting params = %v, want 4 arguments", waiting[0].params)
		}
		if waiting[0].params[1] != float64(0) || waiting[0].params[2] != float64(snapshotMaxResults) {
			t.Errorf("tellWaiting params = %v, want offset 0 and count %d", waiting[0].params, snapshotMaxResults)
		}
	}
}

func TestMulticallFaultBecomesRPCError(t *testing.T) {
	fake := newFakeRPC(t)
	fake.onResult("aria2.tellActive", []map[string]any{})
	fake.onResult("aria2.tellWaiting", []map[string]any{})
	fake.on("aria2.tellStopped", func([]any) (any, error) {
		return nil, &rpcError{Code: 1, Message: "Unauthorized"}
	})
	fake.onResult("aria2.getGlobalStat", globalStatJSON())

	_, err := NewClient(fake.endpoint("wrong")).Snapshot(context.Background())
	if err == nil {
		t.Fatal("Snapshot() error = nil, want a fault")
	}

	var rpcFault *rpcError
	if !errors.As(err, &rpcFault) {
		t.Fatalf("Snapshot() error = %v, want an rpcError", err)
	}
	if rpcFault.Code != 1 || rpcFault.Message != "Unauthorized" {
		t.Errorf("rpcError = %+v, want code 1 and message Unauthorized", rpcFault)
	}
	if !strings.Contains(rpcFault.Error(), "Unauthorized") {
		t.Errorf("rpcError.Error() = %q, want it to mention the message", rpcFault.Error())
	}
}

func TestSnapshotReportsContextDeadline(t *testing.T) {
	fake := newFakeRPC(t)
	fake.on("aria2.tellActive", func([]any) (any, error) {
		time.Sleep(200 * time.Millisecond)
		return []map[string]any{}, nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := NewClient(fake.endpoint("secret")).Snapshot(ctx)
	if err == nil {
		t.Fatal("Snapshot() error = nil, want a deadline error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Snapshot() error = %v, want it to wrap %v", err, context.DeadlineExceeded)
	}
}

func TestAddURIsChunksRequests(t *testing.T) {
	fake := newFakeRPC(t)
	var mu sync.Mutex
	var seen []string
	fake.on("aria2.addUri", func(params []any) (any, error) {
		mu.Lock()
		defer mu.Unlock()
		uris, ok := params[1].([]any)
		if !ok || len(uris) != 1 {
			return nil, fmt.Errorf("unexpected uri argument: %v", params)
		}
		uri, _ := uris[0].(string)
		seen = append(seen, uri)
		return fmt.Sprintf("gid-%d", len(seen)), nil
	})

	uris := make([]string, 0, addURIChunkSize*2+1)
	for i := range addURIChunkSize*2 + 1 {
		uris = append(uris, fmt.Sprintf("http://example.org/%d", i))
	}

	client := NewClient(fake.endpoint("secret"))
	gids, err := client.AddURIs(context.Background(), uris, nil)
	if err != nil {
		t.Fatalf("AddURIs() error = %v", err)
	}

	if len(gids) != len(uris) {
		t.Fatalf("len(gids) = %d, want %d", len(gids), len(uris))
	}
	for i, gid := range gids {
		if gid == "" {
			t.Fatalf("gids[%d] is empty, want a gid for every uri", i)
		}
	}
	if got := fake.requestCount(); got != 3 {
		t.Errorf("http requests = %d, want 3 chunks of %d", got, addURIChunkSize)
	}
	if len(seen) != len(uris) {
		t.Errorf("addUri calls = %d, want %d", len(seen), len(uris))
	}

	first := fake.callsFor("aria2.addUri")[0]
	if first.params[0] != "token:secret" {
		t.Errorf("addUri params[0] = %v, want the token", first.params[0])
	}
	if _, ok := first.params[2].(map[string]any); !ok {
		t.Errorf("addUri params[2] = %v, want an options object", first.params[2])
	}
}

func TestAddURIsReportsPartialFailure(t *testing.T) {
	fake := newFakeRPC(t)
	fake.on("aria2.addUri", func(params []any) (any, error) {
		uris, _ := params[1].([]any)
		uri, _ := uris[0].(string)
		if strings.HasSuffix(uri, "forbidden") {
			return nil, &rpcError{Code: 24, Message: "Removed"}
		}
		return "gid-ok", nil
	})

	client := NewClient(fake.endpoint("secret"))
	gids, err := client.AddURIs(context.Background(), []string{"http://example.org/ok", "http://example.org/forbidden"}, nil)
	if err == nil {
		t.Fatal("AddURIs() error = nil, want the rejected uri reported")
	}
	if !strings.Contains(err.Error(), "forbidden") {
		t.Errorf("AddURIs() error = %q, want it to mention the rejected uri", err)
	}
	if gids[0] != "gid-ok" {
		t.Errorf("gids[0] = %q, want gid-ok", gids[0])
	}
	if gids[1] != "" {
		t.Errorf("gids[1] = %q, want an empty gid for the rejected uri", gids[1])
	}
}

func TestAddURIsWithoutSecretOmitsToken(t *testing.T) {
	fake := newFakeRPC(t)
	fake.onResult("aria2.addUri", "gid-1")

	client := NewClient(fake.endpoint(""))
	if _, err := client.AddURIs(context.Background(), []string{"http://example.org/file"}, nil); err != nil {
		t.Fatalf("AddURIs() error = %v", err)
	}

	call := fake.callsFor("aria2.addUri")[0]
	if _, isToken := call.params[0].(string); isToken {
		t.Errorf("addUri params[0] = %v, want the uri list without a token", call.params[0])
	}
	if _, ok := call.params[0].([]any); !ok {
		t.Errorf("addUri params[0] = %v, want the uri list", call.params[0])
	}
}

func TestActionsUseExpectedMethods(t *testing.T) {
	fake := newFakeRPC(t)
	for _, method := range []string{
		"aria2.pause",
		"aria2.unpause",
		"aria2.remove",
		"aria2.removeDownloadResult",
		"aria2.changeGlobalOption",
	} {
		fake.onResult(method, "OK")
	}

	client := NewClient(fake.endpoint("secret"))
	ctx := context.Background()

	if err := client.Pause(ctx, "gid-1"); err != nil {
		t.Fatalf("Pause() error = %v", err)
	}
	if err := client.Unpause(ctx, "gid-1"); err != nil {
		t.Fatalf("Unpause() error = %v", err)
	}
	if err := client.Remove(ctx, "gid-1"); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if err := client.PurgeResult(ctx, "gid-1"); err != nil {
		t.Fatalf("PurgeResult() error = %v", err)
	}
	if err := client.SetMaxConcurrent(ctx, 3); err != nil {
		t.Fatalf("SetMaxConcurrent() error = %v", err)
	}

	for _, method := range []string{"aria2.pause", "aria2.unpause", "aria2.remove", "aria2.removeDownloadResult"} {
		calls := fake.callsFor(method)
		if len(calls) != 1 {
			t.Fatalf("%s calls = %d, want 1", method, len(calls))
		}
		if len(calls[0].params) != 2 || calls[0].params[1] != "gid-1" {
			t.Errorf("%s params = %v, want the token and gid-1", method, calls[0].params)
		}
	}

	options, ok := fake.callsFor("aria2.changeGlobalOption")[0].params[1].(map[string]any)
	if !ok {
		t.Fatal("changeGlobalOption did not receive an options object")
	}
	if options["max-concurrent-downloads"] != "3" {
		t.Errorf("changeGlobalOption options = %v, want max-concurrent-downloads 3 as a string", options)
	}
}

func TestVersionParsesResult(t *testing.T) {
	fake := newFakeRPC(t)
	fake.onResult("aria2.getVersion", map[string]any{"version": "1.37.0", "enabledFeatures": []string{"BitTorrent"}})

	version, err := NewClient(fake.endpoint("secret")).Version(context.Background())
	if err != nil {
		t.Fatalf("Version() error = %v", err)
	}
	if version != "1.37.0" {
		t.Errorf("Version() = %q, want 1.37.0", version)
	}
}

func TestVersionReportsRPCError(t *testing.T) {
	fake := newFakeRPC(t)
	fake.on("aria2.getVersion", func([]any) (any, error) {
		return nil, &rpcError{Code: 1, Message: "Unauthorized"}
	})

	_, err := NewClient(fake.endpoint("wrong")).Version(context.Background())
	if _, ok := errors.AsType[*rpcError](err); !ok {
		t.Fatalf("Version() error = %v, want an rpcError", err)
	}
}

// aria2 answers a shutdown by closing the connection, so a transport error
// must not be reported as a failure.
func TestShutdownToleratesClosedConnection(t *testing.T) {
	fake := newFakeRPC(t)
	fake.on("aria2.shutdown", func([]any) (any, error) {
		return nil, nil
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "no hijack", http.StatusInternalServerError)
			return
		}
		conn, _, err := hijacker.Hijack()
		if err != nil {
			return
		}
		_ = conn.Close()
	}))
	defer server.Close()

	client := NewClient(Endpoint{URL: server.URL, Secret: "secret"})
	if err := client.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown() error = %v, want nil for a closed connection", err)
	}
}

func TestShutdownReportsRPCError(t *testing.T) {
	fake := newFakeRPC(t)
	fake.on("aria2.shutdown", func([]any) (any, error) {
		return nil, &rpcError{Code: 1, Message: "Unauthorized"}
	})

	err := NewClient(fake.endpoint("wrong")).Shutdown(context.Background())
	if _, ok := errors.AsType[*rpcError](err); !ok {
		t.Fatalf("Shutdown() error = %v, want an rpcError", err)
	}
}

func TestCallReportsHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := NewClient(Endpoint{URL: server.URL}).Version(context.Background())
	if err == nil {
		t.Fatal("Version() error = nil, want an http status error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("Version() error = %q, want it to mention the status code", err)
	}
}

func TestParseMulticallRejectsUnexpectedShapes(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "not an array", raw: `"nope"`},
		{name: "empty array entry", raw: `[]`},
		{name: "empty entry", raw: ` `},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseMulticall([]json.RawMessage{json.RawMessage(tt.raw)}); err == nil {
				t.Errorf("parseMulticall(%s) error = nil, want an error", tt.raw)
			}
		})
	}
}

// A truncated multicall reply must be an error, not an index-out-of-range
// panic inside the polling command.
func TestSnapshotRejectsTruncatedMulticallResult(t *testing.T) {
	fake := newFakeRPC(t)
	fake.serveSnapshot([]map[string]any{activeTaskJSON()}, []map[string]any{}, []map[string]any{})
	fake.dropMulticallResults(1)

	_, err := NewClient(fake.endpoint("secret")).Snapshot(context.Background())
	if err == nil {
		t.Fatal("Snapshot() error = nil, want an error for a short response")
	}
	if !strings.Contains(err.Error(), "results") {
		t.Errorf("Snapshot() error = %q, want it to mention the mismatched result count", err)
	}
}

func TestAddURIsRejectsTruncatedMulticallResult(t *testing.T) {
	fake := newFakeRPC(t)
	fake.onResult("aria2.addUri", "gid-1")
	fake.dropMulticallResults(1)

	gids, err := NewClient(fake.endpoint("secret")).AddURIs(context.Background(),
		[]string{"http://example.org/one", "http://example.org/two"}, nil)
	if err == nil {
		t.Fatal("AddURIs() error = nil, want an error for a short response")
	}
	for i, gid := range gids {
		if gid != "" {
			t.Errorf("gids[%d] = %q, want no gid recorded for a malformed response", i, gid)
		}
	}
}

func TestStopSurvivesAnUnresponsiveShutdown(t *testing.T) {
	fake := newFakeRPC(t)
	fake.on("aria2.shutdown", func([]any) (any, error) {
		time.Sleep(500 * time.Millisecond)
		return "OK", nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	// The daemon is busy flushing, so the reply arrives after the grace period:
	// the caller falls back to killing the process.
	if err := NewClient(fake.endpoint("secret")).Shutdown(ctx); err != nil {
		t.Errorf("Shutdown() error = %v, want nil for a request that outlived the grace period", err)
	}
}

func TestShutdownReportsHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	err := NewClient(Endpoint{URL: server.URL, Secret: "secret"}).Shutdown(context.Background())
	if err == nil {
		t.Fatal("Shutdown() error = nil, want the http failure reported")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("Shutdown() error = %q, want it to mention the status code", err)
	}
}
