package wal

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
)

const (
	HeaderSize  = 12
	FlagSize    = 1
	TotalHeader = HeaderSize + FlagSize
)

const (
	FlagNormal byte = 0x00
	FlagDelete byte = 0x01
)

var ErrChecksumMismatch = errors.New("checksum mismatch")

type Record struct {
	Key   []byte
	Value []byte
	Flag  byte
}

func (r *Record) Size() int {
	return TotalHeader + len(r.Key) + len(r.Value)
}

func (r *Record) Bytes() []byte {
	body := make([]byte, len(r.Key)+len(r.Value))
	copy(body, r.Key)
	copy(body[len(r.Key):], r.Value)
	crc := crc32.ChecksumIEEE(body)

	buf := make([]byte, TotalHeader+len(r.Key)+len(r.Value))
	binary.BigEndian.PutUint32(buf[0:4], crc)
	binary.BigEndian.PutUint32(buf[4:8], uint32(len(r.Key)))
	binary.BigEndian.PutUint32(buf[8:12], uint32(len(r.Value)))
	buf[12] = r.Flag
	copy(buf[13:], body)
	return buf
}

func ReadRecord(r io.Reader) (*Record, error) {
	header := make([]byte, TotalHeader)
	if _, err := io.ReadFull(r, header); err != nil {
		if err == io.EOF {
			return nil, io.EOF
		}
		return nil, err
	}

	crc := binary.BigEndian.Uint32(header[0:4])
	keyLen := binary.BigEndian.Uint32(header[4:8])
	valLen := binary.BigEndian.Uint32(header[8:12])
	flag := header[12]

	bodySize := int(keyLen) + int(valLen)
	if bodySize < 0 {
		return nil, errors.New("invalid record: negative body size")
	}
	body := make([]byte, bodySize)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}

	computed := crc32.ChecksumIEEE(body)
	if computed != crc {
		return nil, ErrChecksumMismatch
	}

	key := make([]byte, keyLen)
	value := make([]byte, valLen)
	copy(key, body[:keyLen])
	copy(value, body[keyLen:])

	return &Record{Key: key, Value: value, Flag: flag}, nil
}

func WriteRecord(w io.Writer, key, value []byte, flag byte) error {
	rec := &Record{Key: key, Value: value, Flag: flag}
	data := rec.Bytes()
	_, err := w.Write(data)
	return err
}
