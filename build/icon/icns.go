package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image/png"
	"os"
)

func optimizeICNS(data []byte) ([]byte, error) {
	if len(data) < 8 || string(data[:4]) != "icns" || uint64(binary.BigEndian.Uint32(data[4:8])) != uint64(len(data)) {
		return nil, errors.New("invalid ICNS header")
	}
	output := append([]byte(nil), data[:8]...)
	for offset := 8; offset < len(data); {
		if len(data)-offset < 8 {
			return nil, errors.New("truncated ICNS representation")
		}
		length := uint64(binary.BigEndian.Uint32(data[offset+4 : offset+8]))
		if length < 8 || length > uint64(len(data)-offset) {
			return nil, errors.New("invalid ICNS representation size")
		}
		end := offset + int(length)
		payload := data[offset+8 : end]
		if bytes.HasPrefix(payload, []byte("\x89PNG\r\n\x1a\n")) {
			decoded, err := png.Decode(bytes.NewReader(payload))
			if err != nil {
				return nil, err
			}
			var compressed bytes.Buffer
			encoder := png.Encoder{CompressionLevel: png.BestCompression}
			if err := encoder.Encode(&compressed, decoded); err != nil {
				return nil, err
			}
			if compressed.Len() < len(payload) {
				payload = compressed.Bytes()
			}
		}
		// Retain every representation ID, including duplicate-size Retina entries,
		// and leave non-PNG metadata/legacy ARGB representations byte-for-byte intact.
		output = append(output, data[offset:offset+4]...)
		output = binary.BigEndian.AppendUint32(output, uint32(len(payload)+8))
		output = append(output, payload...)
		offset = end
	}
	binary.BigEndian.PutUint32(output[4:8], uint32(len(output)))
	return output, nil
}

func optimizeICNSFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	optimized, err := optimizeICNS(data)
	if err != nil {
		return err
	}
	if len(optimized) >= len(data) {
		return nil
	}
	// The packager re-signs the bundle after replacing this resource.
	return os.WriteFile(path, optimized, 0o644)
}
