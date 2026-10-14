package aria2

import "testing"

func TestRawTaskParsesTypedFields(t *testing.T) {
	tests := []struct {
		name string
		raw  rawTask
		want Task
	}{
		{
			name: "full http task",
			raw: rawTask{
				GID:             "2089b05ecca3d829",
				Status:          "active",
				TotalLength:     "1048576",
				CompletedLength: "524288",
				DownloadSpeed:   "131072",
				Connections:     "8",
				ErrorCode:       "0",
				Files:           []rawFile{{Path: "/downloads/ubuntu.iso", Uris: []rawURI{{URI: "http://example.org/ubuntu.iso"}}}},
			},
			want: Task{
				GID:         "2089b05ecca3d829",
				Name:        "ubuntu.iso",
				Status:      StatusActive,
				Total:       1048576,
				Completed:   524288,
				Speed:       131072,
				Connections: 8,
			},
		},
		{
			name: "missing fields stay zero",
			raw:  rawTask{GID: "abc"},
			want: Task{GID: "abc"},
		},
		{
			name: "empty numeric strings stay zero",
			raw: rawTask{
				GID:             "empty",
				Status:          "complete",
				TotalLength:     "",
				CompletedLength: "",
				DownloadSpeed:   "",
				Connections:     "",
			},
			want: Task{GID: "empty", Status: StatusComplete},
		},
		{
			name: "torrent falls back to the info name",
			raw: rawTask{
				GID:    "torrent",
				Status: "active",
				Files:  []rawFile{{Path: "", Uris: []rawURI{{URI: "magnet:?xt=urn:btih:deadbeef"}}}},
				Bittorrent: &rawBittorrent{
					Info: struct {
						Name string `json:"name"`
					}{Name: "debian-13.iso"},
				},
			},
			want: Task{GID: "torrent", Name: "debian-13.iso", Status: StatusActive},
		},
		{
			name: "falls back to the uri before metadata arrives",
			raw: rawTask{
				GID:    "magnet",
				Status: "active",
				Files:  []rawFile{{Path: "", Uris: []rawURI{{URI: "magnet:?xt=urn:btih:deadbeef"}}}},
			},
			want: Task{GID: "magnet", Name: "magnet:?xt=urn:btih:deadbeef", Status: StatusActive},
		},
		{
			name: "windows style path is reduced to its base name",
			raw: rawTask{
				GID:   "win",
				Files: []rawFile{{Path: `C:\Users\kevin\Downloads\file.zip`}},
			},
			want: Task{GID: "win", Name: "file.zip"},
		},
		{
			name: "directory target keeps the last segment",
			raw: rawTask{
				GID:   "dir",
				Files: []rawFile{{Path: "/downloads/archive/", Uris: []rawURI{{URI: "http://example.org/archive.torrent"}}}},
			},
			want: Task{GID: "dir", Name: "archive"},
		},
		{
			name: "error details are kept",
			raw: rawTask{
				GID:          "failed",
				Status:       "error",
				ErrorCode:    "3",
				ErrorMessage: "Resource not found",
			},
			want: Task{GID: "failed", Status: StatusError, ErrorCode: 3, ErrorMessage: "Resource not found"},
		},
		{
			name: "garbage numbers do not fail the parse",
			raw:  rawTask{GID: "garbage", TotalLength: "unknown", Connections: "-"},
			want: Task{GID: "garbage"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.raw.task(); got != tt.want {
				t.Errorf("rawTask.task() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestRawGlobalStatParsesTypedFields(t *testing.T) {
	stat := rawGlobalStat{
		DownloadSpeed: "123456",
		UploadSpeed:   "2048",
		NumActive:     "2",
		NumWaiting:    "3",
		NumStopped:    "4",
	}.stat()

	want := GlobalStat{DownloadSpeed: 123456, UploadSpeed: 2048, NumActive: 2, NumWaiting: 3, NumStopped: 4}
	if stat != want {
		t.Errorf("rawGlobalStat.stat() = %+v, want %+v", stat, want)
	}
}

func TestSnapshotTasksOrdering(t *testing.T) {
	snapshot := Snapshot{
		Active:  []Task{{GID: "a", Status: StatusActive}},
		Waiting: []Task{{GID: "b", Status: StatusWaiting}},
		Stopped: []Task{{GID: "c", Status: StatusComplete}},
	}

	tasks := snapshot.Tasks()
	if len(tasks) != 3 {
		t.Fatalf("len(Tasks()) = %d, want 3", len(tasks))
	}
	for i, wantGID := range []string{"a", "b", "c"} {
		if tasks[i].GID != wantGID {
			t.Errorf("Tasks()[%d].GID = %q, want %q", i, tasks[i].GID, wantGID)
		}
	}
}

func TestSnapshotInProgress(t *testing.T) {
	tests := []struct {
		name     string
		snapshot Snapshot
		want     int
	}{
		{name: "empty", snapshot: Snapshot{}},
		{
			name: "counts active and paused only",
			snapshot: Snapshot{
				Active:  []Task{{Status: StatusActive}, {Status: StatusActive}},
				Waiting: []Task{{Status: StatusWaiting}, {Status: StatusPaused}},
				Stopped: []Task{{Status: StatusComplete}, {Status: StatusError}, {Status: StatusRemoved}},
			},
			want: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.snapshot.InProgress(); got != tt.want {
				t.Errorf("InProgress() = %d, want %d", got, tt.want)
			}
		})
	}
}
