package ybn

import (
	"encoding/binary"
	"fmt"
	"os"
)

const (
	Magic       = "YSTB"
	HeaderSize  = 0x20
)

var DefaultXORKey = []byte{0xD3, 0x6F, 0xAC, 0x96}

// Known XOR keys by version range
var knownKeys = []struct {
	min, max uint32
	key      []byte
}{
	{0x1E1, 0x1E8, []byte{0x0B, 0x8F, 0x00, 0xB1}},
	{0x000, 0x22A, []byte{0xD3, 0x6F, 0xAC, 0x96}},
	{0x22B, 0xFFF, []byte{0xA9, 0xF8, 0xCC, 0x66}},
}

var validOpcodes = map[byte]bool{
	0x6A: true, 0x36: true, 0x2D: true, 0x2C: true,
	0x2B: true, 0x0E: true, 0x31: true, 0x4F: true,
	0x6E: true, 0x11: true, 0x12: true, 0x13: true,
	0x2E: true, 0x0B: true, 0x4D: true,
}

func xorBytes(data, key []byte) []byte {
	out := make([]byte, len(data))
	for i := 0; i < len(data); i++ {
		out[i] = data[i] ^ key[i%len(key)]
	}
	return out
}

type YBNFile struct {
	Header    []byte
	Magic     string
	Version   uint32
	CmdCount  uint32
	CmdLen    uint32
	ParaLen   uint32
	StrLen    uint32
	OtherLen  uint32

	CmdSec   []byte
	ParaSec  []byte
	StrSec   []byte
	OtherSec []byte
	Trailer  []byte
	XORKey   []byte
}

func Read(path string) (*YBNFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	return Parse(data)
}

func Parse(data []byte) (*YBNFile, error) {
	if len(data) < HeaderSize || string(data[:4]) != Magic {
		return nil, fmt.Errorf("bad magic: expected YSTB")
	}

	y := &YBNFile{
		Header:   data[:HeaderSize],
		Magic:    string(data[:4]),
		Version:  binary.LittleEndian.Uint32(data[4:8]),
		CmdCount: binary.LittleEndian.Uint32(data[8:12]),
		CmdLen:   binary.LittleEndian.Uint32(data[0xC:0x10]),
		ParaLen:  binary.LittleEndian.Uint32(data[0x10:0x14]),
		StrLen:   binary.LittleEndian.Uint32(data[0x14:0x18]),
		OtherLen: binary.LittleEndian.Uint32(data[0x18:0x1C]),
		XORKey:   DefaultXORKey,
	}

	if detected := recoverKey(data, y.Version); detected != nil {
		y.XORKey = detected
	}

	p := uint32(HeaderSize)
	y.CmdSec = xorBytes(data[p:p+y.CmdLen], y.XORKey); p += y.CmdLen
	y.ParaSec = xorBytes(data[p:p+y.ParaLen], y.XORKey); p += y.ParaLen
	y.StrSec = xorBytes(data[p:p+y.StrLen], y.XORKey); p += y.StrLen
	y.OtherSec = xorBytes(data[p:p+y.OtherLen], y.XORKey); p += y.OtherLen
	y.Trailer = data[p:]

	return y, nil
}

func (y *YBNFile) Write(path string) error {
	var result []byte
	result = append(result, y.Header...)
	result = append(result, xorBytes(y.CmdSec, y.XORKey)...)
	result = append(result, xorBytes(y.ParaSec, y.XORKey)...)
	result = append(result, xorBytes(y.StrSec, y.XORKey)...)
	result = append(result, xorBytes(y.OtherSec, y.XORKey)...)
	result = append(result, y.Trailer...)
	return os.WriteFile(path, result, 0644)
}

func (y *YBNFile) GetCmdList() []CmdEntry {
	cmds := make([]CmdEntry, y.CmdCount)
	for i := uint32(0); i < y.CmdCount; i++ {
		off := i * 4
		if int(off+2) > len(y.CmdSec) {
			cmds[i] = CmdEntry{Opcode: 0, ParamCount: 0}
			continue
		}
		cmds[i] = CmdEntry{
			Opcode:     y.CmdSec[off],
			ParamCount: y.CmdSec[off+1],
		}
	}
	return cmds
}

type ParaEntry struct {
	Pre    uint32
	Length uint32
	Offset uint32
}

func (y *YBNFile) GetParaEntries() []ParaEntry {
	n := uint32(len(y.ParaSec)) / 12
	entries := make([]ParaEntry, n)
	for i := uint32(0); i < n; i++ {
		off := i * 12
		entries[i] = ParaEntry{
			Pre:    binary.LittleEndian.Uint32(y.ParaSec[off:]),
			Length: binary.LittleEndian.Uint32(y.ParaSec[off+4:]),
			Offset: binary.LittleEndian.Uint32(y.ParaSec[off+8:]),
		}
	}
	return entries
}

func (y *YBNFile) SetParaEntries(entries []ParaEntry) {
	buf := make([]byte, len(entries)*12)
	for i, e := range entries {
		off := i * 12
		binary.LittleEndian.PutUint32(buf[off:], e.Pre)
		binary.LittleEndian.PutUint32(buf[off+4:], e.Length)
		binary.LittleEndian.PutUint32(buf[off+8:], e.Offset)
	}
	y.ParaSec = buf
}

type StringEntry struct {
	Offset int
	Text   string
	Kind   string
}

type TextRef struct {
	Opcode     byte
	Offset     uint32
	Length     uint32
	Text       string
	TextRaw    []byte
	EntryIndex int
	Type       string // "TEXT" or "BYTECODE"
	Pre        uint32
	RawHex     string
	Strings    []StringEntry
}

func (y *YBNFile) GetTextRefs() []TextRef {
	cmds := y.GetCmdList()
	entries := y.GetParaEntries()
	var refs []TextRef
	entryIdx := 0
	jpLeft := []byte{0x81, 0x75}
	jpRight := []byte{0x81, 0x76}

	for _, cmd := range cmds {
		pc := int(cmd.ParamCount)
		if pc > 0 && entryIdx+pc <= len(entries) {
			cmdEntries := entries[entryIdx : entryIdx+pc]

			var offset, length uint32
			switch cmd.Opcode {
			case 0x6A, 0x36:
				length = cmdEntries[0].Length
				offset = cmdEntries[0].Offset
			}

			if offset > 0 && length > 0 && length < 0x10000 && int(offset+length) <= len(y.StrSec) {
				textRaw := y.StrSec[offset : offset+length]
				dialogueText := extractDialogue(textRaw, jpLeft, jpRight)
				if dialogueText == "" {
					dialogueText = decodeSJIS(textRaw)
				}
				refs = append(refs, TextRef{
					Opcode:     cmd.Opcode,
					Offset:     offset,
					Length:     length,
					Text:       dialogueText,
					TextRaw:    textRaw,
					EntryIndex: entryIdx,
				})
			}
		}
		entryIdx += pc
	}
	return refs
}

func extractDialogue(raw []byte, jpLeft, jpRight []byte) string {
	i := 0
	for i < len(raw) {
		if i+1 < len(raw) && raw[i] == jpLeft[0] && raw[i+1] == jpLeft[1] {
			for j := i + 2; j < len(raw)-1; j++ {
				if raw[j] == jpRight[0] && raw[j+1] == jpRight[1] {
					decoded := decodeSJIS(raw[i+2 : j])
					if decoded != "" {
						return decoded
					}
					i = j + 2
					break
				}
			}
			i++
		} else if raw[i] == 0x22 {
			for j := i + 1; j < len(raw); j++ {
				if raw[j] == 0x22 {
					decoded := decodeSJIS(raw[i+1 : j])
					if decoded != "" {
						return decoded
					}
					i = j + 1
					break
				}
			}
			i++
		} else {
			i++
		}
	}
	return ""
}

func decodeSJIS(b []byte) string {
	decoded, err := sjisDecoder.Bytes(b)
	if err != nil {
		return string(b)
	}
	return string(decoded)
}
