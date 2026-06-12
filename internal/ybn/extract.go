package ybn

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func stripPrefix(raw, jpLeft, jpRight []byte) string {
	// When 「 is present but 」 is in another para, decode from after 「
	// Also strip trailing 」 if present (closing quote in this para)
	start := 0
	end := len(raw)
	
	if len(jpLeft) == 2 {
		for i := 0; i < len(raw)-1; i++ {
			if raw[i] == jpLeft[0] && raw[i+1] == jpLeft[1] {
				start = i + 2
				break
			}
		}
	}
	if len(jpRight) == 2 {
		for i := len(raw) - 2; i >= start; i-- {
			if raw[i] == jpRight[0] && raw[i+1] == jpRight[1] {
				end = i
				break
			}
		}
	}
	
	return decodeSJIS(raw[start:end])
}

// ExtractText extracts all dialogue text from a YBN file.
// Returns entries grouped by file: {filename: [{opcode, offset, length, text}]}
func ExtractText(path string) ([]TextEntry, error) {
	y, err := Read(path)
	if err != nil {
		return nil, err
	}
	if y.Magic != "YSTB" {
		return nil, fmt.Errorf("not a YSTB file")
	}

	var entries []TextEntry
	cmds := y.GetCmdList()
	paras := y.GetParaEntries()
	ei := 0
	seq := 0

	jpLeft := []byte{0x81, 0x75}
	jpRight := []byte{0x81, 0x76}

	for _, cmd := range cmds {
		pc := int(cmd.ParamCount)
		if pc > 0 && ei+pc <= len(paras) && (cmd.Opcode == 0x6A || cmd.Opcode == 0x36) {
			idx := 0
			if cmd.Opcode == 0x36 && pc >= 2 {
				idx = pc - 1
			}
			p := paras[ei+idx]
			if p.Length > 0 && p.Length < 0x10000 && int(p.Offset+p.Length) <= len(y.StrSec) {
				raw := y.StrSec[p.Offset : p.Offset+p.Length]
				hasOp := bytesContain(raw, jpLeft)
				hasCl := bytesContain(raw, jpRight)
				text := extractDialogue(raw, jpLeft, jpRight)
				if text == "" {
					text = stripPrefix(raw, jpLeft, jpRight)
				}
				entries = append(entries, TextEntry{
					File:     filepath.Base(path),
					Seq:      seq,
					Opcode:   fmt.Sprintf("0x%02X", cmd.Opcode),
					Offset:   int(p.Offset),
					Length:   int(p.Length),
					Text:     text,
					HasOpen:  hasOp,
					HasClose: hasCl,
				})
				seq++
			}
		}
		ei += pc
	}

	// Merge consecutive WORD entries that split quoted text across instructions
	entries = mergeEntries(entries)

	return entries, nil
}

func mergeEntries(entries []TextEntry) []TextEntry {
	return entries // no merge — keep per-para for correct injection matching
}

func bytesContain(data, sub []byte) bool {
	for i := 0; i <= len(data)-len(sub); i++ {
		match := true
		for j := 0; j < len(sub); j++ {
			if data[i+j] != sub[j] {
				match = false
				break
			}
		}
		if match { return true }
	}
	return false
}

// TextEntry represents one extracted text line.
type TextEntry struct {
	File     string `json:"file"`
	Seq      int    `json:"seq"`
	Opcode   string `json:"opcode"`
	Offset   int    `json:"offset"`
	Length   int    `json:"length"`
	Text     string `json:"text"`
	HasOpen  bool   `json:"-"` // raw bytes contain 「
	HasClose bool   `json:"-"` // raw bytes contain 」
}

// ExtractDir extracts text from all YBN files in a directory.
func ExtractDir(dir string) ([]TextEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var result []TextEntry
	for _, e := range entries {
		if !strings.HasSuffix(strings.ToLower(e.Name()), ".ybn") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		ents, err := ExtractText(path)
		if err != nil {
			continue
		}
		result = append(result, ents...)
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].File != result[j].File {
			return result[i].File < result[j].File
		}
		return result[i].Seq < result[j].Seq
	})

	return result, nil
}

// SaveTextJSON writes extracted text to a JSON file.
func SaveTextJSON(entries []TextEntry, path string) error {
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// LoadTextJSON reads a translation JSON file.
// Format: array of {file, seq, opcode, offset, length, text}
func LoadTextJSON(path string) ([]TextEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var entries []TextEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// LoadNamesJSON reads a simple {jp: cn} name mapping JSON file.
func LoadNamesJSON(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}
