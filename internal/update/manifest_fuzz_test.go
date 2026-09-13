package update

import "testing"

func FuzzParseManifest(f *testing.F) {
	f.Add([]byte(`{"version":"v1.2.3","url":"https://updates.example/launcher","sha256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","key_id":"test-only","signature":"AA"}`))
	f.Add([]byte(`{}`))
	f.Add([]byte{0xff, 0x00, '{'})
	f.Fuzz(func(t *testing.T, data []byte) {
		manifest, err := ParseManifest(data)
		if err != nil {
			return
		}
		if manifest.Version == "" || manifest.URL == "" || manifest.KeyID == "" {
			t.Fatal("parser accepted an incomplete manifest")
		}
	})
}
