package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestPreviewBuild proves that the preview's seed, its sign-in route, its links to the list of people, and
// its outbox of the emails it keeps instead of sending them, with the page that shows them, are only in
// the app built with the preview build tag. The default build uses the tags the Dockerfile builds the app with. It checks two things for each build, and checks the preview build too, so that
// a check which could never find anything would fail here instead of passing:
//   - whether the app depends on the preview package at all, from go list, and
//   - whether the built binary holds the route's path, the made-up group's name, a line of its posts, the
//     words of the links that pages show to switch person and to sign in, and the path, the heading, and
//     a line of the log of the outbox.
func TestPreviewBuild(t *testing.T) {
	for _, test := range []struct {
		name     string
		tags     []string
		expected bool
	}{
		{name: "the default build has neither the seed, nor the sign-in route, nor the links to the list of people, nor the outbox", tags: []string{"-tags", "sqlite_fts5,sqlite_math_functions"}, expected: false},
		{name: "the preview build has them all, so the checks can see them", tags: []string{"-tags", "preview,sqlite_fts5,sqlite_math_functions"}, expected: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			deps := goCommand(t, append(append([]string{"list", "-deps", "-f", "{{.ImportPath}}"}, test.tags...), ".")...)
			if got := slices.Contains(strings.Fields(string(deps)), "app/preview"); got != test.expected {
				t.Fatalf("expected the app to depend on app/preview to be %v, but it was %v", test.expected, got)
			}

			binary := filepath.Join(t.TempDir(), "app")
			goCommand(t, append(append([]string{"build", "-o", binary}, test.tags...), ".")...)
			contents, err := os.ReadFile(binary)
			if err != nil {
				t.Fatal(err)
			}
			for _, marker := range []string{"/preview/sign-in", "The Loft", "a noise like a tired cow", "Switch person", "Choose someone to sign in as", "/preview/emails", "Emails the preview kept", "instead of sending it"} {
				if got := bytes.Contains(contents, []byte(marker)); got != test.expected {
					t.Fatalf("expected the binary to contain %q to be %v, but it was %v", marker, test.expected, got)
				}
			}
		})
	}
}

func goCommand(t *testing.T, args ...string) []byte {
	t.Helper()

	out, err := exec.CommandContext(t.Context(), "go", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("go %v: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}
