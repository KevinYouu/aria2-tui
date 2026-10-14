package aria2

import (
	"strconv"
	"strings"
)

// Status is the lifecycle state of a download as reported by aria2.
type Status string

const (
	StatusActive   Status = "active"
	StatusWaiting  Status = "waiting"
	StatusPaused   Status = "paused"
	StatusComplete Status = "complete"
	StatusError    Status = "error"
	StatusRemoved  Status = "removed"
)

// Task is a single download. aria2 reports every numeric field as a string;
// parsing happens here so no raw string reaches the UI.
type Task struct {
	GID          string
	Name         string
	Status       Status
	Total        uint64 // total bytes
	Completed    uint64 // downloaded bytes
	Speed        uint64 // bytes per second
	Connections  int
	ErrorCode    int
	ErrorMessage string
}

// Snapshot is one consistent view of the daemon.
type Snapshot struct {
	Active  []Task
	Waiting []Task
	Stopped []Task
	Stat    GlobalStat
}

// Tasks returns every download in display order: active, waiting, then stopped.
func (s Snapshot) Tasks() []Task {
	tasks := make([]Task, 0, len(s.Active)+len(s.Waiting)+len(s.Stopped))
	tasks = append(tasks, s.Active...)
	tasks = append(tasks, s.Waiting...)
	tasks = append(tasks, s.Stopped...)
	return tasks
}

// InProgress counts the downloads that are still transferring or resumable.
func (s Snapshot) InProgress() int {
	count := 0
	for _, task := range s.Tasks() {
		if task.Status == StatusActive || task.Status == StatusPaused {
			count++
		}
	}
	return count
}

// GlobalStat is the daemon wide statistics from aria2.getGlobalStat.
type GlobalStat struct {
	DownloadSpeed uint64
	UploadSpeed   uint64
	NumActive     int
	NumWaiting    int
	NumStopped    int
}

// rawTask mirrors aria2's wire format, where every numeric field is a string.
type rawTask struct {
	GID             string         `json:"gid"`
	Status          string         `json:"status"`
	TotalLength     string         `json:"totalLength"`
	CompletedLength string         `json:"completedLength"`
	DownloadSpeed   string         `json:"downloadSpeed"`
	Connections     string         `json:"connections"`
	ErrorCode       string         `json:"errorCode"`
	ErrorMessage    string         `json:"errorMessage"`
	Files           []rawFile      `json:"files"`
	Bittorrent      *rawBittorrent `json:"bittorrent"`
}

type rawFile struct {
	Path string   `json:"path"`
	Uris []rawURI `json:"uris"`
}

type rawURI struct {
	URI string `json:"uri"`
}

type rawBittorrent struct {
	Info struct {
		Name string `json:"name"`
	} `json:"info"`
}

func (r rawTask) task() Task {
	return Task{
		GID:          r.GID,
		Name:         r.name(),
		Status:       Status(r.Status),
		Total:        parseUint(r.TotalLength),
		Completed:    parseUint(r.CompletedLength),
		Speed:        parseUint(r.DownloadSpeed),
		Connections:  parseInt(r.Connections),
		ErrorCode:    parseInt(r.ErrorCode),
		ErrorMessage: r.ErrorMessage,
	}
}

// name resolves the best available display name: the target file name, then
// the torrent name, then the URI (which the UI truncates to the row width).
func (r rawTask) name() string {
	if len(r.Files) > 0 {
		if base := baseName(r.Files[0].Path); base != "" {
			return base
		}
	}
	if r.Bittorrent != nil {
		if name := strings.TrimSpace(r.Bittorrent.Info.Name); name != "" {
			return name
		}
	}
	if len(r.Files) > 0 && len(r.Files[0].Uris) > 0 {
		return r.Files[0].Uris[0].URI
	}
	return ""
}

// baseName returns the last path element, accepting both separators so that
// Windows style paths from a remote daemon are handled too.
func baseName(path string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(path), `/\`)
	if trimmed == "" {
		return ""
	}
	if idx := strings.LastIndexAny(trimmed, `/\`); idx >= 0 {
		trimmed = trimmed[idx+1:]
	}
	switch trimmed {
	case "", ".", "..":
		return ""
	}
	return trimmed
}

func parseUint(value string) uint64 {
	parsed, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func parseInt(value string) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return parsed
}

// rawGlobalStat mirrors the string valued response of aria2.getGlobalStat.
type rawGlobalStat struct {
	DownloadSpeed string `json:"downloadSpeed"`
	UploadSpeed   string `json:"uploadSpeed"`
	NumActive     string `json:"numActive"`
	NumWaiting    string `json:"numWaiting"`
	NumStopped    string `json:"numStopped"`
}

func (r rawGlobalStat) stat() GlobalStat {
	return GlobalStat{
		DownloadSpeed: parseUint(r.DownloadSpeed),
		UploadSpeed:   parseUint(r.UploadSpeed),
		NumActive:     parseInt(r.NumActive),
		NumWaiting:    parseInt(r.NumWaiting),
		NumStopped:    parseInt(r.NumStopped),
	}
}
