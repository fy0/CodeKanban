package terminal

import (
	"reflect"
	"testing"
)

func TestResizeSequenceDarwinSameSizeUsesRowNudge(t *testing.T) {
	got := resizeSequence("darwin", 120, 40, 120, 40)
	want := []terminalSize{{cols: 120, rows: 41}, {cols: 120, rows: 40}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestShouldNudgeSameSizeResizeOnlyForSupportedUnixPlatforms(t *testing.T) {
	for _, test := range []struct {
		goos string
		want bool
	}{
		{goos: "linux", want: true},
		{goos: "darwin", want: true},
		{goos: "windows", want: false},
	} {
		if got := shouldNudgeSameSizeResize(test.goos); got != test.want {
			t.Errorf("shouldNudgeSameSizeResize(%q) = %v, want %v", test.goos, got, test.want)
		}
	}
}
