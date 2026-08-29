package utils

import "testing"

func TestConsoleLineEndingForOS(t *testing.T) {
	tests := []struct {
		name string
		goos string
		want string
	}{
		{name: "windows", goos: "windows", want: "\r\n"},
		{name: "linux", goos: "linux", want: "\n"},
		{name: "darwin", goos: "darwin", want: "\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := consoleLineEndingForOS(test.goos); got != test.want {
				t.Fatalf("consoleLineEndingForOS(%q) = %q, want %q", test.goos, got, test.want)
			}
		})
	}
}
