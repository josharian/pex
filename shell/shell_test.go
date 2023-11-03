package shell

import (
	"log/slog"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	if testing.Verbose() {
		h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})
		sl := slog.New(h)
		slog.SetDefault(sl)
	}

	tests := []struct {
		in     string
		want   []Command
		pipes  []int
		errsub string
	}{
		{
			in: "sed 's/e/r/g' | grep x",
			want: []Command{
				{
					Argv: []string{"sed", "s/e/r/g"},
					Raw:  "sed 's/e/r/g'",
				},
				{
					Argv: []string{"grep", "x"},
					Raw:  "grep x",
				},
			},
			pipes: []int{14},
		},
		{
			in: "sed 's/e/r/g' | grep x | wc -l",
			want: []Command{
				{
					Argv: []string{"sed", "s/e/r/g"},
					Raw:  "sed 's/e/r/g'",
				},
				{
					Argv: []string{"grep", "x"},
					Raw:  "grep x",
				},
				{
					Argv: []string{"wc", "-l"},
					Raw:  "wc -l",
				},
			},
			pipes: []int{14, 23},
		},
		{
			in: `echo \\ '\\' "\"" |`,
			want: []Command{
				{
					Argv: []string{"echo", `\`, `\\`, `"`},
					Raw:  `echo \\ '\\' "\""`,
				},
				{
					Argv: nil,
					Raw:  "",
				},
			},
			pipes: []int{18},
		},
		{
			in: "go test -json -race ./... | jq -s 'map(select(.Test != null)) | sort_by(.Elapsed)'",
			want: []Command{
				{
					Argv: []string{"go", "test", "-json", "-race", "./..."},
					Raw:  "go test -json -race ./...",
				},
				{
					Argv: []string{"jq", "-s", "map(select(.Test != null)) | sort_by(.Elapsed)"},
					Raw:  "jq -s 'map(select(.Test != null)) | sort_by(.Elapsed)'",
				},
			},
			pipes: []int{26},
		},
		{
			in: "grep x | ",
			want: []Command{
				{
					Argv: []string{"grep", "x"},
					Raw:  "grep x",
				},
				{
					Argv: nil,
					Raw:  " ",
				},
			},
			pipes: []int{7},
		},
		{
			in: "grep x |",
			want: []Command{
				{
					Argv: []string{"grep", "x"},
					Raw:  "grep x",
				},
				{
					Argv: nil,
					Raw:  "",
				},
			},
			pipes: []int{7},
		},
		{
			in: "grep x| ",
			want: []Command{
				{
					Argv: []string{"grep", "x"},
					Raw:  "grep x",
				},
				{
					Argv: nil,
					Raw:  " ",
				},
			},
			pipes: []int{6},
		},
		{
			in:     "|",
			errsub: "missing statement before |",
		},
		{
			in:     "   |",
			errsub: "missing statement before |",
		},
		{
			in:     "|   ",
			errsub: "missing statement before |",
		},
		// TODO: unicode tests
		{
			in:     "if true; then echo hi; fi",
			errsub: "if statements are not supported",
		},
		{
			in:     "echo hi > /dev/null",
			errsub: "redirects are not supported",
		},
		{
			in:     "grep x #| ",
			errsub: "comments are not supported",
		},
		{
			in:     "grep x || foo",
			errsub: "|| is not supported",
		},
		{
			in:     "grep x && foo",
			errsub: "&& is not supported",
		},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if tt.want != nil && tt.errsub != "" {
				t.Fatalf("bad test: want and errsub both set")
			}
			got, pipes, err := Parse(tt.in)
			if err != nil {
				if tt.errsub == "" {
					t.Fatalf("parseShell(%q) = %#v, %v, %v; want %#v, <nil>", tt.in, got, pipes, err, tt.want)
				}
				if !strings.Contains(err.Error(), tt.errsub) {
					t.Fatalf("parseShell(%q) = %#v, %v, %v; want error to contain %v", tt.in, got, pipes, err, tt.errsub)
				}
				return
			}
			if tt.errsub != "" {
				t.Fatalf("parseShell(%q) = %#v, %v, %v; want error containing %v", tt.in, got, pipes, err, tt.errsub)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseShell(%q).commands = %#v, want %#v", tt.in, got, tt.want)
			}
			if !reflect.DeepEqual(pipes, tt.pipes) {
				t.Fatalf("parseShell(%q).pipes = %#v, want %#v", tt.in, pipes, tt.pipes)
			}
			for _, p := range pipes {
				if tt.in[p] != '|' {
					t.Fatalf("pipe offset %v contains a non-pipe %v", p, tt.in[p])
				}
			}
			if t.Failed() {
				return
			}
			t.Logf("parseShell(%q)", tt.in)
			for _, arg := range tt.want {
				t.Logf("\tcommands: %q => %q", arg.Raw, arg.Argv)
			}
		})
	}
}
