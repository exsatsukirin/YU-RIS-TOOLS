package ybn

import (
	"golang.org/x/text/encoding/japanese"
)

var sjisEncoder = japanese.ShiftJIS.NewEncoder()
var sjisDecoder = japanese.ShiftJIS.NewDecoder()

// Low bytes to avoid in tunnel encoding
var lowBytesToAvoid = []byte{0x09, 0x0A, 0x0D, 0x20, 0x2C}

type SjisTunnel struct {
	Mappings  []string
	charToIdx map[rune]int
}

func NewSjisTunnel() *SjisTunnel {
	return &SjisTunnel{
		charToIdx: make(map[rune]int),
	}
}

func (t *SjisTunnel) Encode(text string) []byte {
	var result []byte
	for _, ch := range text {
		_, err := sjisEncoder.Bytes([]byte(string(ch)))
		if err == nil {
			enc, _ := sjisEncoder.Bytes([]byte(string(ch)))
			result = append(result, enc...)
		} else {
			if _, ok := t.charToIdx[ch]; !ok {
				t.charToIdx[ch] = len(t.Mappings)
				t.Mappings = append(t.Mappings, string(ch))
			}
			idx := t.charToIdx[ch]
			result = append(result, idxToTunnel(idx)...)
		}
	}
	return result
}

func idxToTunnel(idx int) []byte {
	perBlock := 0x40 - len(lowBytesToAvoid) - 1
	hi := idx / perBlock
	lo := idx % perBlock

	var hb byte
	if hi < 0x1F {
		hb = byte(0x81 + hi)
	} else {
		hb = byte(0xE0 + (hi - 0x1F))
	}

	lb := byte(1 + lo)
	for _, b := range lowBytesToAvoid {
		if lb >= b {
			lb++
		}
	}

	return []byte{hb, lb}
}
