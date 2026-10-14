package tui

import (
	"context"
	"errors"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/KevinYouu/aria2-tui/internal/aria2"
	"github.com/KevinYouu/aria2-tui/internal/config"
)

const (
	appTitle = "aria2-tui"

	// snapshotInterval is the poll period. The timer is re-armed only after a
	// snapshot has arrived, so a slow daemon cannot pile up requests.
	snapshotInterval = time.Second
	// rpcTimeout bounds a single UI triggered RPC.
	rpcTimeout = 10 * time.Second
	// addTimeout bounds submitting pasted links, which can take longer than a
	// status query while aria2 fetches torrent or metalink metadata.
	addTimeout = 30 * time.Second

	// defaultTermWidth and defaultTermHeight keep the views sane before the
	// terminal reports its size; the tests rely on them too.
	defaultTermWidth  = 80
	defaultTermHeight = 24

	// quitPrompt is the inline quit confirmation. The [y/N] hint leads so it
	// survives truncation on narrow terminals.
	quitPrompt = "[y/N] Quit? aria2c stops; partial files and .aria2 files stay on disk and resume when re-added."
)

// Deps are the app's collaborators, all owned and outliving the app.
type Deps struct {
	Client       *aria2.Client
	Store        *config.Store
	Config       config.Config
	InitialURIs  []string
	DaemonDone   <-chan struct{}
	Aria2Version string
}

type viewKind int

const (
	viewList viewKind = iota
	viewAdd
)

// Run starts the TUI and returns once the user quits. ErrInterrupted and
// ErrProgramKilled are the framework's own signal paths.
func Run(ctx context.Context, deps Deps) error {
	program := tea.NewProgram(newApp(ctx, deps), tea.WithContext(ctx))
	if _, err := program.Run(); err != nil &&
		!errors.Is(err, tea.ErrInterrupted) &&
		!errors.Is(err, tea.ErrProgramKilled) {
		return err
	}
	return nil
}

// Messages exchanged between the app and its commands.
type (
	snapshotMsg struct {
		snapshot aria2.Snapshot
		err      error
	}
	actionMsg struct {
		err    error
		notice string
	}
	concurrencyMsg struct {
		value int
		err   error
	}
	daemonExitMsg struct{}
	tickMsg       struct{}
)

// App is the single long lived model: it owns the shared state, the polling
// schedule and the two views.
type App struct {
	ctx    context.Context
	deps   Deps
	config config.Config

	list *listModel
	add  *addModel
	view viewKind

	width, height int
	snapshot      aria2.Snapshot

	banner     string
	bannerKind bannerKind
	// confirmQuit is set while the inline quit confirmation is waiting for an
	// answer.
	confirmQuit bool

	// awaitingFirstSnapshot decides whether an empty list should open the
	// paste view instead.
	awaitingFirstSnapshot bool
	daemonGone            bool
}

func newApp(ctx context.Context, deps Deps) *App {
	app := &App{
		ctx:        ctx,
		deps:       deps,
		config:     deps.Config,
		list:       newListModel(),
		add:        newAddModel(),
		view:       viewList,
		width:      defaultTermWidth,
		height:     defaultTermHeight,
		bannerKind: bannerInfo,
		daemonGone: false,
	}
	if len(deps.InitialURIs) == 0 {
		app.awaitingFirstSnapshot = true
	} else {
		app.setNotice(fmt.Sprintf("adding %d link(s)", len(deps.InitialURIs)))
	}
	return app
}

func (a *App) Init() tea.Cmd {
	cmds := make([]tea.Cmd, 0, 3)
	cmds = append(cmds, a.fetchSnapshotCmd())
	if len(a.deps.InitialURIs) > 0 {
		cmds = append(cmds, a.addLinksCmd(a.deps.InitialURIs, 0))
	}
	if a.deps.DaemonDone != nil {
		cmds = append(cmds, waitForDaemonCmd(a.deps.DaemonDone))
	}
	return tea.Batch(cmds...)
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.resize(msg.Width, msg.Height)
		return a, nil
	case tickMsg:
		return a, a.fetchSnapshotCmd()
	case snapshotMsg:
		return a, a.applySnapshot(msg)
	case actionMsg:
		a.applyAction(msg)
		return a, nil
	case concurrencyMsg:
		a.applyConcurrency(msg)
		return a, nil
	case daemonExitMsg:
		a.daemonGone = true
		a.setBanner(errors.New("aria2c exited; restart aria2-tui to continue downloading"), bannerError)
		return a, nil
	case tea.KeyPressMsg:
		return a, a.handleKey(msg)
	}

	if a.view == viewAdd {
		if _, cmd := a.add.update(msg); cmd != nil {
			return a, cmd
		}
	}
	return a, nil
}

func (a *App) View() tea.View {
	var content string
	switch a.view {
	case viewAdd:
		content = a.add.view(a.width, a.height, a.footer())
	default:
		content = a.list.view(a.listView())
	}

	view := tea.NewView(content)
	view.AltScreen = true
	view.WindowTitle = appTitle
	return view
}

func (a *App) resize(width, height int) {
	if width > 0 {
		a.width = width
	}
	if height > 0 {
		a.height = height
	}
}

// listView assembles the list rendering input.
func (a *App) listView() listView {
	return listView{
		tasks:          a.snapshot.Tasks(),
		stat:           a.snapshot.Stat,
		width:          a.width,
		height:         a.height,
		maxConnections: a.config.MaxConcurrentDownloads,
		aria2Version:   a.deps.Aria2Version,
		footer:         a.footer(),
	}
}

// footer is the shared single line message under the content: the quit
// confirmation takes precedence over the last error or notice.
func (a *App) footer() footerLine {
	if a.confirmQuit {
		return footerLine{text: quitPrompt, kind: bannerWarn, set: true}
	}
	if a.banner == "" {
		return footerLine{}
	}
	return footerLine{text: a.banner, kind: a.bannerKind, set: true}
}

// setBanner replaces the message line; an error always wins over a notice.
func (a *App) setBanner(err error, kind bannerKind) {
	if err == nil {
		a.banner = ""
		return
	}
	a.banner = err.Error()
	a.bannerKind = kind
}

// setNotice shows an informational message.
func (a *App) setNotice(notice string) {
	a.banner = notice
	a.bannerKind = bannerInfo
}

func (a *App) applyAction(msg actionMsg) {
	switch {
	case msg.err != nil:
		a.setBanner(msg.err, bannerError)
	case msg.notice != "":
		a.setNotice(msg.notice)
	}
}

// fetchSnapshotCmd queries the daemon; a failing snapshot keeps the last good
// one on screen and only updates the message line.
func (a *App) fetchSnapshotCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(a.ctx, rpcTimeout)
		defer cancel()
		snapshot, err := a.deps.Client.Snapshot(ctx)
		return snapshotMsg{snapshot: snapshot, err: err}
	}
}

// waitForDaemonCmd reports the daemon exit through the message loop.
func waitForDaemonCmd(done <-chan struct{}) tea.Cmd {
	return func() tea.Msg {
		<-done
		return daemonExitMsg{}
	}
}

// applySnapshot stores a successful snapshot and arms the next poll.
func (a *App) applySnapshot(msg snapshotMsg) tea.Cmd {
	if a.daemonGone {
		// The daemon is gone for good, so polling stops: a stream of transport
		// errors would otherwise bury the exit message within a second.
		return nil
	}

	if msg.err != nil {
		a.setBanner(msg.err, bannerError)
	} else {
		a.snapshot = msg.snapshot
		if a.bannerKind == bannerError {
			// The daemon answers again: the stale error no longer applies.
			a.banner = ""
		}
		if a.awaitingFirstSnapshot {
			a.awaitingFirstSnapshot = false
			if len(a.snapshot.Tasks()) == 0 {
				a.view = viewAdd
			}
		}
		a.list.sync(a.snapshot.Tasks())
	}

	return tea.Tick(snapshotInterval, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

func (a *App) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	// The quit confirmation captures the next key: anything but y cancels it.
	if a.confirmQuit {
		if msg.String() == "y" {
			return tea.Quit
		}
		a.confirmQuit = false
		return nil
	}

	if a.view == viewAdd {
		action, cmd := a.add.update(msg)
		switch action {
		case addSubmit:
			return a.submitLinks()
		case addBack:
			a.view = viewList
			return nil
		case addQuit:
			return a.requestQuit()
		}
		return cmd
	}
	return a.handleListKey(msg)
}

func (a *App) handleListKey(msg tea.KeyPressMsg) tea.Cmd {
	geometry := listGeometryFor(a.height, a.footer().set)
	switch a.list.update(msg, a.snapshot.Tasks(), geometry.rows) {
	case listQuit:
		return a.requestQuit()
	case listAddLinks:
		a.view = viewAdd
		return nil
	case listTogglePause:
		return a.togglePauseCmd()
	case listRemove:
		return a.removeSelectedCmd()
	case listMoreConnections:
		return a.concurrencyCmd(1)
	case listFewerConnections:
		return a.concurrencyCmd(-1)
	}
	return nil
}

// requestQuit asks for confirmation while downloads are in progress.
func (a *App) requestQuit() tea.Cmd {
	if a.snapshot.InProgress() > 0 {
		a.confirmQuit = true
		return nil
	}
	return tea.Quit
}

// submitLinks hands the parsed links to aria2 and moves to the list view.
func (a *App) submitLinks() tea.Cmd {
	links, invalid := a.add.links()
	if len(links) == 0 {
		a.setBanner(errors.New("no valid links: paste http, https, ftp or magnet URLs, one per line"), bannerError)
		return nil
	}

	a.add.reset()
	a.view = viewList
	a.setNotice(fmt.Sprintf("adding %d link(s)", len(links)))

	return a.addLinksCmd(links, invalid)
}

// addLinksCmd submits the links and reports how many of the pasted lines were
// rejected along with the result.
func (a *App) addLinksCmd(links []string, invalid int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(a.ctx, addTimeout)
		defer cancel()

		gids, err := a.deps.Client.AddURIs(ctx, links, nil)
		if err != nil {
			return actionMsg{err: err}
		}

		added := 0
		for _, gid := range gids {
			if gid != "" {
				added++
			}
		}

		notice := fmt.Sprintf("added %d download(s)", added)
		if invalid > 0 {
			notice += fmt.Sprintf(" (%d invalid entr%s ignored)", invalid, plural(invalid, "y", "ies"))
		}
		return actionMsg{notice: notice}
	}
}

// togglePauseCmd pauses an active or waiting download and resumes a paused one.
func (a *App) togglePauseCmd() tea.Cmd {
	task, ok := a.list.selected(a.snapshot.Tasks())
	if !ok {
		return nil
	}
	gid := task.GID

	switch task.Status {
	case aria2.StatusActive, aria2.StatusWaiting:
		return a.actionCmd("paused "+noticeName(task), func(ctx context.Context) error {
			return a.deps.Client.Pause(ctx, gid)
		})
	case aria2.StatusPaused:
		return a.actionCmd("resumed "+noticeName(task), func(ctx context.Context) error {
			return a.deps.Client.Unpause(ctx, gid)
		})
	default:
		a.setNotice("only active, waiting or paused downloads can be paused")
		return nil
	}
}

// removeSelectedCmd removes an unfinished download and drops a finished one
// from the result list.
func (a *App) removeSelectedCmd() tea.Cmd {
	task, ok := a.list.selected(a.snapshot.Tasks())
	if !ok {
		return nil
	}
	gid := task.GID

	switch task.Status {
	case aria2.StatusComplete, aria2.StatusError, aria2.StatusRemoved:
		return a.actionCmd("removed "+noticeName(task), func(ctx context.Context) error {
			return a.deps.Client.PurgeResult(ctx, gid)
		})
	default:
		return a.actionCmd("removed "+noticeName(task), func(ctx context.Context) error {
			return a.deps.Client.Remove(ctx, gid)
		})
	}
}

// actionCmd runs one RPC and reports the outcome through the message line.
func (a *App) actionCmd(notice string, run func(context.Context) error) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(a.ctx, rpcTimeout)
		defer cancel()
		if err := run(ctx); err != nil {
			return actionMsg{err: err}
		}
		return actionMsg{notice: notice}
	}
}

// concurrencyCmd changes the parallelism limit optimistically, persists it and
// rolls back when aria2 rejects the change.
func (a *App) concurrencyCmd(delta int) tea.Cmd {
	next := clampInt(a.config.MaxConcurrentDownloads+delta, config.MinConcurrentDownloads, config.MaxConcurrentDownloads)
	if next == a.config.MaxConcurrentDownloads {
		return nil
	}
	previous := a.config.MaxConcurrentDownloads
	a.config.MaxConcurrentDownloads = next

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(a.ctx, rpcTimeout)
		defer cancel()

		if err := a.deps.Client.SetMaxConcurrent(ctx, next); err != nil {
			return concurrencyMsg{value: previous, err: err}
		}
		if err := a.deps.Store.Save(a.config); err != nil {
			return concurrencyMsg{value: next, err: err}
		}
		return concurrencyMsg{value: next}
	}
}

func (a *App) applyConcurrency(msg concurrencyMsg) {
	a.config.MaxConcurrentDownloads = msg.value
	if msg.err != nil {
		a.setBanner(msg.err, bannerError)
		return
	}
	a.setNotice(fmt.Sprintf("max concurrent downloads: %d", msg.value))
}

// plural picks the singular or plural suffix for count.
func plural(count int, singular, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}
