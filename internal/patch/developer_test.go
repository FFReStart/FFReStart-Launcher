package patch

import (
	"archive/zip"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestDeveloperChannelURLPinning(t *testing.T) {
	t.Parallel()
	tests := []struct {
		value   string
		initial bool
		want    bool
	}{
		{DeveloperVersionURL, true, true},
		{DeveloperArchiveURL, true, true},
		{"http://github.com/FFReStart/FFReStart_Builds/releases/latest/download/version.txt", true, false},
		{"https://github.com/FFReStart/FFReStart_Builds/releases/latest/download/other.zip", true, false},
		{DeveloperVersionURL + "?mirror=1", true, false},
		{"https://github.com:444/FFReStart/FFReStart_Builds/releases/latest/download/version.txt", true, false},
		{"https://github.com.evil.example/FFReStart/FFReStart_Builds/releases/latest/download/version.txt", true, false},
		{"https://release-assets.githubusercontent.com/asset", false, true},
		{"https://github.com/FFReStart/FFReStart_Builds/releases/download/v0.0.21/version.txt", false, true},
		{"https://github.com/another/path", false, false},
		{"https://example.com/asset", false, false},
	}
	for _, test := range tests {
		parsed, err := url.Parse(test.value)
		if err != nil {
			t.Fatal(err)
		}
		if got := allowedDeveloperURL(parsed, test.initial); got != test.want {
			t.Errorf("allowedDeveloperURL(%q, %v) = %v, want %v", test.value, test.initial, got, test.want)
		}
	}
}

func TestExtractDeveloperZipRejectsTraversal(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	archivePath := filepath.Join(root, "unsafe.zip")
	file, err := os.Create(archivePath) // #nosec G304 -- path is beneath t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create("../escaped.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("escape")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	destination := filepath.Join(root, "staging")
	if err := extractDeveloperZip(archivePath, destination); err == nil {
		t.Fatal("expected traversal rejection")
	}
	if _, err := os.Stat(filepath.Join(root, "escaped.txt")); !os.IsNotExist(err) {
		t.Fatalf("archive escaped staging: %v", err)
	}
}

func TestPromoteDeveloperBuildRetainsRollback(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	target := filepath.Join(root, developerBuildDir)
	staging := filepath.Join(root, "staged", developerBuildDir)
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(staging, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "old.txt"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "new.txt"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := promoteDeveloperBuild(root, staging, "0.0.21"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(target, "new.txt")); err != nil {
		t.Fatalf("new build was not promoted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target+".previous", "old.txt")); err != nil {
		t.Fatalf("old build was not retained: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, developerVersion)) // #nosec G304 -- path is beneath t.TempDir.
	if err != nil || string(data) != "0.0.21" {
		t.Fatalf("Version.txt = %q, %v", data, err)
	}
}
