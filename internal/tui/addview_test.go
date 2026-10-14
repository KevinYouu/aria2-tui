package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestValidateURI(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{name: "https", in: "https://example.org/file.iso"},
		{name: "http", in: "http://example.org/file.iso"},
		{name: "ftp", in: "ftp://example.org/file.iso"},
		{name: "magnet", in: "magnet:?xt=urn:btih:deadbeef&dn=file"},
		{name: "uppercase scheme", in: "HTTPS://example.org/file.iso"},
		{name: "surrounding whitespace", in: "  https://example.org/file.iso  "},
		{name: "empty", in: "", wantErr: true},
		{name: "whitespace only", in: "   ", wantErr: true},
		{name: "no scheme", in: "example.org/file.iso", wantErr: true},
		{name: "prose", in: "bunny rabbits", wantErr: true},
		{name: "unsupported scheme", in: "file:///etc/hosts", wantErr: true},
		{name: "opaque non magnet scheme", in: "sftp://example.org/file.iso", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateURI(tt.in)
			if tt.wantErr && err == nil {
				t.Fatalf("ValidateURI(%q) error = nil, want an error", tt.in)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("ValidateURI(%q) error = %v", tt.in, err)
			}
		})
	}
}

func TestValidateURIMessageNamesTheScheme(t *testing.T) {
	err := ValidateURI("sftp://example.org/file.iso")
	if err == nil {
		t.Fatal("ValidateURI() error = nil, want an error")
	}
	for _, want := range []string{"sftp", "http", "https", "ftp", "magnet"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("ValidateURI() error = %q, want it to mention %q", err, want)
		}
	}
}

func TestParseURIs(t *testing.T) {
	tests := []struct {
		name        string
		in          string
		wantLinks   int
		wantInvalid int
	}{
		{name: "empty text"},
		{name: "one link", in: "https://example.org/a", wantLinks: 1},
		{
			name:      "one per line with blank lines",
			in:        "https://example.org/a\n\n  \nftp://example.org/b\n",
			wantLinks: 2,
		},
		{
			name:        "counts invalid lines",
			in:          "https://example.org/a\nbunny rabbits\nfile:///etc/hosts\n",
			wantLinks:   1,
			wantInvalid: 2,
		},
		{
			name:        "only invalid lines",
			in:          "one\ntwo\n",
			wantInvalid: 2,
		},
		{
			name:      "keeps duplicates",
			in:        "  https://example.org/a  \nhttps://example.org/a\n",
			wantLinks: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			links, invalid := ParseURIs(tt.in)
			if len(links) != tt.wantLinks {
				t.Errorf("ParseURIs(%q) links = %v, want %d", tt.in, links, tt.wantLinks)
			}
			if invalid != tt.wantInvalid {
				t.Errorf("ParseURIs(%q) invalid = %d, want %d", tt.in, invalid, tt.wantInvalid)
			}
		})
	}
}

func TestParseURIsReturnsTrimmedLinks(t *testing.T) {
	links, invalid := ParseURIs("  https://example.org/a  \n")
	if invalid != 0 {
		t.Fatalf("invalid = %d, want 0", invalid)
	}
	if len(links) != 1 || links[0] != "https://example.org/a" {
		t.Errorf("links = %v, want the trimmed link", links)
	}
}

func TestAddViewRendersFooterAndHelpBar(t *testing.T) {
	model := newAddModel()

	view := ansi.Strip(model.view(80, 20, footerLine{text: "boom", kind: bannerError, set: true}))
	for _, want := range []string{"boom", "[ctrl+s] download", "[esc] back"} {
		if !strings.Contains(view, want) {
			t.Errorf("view = %q, want it to contain %q", view, want)
		}
	}
}

func TestAddViewAcceptsPastedContent(t *testing.T) {
	model := newAddModel()

	model.update(tea.PasteMsg{Content: "https://example.org/a\nhttps://example.org/b"})

	links, invalid := model.links()
	if len(links) != 2 || invalid != 0 {
		t.Errorf("links = %v invalid = %d, want the two pasted links", links, invalid)
	}
}

func TestAddViewSubmitsOnlyOnControlKeys(t *testing.T) {
	tests := []struct {
		name string
		key  tea.KeyPressMsg
		want addAction
	}{
		{name: "ctrl+s submits", key: tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}, want: addSubmit},
		{name: "esc goes back", key: tea.KeyPressMsg{Code: tea.KeyEscape}, want: addBack},
		{name: "ctrl+c asks to quit", key: tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}, want: addQuit},
		{name: "typing stays in the box", key: tea.KeyPressMsg{Code: 'a', Text: "a"}, want: addNoop},
		{name: "enter inserts a newline", key: tea.KeyPressMsg{Code: tea.KeyEnter}, want: addNoop},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := newAddModel()
			got, _ := model.update(tt.key)
			if got != tt.want {
				t.Errorf("update(%v) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}
