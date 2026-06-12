package ybn

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

var textLineRe = regexp.MustCompile(`(WORD|SPEAKER)\["((?:[^"\\]|\\.)*)"\]`)

type YSTEntry struct {
	Opcode string
	Text   string
}

func ParseYST(path string) (map[int]YSTEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read yst: %w", err)
	}

	entries := make(map[int]YSTEntry)
	lines := strings.Split(string(data), "\n")
	for lineNo, line := range lines {
		lineNo++ // 1-indexed
		m := textLineRe.FindStringSubmatch(line)
		if m != nil {
			opcode := m[1]
			text := m[2]
			text = strings.ReplaceAll(text, `\"`, `"`)
			text = strings.ReplaceAll(text, `\\`, `\`)
			entries[lineNo] = YSTEntry{Opcode: opcode, Text: text}
		}
	}
	return entries, nil
}

func BuildTranslationMap(origPath, transPath string) (map[string]string, error) {
	orig, err := ParseYST(origPath)
	if err != nil {
		return nil, err
	}
	trans, err := ParseYST(transPath)
	if err != nil {
		return nil, err
	}

	result := make(map[string]string)
	for lineNo, origEntry := range orig {
		if transEntry, ok := trans[lineNo]; ok && transEntry.Opcode == origEntry.Opcode {
			if origEntry.Text != transEntry.Text {
				result[origEntry.Text] = transEntry.Text
			}
		}
	}
	return result, nil
}

func CompileYST(originalYBN, translatedYST, outputYBN, originalYST string) (int, error) {
	var translationMap map[string]string

	if originalYST != "" {
		var err error
		translationMap, err = BuildTranslationMap(originalYST, translatedYST)
		if err != nil {
			return 0, fmt.Errorf("build translation map: %w", err)
		}
	} else {
		entries, err := ParseYST(translatedYST)
		if err != nil {
			return 0, err
		}

		y := &YBNFile{}
		y, err = Read(originalYBN)
		if err != nil {
			return 0, err
		}
		refs := y.GetTextRefs()

		var wordVals, speakerVals []string
		for _, v := range entries {
			switch v.Opcode {
			case "WORD":
				wordVals = append(wordVals, v.Text)
			case "SPEAKER":
				speakerVals = append(speakerVals, v.Text)
			}
		}

		translationMap = make(map[string]string)
		wi, si := 0, 0
		for _, ref := range refs {
			if ref.Opcode == 0x6A && wi < len(wordVals) {
				cnText := wordVals[wi]
				if cnText != ref.Text {
					translationMap[ref.Text] = cnText
				}
				wi++
			} else if ref.Opcode == 0x36 && si < len(speakerVals) {
				cnText := speakerVals[si]
				if cnText != ref.Text {
					translationMap[ref.Text] = cnText
				}
				si++
			}
		}
	}

	if len(translationMap) == 0 {
		return 0, nil
	}

	tunnel := NewSjisTunnel()
	y, err := Read(originalYBN)
	if err != nil {
		return 0, err
	}

	replaced := y.ReplaceText(translationMap, tunnel)
	if err := y.Write(outputYBN); err != nil {
		return 0, err
	}
	return replaced, nil
}

func BatchCompile(originalDir, ystDir, outputDir, originalYSTDir string) (int, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return 0, err
	}

	entries, err := os.ReadDir(ystDir)
	if err != nil {
		return 0, err
	}

	total := 0
	files := 0
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".yst") {
			continue
		}
		base := strings.TrimSuffix(e.Name(), ".yst")
		ybnName := base + ".ybn"
		ybnPath := originalDir + "/" + ybnName
		ystPath := ystDir + "/" + e.Name()
		outPath := outputDir + "/" + ybnName

		if _, err := os.Stat(ybnPath); os.IsNotExist(err) {
			fmt.Printf("[SKIP] %s: no original YBN\n", e.Name())
			continue
		}

		origYST := ""
		if originalYSTDir != "" {
			origYST = originalYSTDir + "/" + e.Name()
		}

		n, err := CompileYST(ybnPath, ystPath, outPath, origYST)
		if err != nil {
			fmt.Printf("[FAIL] %s: %v\n", e.Name(), err)
			continue
		}
		status := "SKIP"
		if n > 0 {
			status = "OK"
		}
		fmt.Printf("  [%s] %s: %d replacements\n", status, e.Name(), n)
		total += n
		files++
	}
	fmt.Printf("\nDone. %d replacements in %d files -> %s/\n", total, files, outputDir)
	return total, nil
}
