package main

import (
	"reflect"
	"testing"
)

func TestFolderOpenCommand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		goos string
		want string
	}{
		{"windows", "explorer.exe"},
		{"darwin", "open"},
		{"linux", "xdg-open"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.goos, func(t *testing.T) {
			t.Parallel()
			command, arguments, err := folderOpenCommand(test.goos, `C:\game root`)
			if err != nil {
				t.Fatal(err)
			}
			if command != test.want || !reflect.DeepEqual(arguments, []string{`C:\game root`}) {
				t.Fatalf("got %q %q", command, arguments)
			}
		})
	}
	if _, _, err := folderOpenCommand("plan9", "/game"); err == nil {
		t.Fatal("expected unsupported operating system error")
	}
}
