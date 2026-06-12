package ybn

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

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
				text := extractDialogue(raw, jpLeft, jpRight)
				if text == "" {
					text = decodeSJIS(raw)
				}
				entries = append(entries, TextEntry{
					File:   filepath.Base(path),
					Seq:    seq,
					Opcode: fmt.Sprintf("0x%02X", cmd.Opcode),
					Offset: int(p.Offset),
					Length: int(p.Length),
					Text:   text,
				})
				seq++
			}
		}
		ei += pc
	}
	return entries, nil
}

// TextEntry represents one extracted text line.
type TextEntry struct {
	File   string `json:"file"`
	Seq    int    `json:"seq"`
	Opcode string `json:"opcode"`
	Offset int    `json:"offset"`
	Length int    `json:"length"`
	Text   string `json:"text"`
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
