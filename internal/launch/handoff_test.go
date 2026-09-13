package launch

import (
	"context"
	"encoding/binary"
	"strings"
	"testing"
	"time"
)

type captureStdinStarter struct {
	args    []string
	payload []byte
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
