package ybn

import (
	"encoding/binary"
	"os"
)

type CmdEntry struct {
	Opcode     byte
	ParamCount byte
}

func recoverKey(data []byte, version uint32) []byte {
	if len(data) < 0x30 || string(data[:4]) != Magic {
		return nil
	}

	cmdLen := binary.LittleEndian.Uint32(data[0xC:0x10])
	paraStart := uint32(0x20) + cmdLen
	if paraStart+12 > uint32(len(data)) {
		return nil
	}

	// Strategy 1: known key table lookup
	for _, k := range knownKeys {
		if k.min <= version && version <= k.max {
			opcode := data[0x20] ^ k.key[0]
			if validOpcodes[opcode] {
				return k.key
			}
		}
	}

	// Strategy 2: known-plaintext from first para offset
	encOff := data[paraStart+8 : paraStart+12]
	candidate := []byte{encOff[0], encOff[1], encOff[2], encOff[3]}
	opcode := data[0x20] ^ candidate[0]
	if validOpcodes[opcode] {
		return candidate
	}

	return nil
}

func RecoverKey(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	detected := recoverKey(data, 0)
	return detected, nil
}
