package launch

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"strings"
	"testing"
	"time"
)

type captureStdinStarter struct {
	args    []string
	payload []byte
}

func protobufVarintField(t *testing.T, data []byte, wanted uint64) uint64 {
	t.Helper()
	for len(data) > 0 {
		tag, size := binary.Uvarint(data)
		if size <= 0 {
			t.Fatal("invalid protobuf tag")
		}
		data = data[size:]
		switch tag & 7 {
		case 0:
			value, read := binary.Uvarint(data)
			if read <= 0 {
				t.Fatal("invalid protobuf varint")
			}
			if tag>>3 == wanted {
				return value
			}
			data = data[read:]
		case 2:
			length, read := binary.Uvarint(data)
			if read <= 0 || uint64(len(data[read:])) < length {
				t.Fatal("invalid protobuf bytes field")
			}
			data = data[read+int(length):] // #nosec G115 -- length is bounded by the remaining slice.
		default:
			t.Fatalf("unsupported wire type %d", tag&7)
		}
	}
	t.Fatalf("protobuf varint field %d is missing", wanted)
	return 0
}

func requireIdenticalBytes(t *testing.T, got, want []byte) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("protobuf length differs: got %d bytes, want %d", len(got), len(want))
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("protobuf differs at byte %d: got 0x%02x, want 0x%02x", index, got[index], want[index])
		}
	}
}

func TestLaunchHandoffMatchesCrossLanguageGoldenFixture(t *testing.T) {
	fixture, err := os.ReadFile("testdata/launch-handoff.bin") // #nosec G304 -- repository-owned test fixture.
	if err != nil {
		t.Fatal(err)
	}
	// The TypeScript-generated canonical fixture is a raw LaunchHandoff. The
	// launcher intentionally adds the D09 length delimiter only on game stdin.
	if length, size := binary.Uvarint(fixture); size > 0 {
		fixturePayloadSize := uint64(len(fixture) - size) // #nosec G115 -- the 111-byte repository fixture is bounded.
		if length == fixturePayloadSize {
			t.Fatal("canonical fixture unexpectedly contains a length delimiter")
		}
	}
	nested := protobufBytesField(t, fixture, 2)
	bootstrap := LaunchBootstrap{
		RealmID:        string(protobufBytesField(t, nested, 1)),
		WorldEndpoint:  string(protobufBytesField(t, nested, 2)),
		GNSCAKeyID:     string(protobufBytesField(t, nested, 3)),
		ProtocolMin:    uint32(protobufVarintField(t, nested, 4)), // #nosec G115 -- fixture values are contract uint32 fields.
		ProtocolMax:    uint32(protobufVarintField(t, nested, 5)), // #nosec G115 -- fixture values are contract uint32 fields.
		ContentVersion: string(protobufBytesField(t, nested, 6)),
	}
	encoded, err := marshalDelimitedHandoff(protobufBytesField(t, fixture, 1), bootstrap)
	if err != nil {
		t.Fatal(err)
	}
	messageLength, prefixSize := binary.Uvarint(encoded)
	if prefixSize <= 0 {
		t.Fatal("Go encoder produced an invalid length delimiter")
	}
	encodedPayloadSize := uint64(len(encoded) - prefixSize) // #nosec G115 -- the encoder bounds this in-memory test message.
	if messageLength != encodedPayloadSize {
		t.Fatal("Go encoder produced an invalid length delimiter")
	}
	if bytes.Equal(encoded, fixture) {
		t.Fatal("Go output unexpectedly omitted the required stdin length delimiter")
	}
	requireIdenticalBytes(t, encoded[prefixSize:], fixture)
}

func (s *captureStdinStarter) Start(context.Context, string, ...string) (Process, error) {
	return survivingProcess(), nil
}

func (s *captureStdinStarter) StartWithStdin(_ context.Context, _ string, payload []byte, args ...string) (Process, error) {
	s.args = append([]string(nil), args...)
	s.payload = append([]byte(nil), payload...)
	return survivingProcess(), nil
}

func protobufBytesField(t *testing.T, data []byte, wanted uint64) []byte {
	t.Helper()
	for len(data) > 0 {
		tag, size := binary.Uvarint(data)
		if size <= 0 {
			t.Fatal("invalid protobuf tag")
		}
		data = data[size:]
		wire := tag & 7
		if wire == 2 {
			length, read := binary.Uvarint(data)
			if read <= 0 || uint64(len(data[read:])) < length {
				t.Fatal("invalid protobuf bytes field")
			}
			fieldLength := int(length) // #nosec G115 -- length was bounded by the remaining slice above.
			value := data[read : read+fieldLength]
			if tag>>3 == wanted {
				return value
			}
			data = data[read+fieldLength:]
			continue
		}
		if wire == 0 {
			_, read := binary.Uvarint(data)
			if read <= 0 {
				t.Fatal("invalid protobuf varint")
			}
			data = data[read:]
			continue
		}
		t.Fatalf("unsupported wire type %d", wire)
	}
	return nil
}

func TestTestArgumentsFollowTheStdinFlag(t *testing.T) {
	const ticket = "ticket-secret-never-in-argv"
	starter := &captureStdinStarter{}
	service := NewService("game", starter, nil)
	service.SetLaunchGrace(time.Millisecond)
	arguments := []string{"-batchmode", "--ffr-e2e-result", `C:\e2e\result.json`}
	service.SetTestArguments(arguments)
	arguments[0] = "changed after the call"
	bootstrap := LaunchBootstrap{RealmID: "local", WorldEndpoint: "127.0.0.1:27020", GNSCAKeyID: "dev-ca", ProtocolMin: 1, ProtocolMax: 2, ContentVersion: "dev-content"}
	if err := service.PlayMultiplayer(context.Background(), []byte(ticket), bootstrap); err != nil {
		t.Fatal(err)
	}
	want := []string{"--auth-token-stdin", "-batchmode", "--ffr-e2e-result", `C:\e2e\result.json`}
	if strings.Join(starter.args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("argv = %#v, want %#v", starter.args, want)
	}
	_, size := binary.Uvarint(starter.payload)
	if got := string(protobufBytesField(t, starter.payload[size:], 1)); got != ticket {
		t.Fatalf("ticket field = %q", got)
	}
}

func TestMultiplayerLaunchUsesDelimitedStdinWithoutTokenArgv(t *testing.T) {
	const ticket = "ticket-secret-never-in-argv"
	starter := &captureStdinStarter{}
	service := NewService("game", starter, nil)
	service.SetLaunchGrace(time.Millisecond)
	bootstrap := LaunchBootstrap{RealmID: "local", WorldEndpoint: "127.0.0.1:27020", GNSCAKeyID: "dev-ca", ProtocolMin: 1, ProtocolMax: 2, ContentVersion: "dev-content"}
	if err := service.PlayMultiplayer(context.Background(), []byte(ticket), bootstrap); err != nil {
		t.Fatal(err)
	}
	if len(starter.args) != 1 || starter.args[0] != "--auth-token-stdin" || strings.Contains(strings.Join(starter.args, " "), ticket) {
		t.Fatalf("unsafe argv: %#v", starter.args)
	}
	messageLength, size := binary.Uvarint(starter.payload)
	// #nosec G115 -- payload length is non-negative once the varint parsed.
	if size <= 0 || messageLength != uint64(len(starter.payload)-size) {
		t.Fatal("hand-off is not one length-delimited protobuf")
	}
	message := starter.payload[size:]
	if got := string(protobufBytesField(t, message, 1)); got != ticket {
		t.Fatalf("ticket field = %q", got)
	}
	nested := protobufBytesField(t, message, 2)
	if got := string(protobufBytesField(t, nested, 1)); got != "local" {
		t.Fatalf("realm = %q", got)
	}
	if got := string(protobufBytesField(t, nested, 2)); got != "127.0.0.1:27020" {
		t.Fatalf("world endpoint = %q", got)
	}
}
