package tui

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"charm.land/bubbles/v2/cursor"
	tea "charm.land/bubbletea/v2"
	"github.com/KevinYouu/aria2-tui/internal/aria2"
	"github.com/KevinYouu/aria2-tui/internal/config"
)

// commandTimeout bounds how long the harness waits for a command. Long lived
// commands (the poll timer) are skipped by the caller instead.
const commandTimeout = time.Second

// fakeCall is one JSON-RPC method the app invoked.
type fakeCall struct {
	method string
	params []any
}

// fakeRPC is an httptest backed aria2 daemon serving canned state.
type fakeRPC struct {
	server *httptest.Server

	mu        sync.Mutex
	calls     []fakeCall
	active    []map[string]any
	waiting   []map[string]any
	stopped   []map[string]any
	stat      map[string]any
	version   string
	addedURIs []string
	gidSeq    int
}

func newFakeRPC(t *testing.T) *fakeRPC {
	t.Helper()

	fake := &fakeRPC{
		version: "1.37.0",
		stat: map[string]any{
			"downloadSpeed": "0",
			"uploadSpeed":   "0",
			"numActive":     "0",
			"numWaiting":    "0",
			"numStopped":    "0",
		},
	}
	fake.server = httptest.NewServer(http.HandlerFunc(fake.handle))
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *fakeRPC) client() *aria2.Client {
	return aria2.NewClient(aria2.Endpoint{URL: f.server.URL, Secret: "secret"})
}

func (f *fakeRPC) setTasks(active, waiting, stopped []map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.active, f.waiting, f.stopped = active, waiting, stopped
	f.stat["numActive"] = strconv.Itoa(len(active))
	f.stat["numWaiting"] = strconv.Itoa(len(waiting))
	f.stat["numStopped"] = strconv.Itoa(len(stopped))
}

func (f *fakeRPC) recorded() []fakeCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fakeCall(nil), f.calls...)
}

func (f *fakeRPC) callsFor(method string) []fakeCall {
	var found []fakeCall
	for _, call := range f.recorded() {
		if call.method == method {
			found = append(found, call)
		}
	}
	return found
}

func (f *fakeRPC) added() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.addedURIs...)
}

func (f *fakeRPC) record(method string, params []any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{method: method, params: params})
}

func (f *fakeRPC) dispatch(method string, params []any) (any, error) {
	f.record(method, params)

	f.mu.Lock()
	defer f.mu.Unlock()

	switch method {
	case "aria2.tellActive":
		return f.active, nil
	case "aria2.tellWaiting":
		return f.waiting, nil
	case "aria2.tellStopped":
		return f.stopped, nil
	case "aria2.getGlobalStat":
		return f.stat, nil
	case "aria2.getVersion":
		return map[string]any{"version": f.version}, nil
	case "aria2.addUri":
		if uris, ok := params[1].([]any); ok && len(uris) > 0 {
			if uri, ok := uris[0].(string); ok {
				f.addedURIs = append(f.addedURIs, uri)
			}
		}
		f.gidSeq++
		return "gid-" + strconv.Itoa(f.gidSeq), nil
	default:
		return "OK", nil
	}
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
			value, err := f.dispatch(call.MethodName, decodeRawParams(call.Params))
			if err != nil {
				entries = append(entries, map[string]any{"faultCode": -32000, "faultString": err.Error()})
				continue
			}
			entries = append(entries, []any{value})
		}
		writeTestJSON(w, map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": entries})
		return
	}

	value, err := f.dispatch(request.Method, decodeRawParams(request.Params))
	if err != nil {
		writeTestJSON(w, map[string]any{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"error":   map[string]any{"code": -32000, "message": err.Error()},
		})
		return
	}
	writeTestJSON(w, map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": value})
}

func decodeRawParams(raw []json.RawMessage) []any {
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

func writeTestJSON(w http.ResponseWriter, payload any) {
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(payload)
}

// taskJSON builds one aria2 task object; every numeric field is a string.
func taskJSON(gid, name, status string, total, completed, speed uint64, connections int) map[string]any {
	return map[string]any{
		"gid":             gid,
		"status":          status,
		"totalLength":     strconv.FormatUint(total, 10),
		"completedLength": strconv.FormatUint(completed, 10),
		"downloadSpeed":   strconv.FormatUint(speed, 10),
		"connections":     strconv.Itoa(connections),
		"errorCode":       "0",
		"errorMessage":    "",
		"files": []map[string]any{
			{"path": "/downloads/" + name, "uris": []map[string]any{{"uri": "http://example.org/" + name}}},
		},
	}
}

// harness drives the app the way the program does: every command produced by
// an Update is executed and its message fed back in.
type harness struct {
	t    *testing.T
	app  *App
	fake *fakeRPC
	quit bool
}

func newHarness(t *testing.T, fake *fakeRPC, initial []string) *harness {
	t.Helper()

	return newHarnessWithConfig(t, fake, defaultTestConfig(), initial)
}

func defaultTestConfig() config.Config {
	return config.Config{
		DownloadDir:            "/downloads",
		MaxConcurrentDownloads: config.DefaultConcurrentDownloads,
		Theme:                  "dark",
	}
}

func newHarnessWithConfig(t *testing.T, fake *fakeRPC, cfg config.Config, initial []string) *harness {
	t.Helper()

	app := newApp(context.Background(), Deps{
		Client:       fake.client(),
		Store:        config.NewStore(filepath.Join(t.TempDir(), "aria2-tui.json")),
		Config:       cfg,
		InitialURIs:  initial,
		Aria2Version: "1.37.0",
	})
	app.resize(100, 20)

	h := &harness{t: t, app: app, fake: fake}
	h.drain(app.Init())
	return h
}

func (h *harness) send(msg tea.Msg) *harness {
	h.t.Helper()
	if h.quit {
		h.t.Fatalf("message %T sent after the app requested a quit", msg)
	}
	model, cmd := h.app.Update(msg)
	app, ok := model.(*App)
	if !ok {
		h.t.Fatalf("Update(%T) returned %T, want *App", msg, model)
	}
	h.app = app
	h.drain(cmd)
	return h
}

// drain executes commands, feeding the resulting messages back to the app.
func (h *harness) drain(cmd tea.Cmd) {
	h.t.Helper()

	queue := []tea.Cmd{cmd}
	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]
		if next == nil {
			continue
		}

		msg := h.run(next)
		if msg == nil {
			continue
		}

		switch m := msg.(type) {
		case tea.BatchMsg:
			queue = append(queue, m...)
			continue
		case tea.QuitMsg, tea.InterruptMsg:
			h.quit = true
			continue
		case cursor.BlinkMsg, tickMsg:
			continue
		}

		model, follow := h.app.Update(msg)
		if app, ok := model.(*App); ok {
			h.app = app
		}
		if _, isPoll := msg.(snapshotMsg); isPoll {
			// A snapshot only schedules the next poll timer.
			continue
		}
		queue = append(queue, follow)
	}
}

// run executes one command; a command that outlives commandTimeout (the poll
// timer) is abandoned.
func (h *harness) run(cmd tea.Cmd) tea.Msg {
	h.t.Helper()

	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()

	select {
	case msg := <-done:
		return msg
	case <-time.After(commandTimeout):
		return nil
	}
}

// key sends a printable key press.
func (h *harness) key(r rune) *harness {
	return h.send(tea.KeyPressMsg{Code: r, Text: string(r)})
}

// ctrl sends a control combination such as ctrl+s.
func (h *harness) ctrl(r rune) *harness {
	return h.send(tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl})
}

// special sends a named key such as tea.KeyUp or tea.KeyPgDown.
func (h *harness) special(code rune) *harness {
	return h.send(tea.KeyPressMsg{Code: code})
}

// paste sends bracketed paste content.
func (h *harness) paste(content string) *harness {
	return h.send(tea.PasteMsg{Content: content})
}

func (h *harness) resize(width, height int) *harness {
	return h.send(tea.WindowSizeMsg{Width: width, Height: height})
}

// refresh forces a poll round through the real HTTP path.
func (h *harness) refresh() *harness {
	return h.send(tickMsg{})
}

func (h *harness) view() string {
	return h.app.View().Content
}
