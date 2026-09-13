package launch

import "errors"

// LaunchBootstrap is server-controlled admission metadata. Trust roots remain
// pinned in the signed game build; the server may select only a key ID.
type LaunchBootstrap struct {
	RealmID        string
	WorldEndpoint  string
	GNSCAKeyID     string
	ProtocolMin    uint32
	ProtocolMax    uint32
	ContentVersion string
}

func marshalDelimitedHandoff(ticket []byte, bootstrap LaunchBootstrap) ([]byte, error) {
	if len(ticket) == 0 || bootstrap.RealmID == "" || bootstrap.WorldEndpoint == "" || bootstrap.GNSCAKeyID == "" || bootstrap.ContentVersion == "" {
		return nil, errors.New("launch hand-off is incomplete")
	}
	nested := make([]byte, 0, 128)
	nested = appendBytesField(nested, 1, []byte(bootstrap.RealmID))
	nested = appendBytesField(nested, 2, []byte(bootstrap.WorldEndpoint))
	nested = appendBytesField(nested, 3, []byte(bootstrap.GNSCAKeyID))
	nested = appendVarintField(nested, 4, uint64(bootstrap.ProtocolMin))
	nested = appendVarintField(nested, 5, uint64(bootstrap.ProtocolMax))
	nested = appendBytesField(nested, 6, []byte(bootstrap.ContentVersion))
	message := make([]byte, 0, len(ticket)+len(nested)+16)
	message = appendBytesField(message, 1, ticket)
	message = appendBytesField(message, 2, nested)
	delimited := appendVarint(nil, uint64(len(message)))
	delimited = append(delimited, message...)
	clear(nested)
	clear(message)
	return delimited, nil
}

func appendBytesField(output []byte, field uint64, value []byte) []byte {
	output = appendVarint(output, field<<3|2)
	output = appendVarint(output, uint64(len(value)))
	return append(output, value...)
}

func appendVarintField(output []byte, field, value uint64) []byte {
	output = appendVarint(output, field<<3)
	return appendVarint(output, value)
}

func appendVarint(output []byte, value uint64) []byte {
	for value >= 0x80 {
		output = append(output, byte(value)|0x80)
		value >>= 7
	}
	return append(output, byte(value))
}
