package monitor

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
)

type ServerGetStatus struct {
	Nonce uint64
}

type ServerStatus struct {
	Nonce         uint64
	ServerVersion string
	PlayerCount   uint64
	RoomCount     uint64
	ServerName    string
}

var (
	ErrUnknownType  = errors.New("unknown type for marshal/unmarshal operation")
	ErrMalformed    = errors.New("malformed packet")
	ErrWrongVariant = errors.New("wrong variant")
)

func Marshal(v interface{}) ([]byte, error) {
	buf := &bytes.Buffer{}
	switch msg := v.(type) {
	case *ServerGetStatus:
		variantNum := uint32(4)
		if err := binary.Write(buf, binary.LittleEndian, &variantNum); err != nil {
			return nil, err
		}
		if err := binary.Write(buf, binary.LittleEndian, &msg.Nonce); err != nil {
			return nil, err
		}
	case *ServerStatus:
		variantNum := uint32(5)
		if err := binary.Write(buf, binary.LittleEndian, &variantNum); err != nil {
			return nil, err
		}
		if err := binary.Write(buf, binary.LittleEndian, &msg.Nonce); err != nil {
			return nil, err
		}
		if err := writeString(buf, msg.ServerVersion); err != nil {
			return nil, err
		}
		if err := binary.Write(buf, binary.LittleEndian, &msg.PlayerCount); err != nil {
			return nil, err
		}
		if err := binary.Write(buf, binary.LittleEndian, &msg.RoomCount); err != nil {
			return nil, err
		}
		if err := writeString(buf, msg.ServerName); err != nil {
			return nil, err
		}
	default:
		return nil, ErrUnknownType
	}
	return buf.Bytes(), nil
}

func writeString(buf io.Writer, s string) error {
	sLen := uint64(len(s))
	if err := binary.Write(buf, binary.LittleEndian, &sLen); err != nil {
		return err
	}
	_, err := buf.Write([]byte(s))
	return err
}

func Unmarshal(packetBytes []byte, v interface{}) error {
	var variantNum uint32
	buf := bytes.NewBuffer(packetBytes)
	if err := binary.Read(buf, binary.LittleEndian, &variantNum); err != nil {
		return err
	}
	switch msg := v.(type) {
	case *ServerGetStatus:
		if variantNum != 4 {
			return ErrWrongVariant
		}
		if err := binary.Read(buf, binary.LittleEndian, &msg.Nonce); err != nil {
			return err
		}
	case *ServerStatus:
		if variantNum != 5 {
			return ErrWrongVariant
		}
		if err := binary.Read(buf, binary.LittleEndian, &msg.Nonce); err != nil {
			return err
		}
		serverVersion, err := readString(buf)
		if err != nil {
			return err
		}
		msg.ServerVersion = serverVersion
		if err := binary.Read(buf, binary.LittleEndian, &msg.PlayerCount); err != nil {
			return err
		}
		if err := binary.Read(buf, binary.LittleEndian, &msg.RoomCount); err != nil {
			return err
		}
		serverName, err := readString(buf)
		if err != nil {
			return err
		}
		msg.ServerName = serverName
	default:
		return ErrUnknownType
	}
	return nil
}

func readString(buf *bytes.Buffer) (string, error) {
	var sLen uint64
	if err := binary.Read(buf, binary.LittleEndian, &sLen); err != nil {
		return "", err
	}
	if sLen > uint64(buf.Len()) {
		return "", ErrMalformed
	}
	sBuf := make([]byte, int(sLen))
	_, err := buf.Read(sBuf)
	if err != nil {
		return "", err
	}
	return string(sBuf), nil
}
