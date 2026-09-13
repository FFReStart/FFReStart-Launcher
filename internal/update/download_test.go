package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestTwentyInterruptedDownloadsResume(t *testing.T) {
	payload := []byte(strings.Repeat("resumable-update-", 4096))
	sum := sha256.Sum256(payload)
	var mu sync.Mutex
	requests := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		id := request.URL.Query().Get("id")
		mu.Lock()
		requests[id]++
		attempt := requests[id]
		mu.Unlock()
		if attempt == 1 {
			response.Header().Set("Content-Length", strconv.Itoa(len(payload)))
			_, _ = response.Write(payload[:len(payload)/2])
			return
		}
		start := len(payload) / 2
		if request.Header.Get("Range") != fmt.Sprintf("bytes=%d-", start) {
			t.Errorf("range = %q", request.Header.Get("Range"))
		}
		response.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(payload)-1, len(payload)))
		response.WriteHeader(http.StatusPartialContent)
		_, _ = response.Write(payload[start:])
	}))
	defer server.Close()
	for index := range 20 {
		destination := filepath.Join(t.TempDir(), "launcher")
		artifactURL := fmt.Sprintf("%s/artifact?id=%d", server.URL, index)
		downloader := Downloader{}
		if err := downloader.Download(context.Background(), artifactURL, destination, int64(len(payload)), hex.EncodeToString(sum[:])); err == nil {
			t.Fatalf("run %d did not interrupt", index)
		}
		if err := downloader.Download(context.Background(), artifactURL, destination, int64(len(payload)), hex.EncodeToString(sum[:])); err != nil {
			t.Fatalf("run %d did not resume: %v", index, err)
		}
		got, err := os.ReadFile(destination) // #nosec G304 -- destination is inside t.TempDir.
		if err != nil || string(got) != string(payload) {
			t.Fatalf("run %d payload mismatch: %v", index, err)
		}
	}
}

func TestServerIgnoringRangeRestartsDownload(t *testing.T) {
	payload := []byte("complete replacement")
	sum := sha256.Sum256(payload)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) { _, _ = response.Write(payload) }))
	defer server.Close()
	destination := filepath.Join(t.TempDir(), "game.bin")
	if err := os.WriteFile(destination+".part", []byte("bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	record := progressRecord{URL: server.URL, Size: int64(len(payload)), SHA256: hex.EncodeToString(sum[:]), Downloaded: 3}
	data, _ := json.Marshal(record)
	if err := os.WriteFile(destination+".progress.json", data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := (Downloader{}).Download(context.Background(), server.URL, destination, int64(len(payload)), hex.EncodeToString(sum[:])); err != nil {
		t.Fatal(err)
	}
}

func TestCorruptedPartialIsDiscardedAfterHashFailure(t *testing.T) {
	payload := []byte("verified complete artifact")
	sum := sha256.Sum256(payload)
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests++
		if request.Header.Get("Range") != "" {
			start := 5
			response.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(payload)-1, len(payload)))
			response.WriteHeader(http.StatusPartialContent)
			_, _ = response.Write(payload[start:])
			return
		}
		_, _ = response.Write(payload)
	}))
	defer server.Close()
	destination := filepath.Join(t.TempDir(), "game.bin")
	if err := os.WriteFile(destination+".part", []byte("xxxxx"), 0o600); err != nil {
		t.Fatal(err)
	}
	record := progressRecord{URL: server.URL, Size: int64(len(payload)), SHA256: hex.EncodeToString(sum[:]), Downloaded: 5}
	data, _ := json.Marshal(record)
	if err := os.WriteFile(destination+".progress.json", data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := (Downloader{}).Download(context.Background(), server.URL, destination, int64(len(payload)), hex.EncodeToString(sum[:])); err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want resume then clean retry", requests)
	}
}
