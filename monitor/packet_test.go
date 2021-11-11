package monitor

import (
	"encoding/hex"
	"testing"
)

var (
	// The byte slices here must be kept in sync with the tests in netwayste and netwaystev2
	// See netwaystev2/src/filter/tests/packet_serialization.rs
	expectedGetStatusBytes = []byte{
		4, 0, 0, 0, // 4=GetStatus
		0xF0, 0xDE, 0xBC, 0x9A, 0x78, 0x56, 0x34, 0x12, // ping.nonce
	}
	inputStatusBytes = []byte{
		5, 0, 0, 0, // 5=Status
		0xF0, 0xDE, 0xBC, 0x9A, 0x78, 0x56, 0x34, 0x12, // pong.nonce
		3, 0, 0, 0, 0, 0, 0, 0, 118, 101, 114, // server_version
		123, 0, 0, 0, 0, 0, 0, 0, // player_count
		200, 1, 0, 0, 0, 0, 0, 0, // room_count
		2, 0, 0, 0, 0, 0, 0, 0, 110, 109, // server_name
	}
	expectedStatus = ServerStatus{
		Nonce:         0x123456789ABCDEF0,
		ServerVersion: "ver",
		PlayerCount:   123,
		RoomCount:     456,
		ServerName:    "nm",
	}
)

func TestMarshalGetStatus(t *testing.T) {
	getStatus := ServerGetStatus{
		Nonce: 0x123456789ABCDEF0,
	}
	gotBytes, err := Marshal(&getStatus)
	if err != nil {
		t.Fatalf("error from marshal: %v", err)
	}
	gotHexStr := hex.EncodeToString(gotBytes)
	expectedHexStr := hex.EncodeToString(expectedGetStatusBytes)
	if expectedHexStr != gotHexStr {
		t.Errorf("expected %s, got %s", expectedHexStr, gotHexStr)
	}
}

func TestUnmarshalStatus(t *testing.T) {
	var status ServerStatus
	if err := Unmarshal(inputStatusBytes, &status); err != nil {
		t.Fatalf("error from unmarshal: %v", err)
	}
	if expectedStatus != status {
		t.Errorf("expected %+v, got %+v", expectedStatus, status)
	}
}
