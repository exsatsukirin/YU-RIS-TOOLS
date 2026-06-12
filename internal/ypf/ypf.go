package ypf

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/text/encoding/japanese"
)

const Magic = "YPF\x00"
const HeaderSize = 0x20

var swapTable00 = []byte{
	0x03, 0x48, 0x06, 0x35, 0x0C, 0x10, 0x11, 0x19, 0x1C, 0x1E,
	0x09, 0x0B, 0x0D, 0x13, 0x15, 0x1B, 0x20, 0x23, 0x26, 0x29,
	0x2C, 0x2F, 0x2E, 0x32,
}

func decryptLength(value byte) byte {
	for i, v := range swapTable00 {
		if v == value {
			if i&1 == 0 {
				return swapTable00[i+1]
			}
			return swapTable00[i-1]
		}
	}
	return value
}

type Entry struct {
	Name         string
	FileType     byte
	PackFlag     bool
	UnpackedSize uint32
	PackedSize   uint32
	FileOffset   uint32
	Data         []byte
}

type Archive struct {
	Version   uint32
	FileCount uint32
	DirSize   uint32
	Entries   []Entry
}

func Open(path string) (*Archive, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	return Parse(data)
}

func Parse(data []byte) (*Archive, error) {
	if len(data) < HeaderSize || string(data[:4]) != Magic {
		return nil, fmt.Errorf("bad magic: expected YPF\\x00")
	}

	version := binary.LittleEndian.Uint32(data[4:8])
	fileCount := binary.LittleEndian.Uint32(data[8:12])
	dirSize := binary.LittleEndian.Uint32(data[12:16])

	extraSize := uint32(0)
	if version >= 0x1D9 {
		extraSize = 4
	} else if version == 0xDE {
		extraSize = 8
	}

	var entries []Entry
	dirOffset := uint32(HeaderSize)

	for i := uint32(0); i < fileCount; i++ {
		if dirOffset+5 > dirSize {
			break
		}

		nameLenEnc := data[dirOffset+4] ^ 0xFF
		nameLen := decryptLength(nameLenEnc)
		if nameLen == 0 || dirOffset+5+uint32(nameLen)+extraSize+27 > dirSize {
			break
		}

		nameStart := dirOffset + 5
		rawName := data[nameStart : nameStart+uint32(nameLen)]

		var key byte
		if nameLen >= 4 {
			key = rawName[nameLen-4] ^ '.'
		}
		nameBytes := make([]byte, nameLen)
		for j := 0; j < int(nameLen); j++ {
			nameBytes[j] = rawName[j] ^ key
		}
		name := decodeSJIS(nameBytes)

		metaStart := nameStart + uint32(nameLen)
		fileType := data[metaStart]
		packFlag := data[metaStart+1]
		packedSize := binary.LittleEndian.Uint32(data[metaStart+6 : metaStart+10])
		fileOffset := binary.LittleEndian.Uint32(data[metaStart+10 : metaStart+14])

		raw := data[fileOffset : fileOffset+packedSize]

		var fileData []byte
		if packFlag != 0 {
			r, err := zlib.NewReader(bytes.NewReader(raw))
			if err == nil {
				fileData, _ = io.ReadAll(r)
				r.Close()
			} else {
				fileData = raw
			}
		} else {
			fileData = raw
		}

		entries = append(entries, Entry{
			Name:         name,
			FileType:     fileType,
			PackFlag:     packFlag != 0,
			PackedSize:   packedSize,
			FileOffset:   fileOffset,
			Data:         fileData,
		})

		dirOffset += 5 + uint32(nameLen) + 1 + 1 + 4 + 4 + 4 + 4 + extraSize
	}

	return &Archive{
		Version:   version,
		FileCount: fileCount,
		DirSize:   dirSize,
		Entries:   entries,
	}, nil
}

func (a *Archive) ExtractTo(dir string) error {
	for i, e := range a.Entries {
		safeName := filepath.FromSlash(e.Name)
		outPath := filepath.Join(dir, safeName)
		outDir := filepath.Dir(outPath)
		if err := os.MkdirAll(outDir, 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", outDir, err)
		}
		if err := os.WriteFile(outPath, e.Data, 0644); err != nil {
			return fmt.Errorf("write %s: %w", outPath, err)
		}
		fmt.Printf("  [%d] %s (%d bytes)\n", i, e.Name, len(e.Data))
	}
	fmt.Printf("Extracted %d files to %s/\n", len(a.Entries), dir)
	return nil
}

func decodeSJIS(b []byte) string {
	sjisDecoder := japanese.ShiftJIS.NewDecoder()
	decoded, err := sjisDecoder.Bytes(b)
	if err != nil {
		return string(b)
	}
	return string(decoded)
}
