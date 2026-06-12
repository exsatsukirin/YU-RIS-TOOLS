package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/exsatsukirin/YU-RIS-TOOLS/internal/ybn"
	"github.com/exsatsukirin/YU-RIS-TOOLS/internal/ypf"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "extract":
		cmdExtract(args)
	case "decompile":
		cmdDecompile(args)
	case "compile":
		cmdCompile(args)
	case "extract-text":
		cmdExtractText(args)
	case "inject-text":
		cmdInjectText(args)
	case "stats":
		cmdStats(args)
	case "keyfind":
		cmdKeyfind(args)
	case "verify":
		cmdVerify(args)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`YU-RIS Tools (yrt) - YBN/YPF manipulation tool

Usage:
  yrt <command> [options]

Commands:
  extract      <input.ypf> [output_dir]   Extract YPF archive
  extract-text <input.ybn|dir/> [-o out.json]  Extract dialogue text to JSON
  inject-text  <ybn_dir> -t <trans.json> [-o out_dir]  Inject translations
  decompile    <input.ybn> [-o output.yst]  Decompile YBN to YST
  compile      <yst_dir> -o <ybn_dir> --original <ybn_dir> [--original-yst <yst_dir>]
  stats        <input.ybn>                Show opcode statistics
  keyfind      <file.ybn|dir/>            Recover XOR key
  verify       <file.ybn|dir/>            Verify round-trip

Examples:
  yrt extract ysbin.ypf extracted/
  yrt decompile yst00066.ybn -o out.yst
  yrt decompile original_ybn/ -o decompiled/
  yrt compile yst/ -o ybn_out/ --original original_ybn/
  yrt stats yst00066.ybn
  yrt keyfind original_ybn/
  yrt verify original_ybn/
`)
}

func cmdExtract(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: yu-ris-tools-go extract <input.ypf> [output_dir]")
		os.Exit(1)
	}
	ypfPath := args[0]
	outDir := "ypf_extracted"
	if len(args) > 1 {
		outDir = args[1]
	}

	a, err := ypf.Open(ypfPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("YPF v0x%X, %d files, dir=%d\n", a.Version, a.FileCount, a.DirSize)
	if err := a.ExtractTo(outDir); err != nil {
		fmt.Fprintf(os.Stderr, "Extract error: %v\n", err)
		os.Exit(1)
	}
}

func cmdDecompile(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: yu-ris-tools-go decompile <input.ybn|dir/> [-o output]")
		os.Exit(1)
	}

	input := args[0]
	output := ""
	for i := 1; i < len(args); i++ {
		if args[i] == "-o" && i+1 < len(args) {
			output = args[i+1]
			i++
		}
	}

	info, err := os.Stat(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if info.IsDir() {
		if output == "" {
			output = "decompiled_yst"
		}
		count := batchDecompile(input, output)
		fmt.Printf("\nDone. %d files decompiled -> %s/\n", count, output)
	} else {
		if output == "" {
			output = input + ".yst"
		}
		decompileFile(input, output)
	}
}

func decompileFile(inputPath, outputPath string) {
	y, err := ybn.Read(inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] Failed to read %s: %v\n", inputPath, err)
		return
	}
	if y.Magic != "YSTB" {
		fmt.Fprintf(os.Stderr, "[SKIP] Not a YBN file: %s\n", inputPath)
		return
	}

	d := ybn.NewDisassembler(y)
	f := ybn.NewYSTFormatter(d)
	output := f.Format()

	outDir := filepath.Dir(outputPath)
	if outDir != "." {
		os.MkdirAll(outDir, 0755)
	}
	if err := os.WriteFile(outputPath, []byte(output), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] Write %s: %v\n", outputPath, err)
		return
	}
	fmt.Printf("[OK] %s -> %s (%d chars, %d cmds)\n",
		filepath.Base(inputPath), outputPath, len(output), y.CmdCount)
}

func batchDecompile(inputDir, outputDir string) int {
	os.MkdirAll(outputDir, 0755)
	count := 0
	entries, err := os.ReadDir(inputDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] %v\n", err)
		return 0
	}
	for _, e := range entries {
		if !strings.HasSuffix(strings.ToLower(e.Name()), ".ybn") {
			continue
		}
		inPath := filepath.Join(inputDir, e.Name())
		outName := strings.TrimSuffix(e.Name(), ".ybn") + ".yst"
		outPath := filepath.Join(outputDir, outName)
		decompileFile(inPath, outPath)
		count++
	}
	return count
}

func cmdCompile(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: yu-ris-tools-go compile <yst_dir> -o <ybn_dir> --original <ybn_dir> [--original-yst <yst_dir>]")
		os.Exit(1)
	}

	ystDir := ""
	outputDir := ""
	originalDir := ""
	originalYSTDir := ""

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-o":
			if i+1 < len(args) {
				outputDir = args[i+1]
				i++
			}
		case "--original":
			if i+1 < len(args) {
				originalDir = args[i+1]
				i++
			}
		case "--original-yst":
			if i+1 < len(args) {
				originalYSTDir = args[i+1]
				i++
			}
		default:
			if ystDir == "" && !strings.HasPrefix(args[i], "-") {
				ystDir = args[i]
			}
		}
	}

	if ystDir == "" || outputDir == "" || originalDir == "" {
		fmt.Fprintln(os.Stderr, "Required: <yst_dir> -o <ybn_dir> --original <ybn_dir>")
		os.Exit(1)
	}

	_, err := ybn.BatchCompile(originalDir, ystDir, outputDir, originalYSTDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func cmdStats(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: yu-ris-tools-go stats <input.ybn>")
		os.Exit(1)
	}

	y, err := ybn.Read(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	d := ybn.NewDisassembler(y)
	instrs := d.Disassemble()

	stats := make(map[byte]int)
	for _, inst := range instrs {
		stats[inst.Opcode]++
	}

	fmt.Printf("File: %s\n", filepath.Base(args[0]))
	fmt.Printf("  Version: %d\n", y.Version)
	fmt.Printf("  Instructions: %d\n", y.CmdCount)
	fmt.Printf("  String pool: %d bytes\n", y.StrLen)
	fmt.Printf("  Opcode distribution:\n")
	for op := 0; op <= 0xFF; op++ {
		b := byte(op)
		if n, ok := stats[b]; ok {
			name := ybn.OpcodeNames[b]
			if name == "" {
				name = fmt.Sprintf("UNK_%02X", b)
			}
			desc := ybn.OpcodeDescriptions[b]
			comment := ""
			if desc != "" {
				comment = fmt.Sprintf("  — %s", desc)
			}
			fmt.Printf("    0x%02X (%-16s): %5d%s\n", b, name, n, comment)
		}
	}

	textCount := stats[0x6A] + stats[0x36]
	dumpCount := stats[0x2D]
	fmt.Printf("  Text ops (0x6A+0x36): %d\n", textCount)
	fmt.Printf("  Dump blocks (0x2D):   %d\n", dumpCount)
}

func cmdKeyfind(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: yu-ris-tools-go keyfind <file.ybn|dir/>")
		os.Exit(1)
	}

	info, err := os.Stat(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if info.IsDir() {
		entries, _ := os.ReadDir(args[0])
		count := 0
		for _, e := range entries {
			if !strings.HasSuffix(strings.ToLower(e.Name()), ".ybn") {
				continue
			}
			path := filepath.Join(args[0], e.Name())
			if showKey(path) {
				count++
			}
		}
		if count == 0 {
			fmt.Fprintln(os.Stderr, "No key recovered.")
		}
	} else {
		showKey(args[0])
	}
}

func showKey(path string) bool {
	key, err := ybn.RecoverKey(path)
	if err != nil || key == nil {
		fmt.Fprintf(os.Stderr, "%-40s  no key found\n", path)
		return false
	}
	fmt.Printf("%-40s  key=%02X%02X%02X%02X\n", path, key[0], key[1], key[2], key[3])
	return true
}

func cmdVerify(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: yu-ris-tools-go verify <file.ybn|dir/>")
		os.Exit(1)
	}

	info, err := os.Stat(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if info.IsDir() {
		entries, _ := os.ReadDir(args[0])
		passed, failed := 0, 0
		var firstErrs []string
		for _, e := range entries {
			if !strings.HasSuffix(strings.ToLower(e.Name()), ".ybn") {
				continue
			}
			path := filepath.Join(args[0], e.Name())
			if verifyFile(path) {
				passed++
			} else {
				if len(firstErrs) < 5 {
					firstErrs = append(firstErrs, fmt.Sprintf("%s: round-trip failed", e.Name()))
				}
				failed++
			}
		}
		fmt.Printf("\nResults: %d passed, %d failed\n", passed, failed)
		for _, err := range firstErrs {
			fmt.Printf("  %s\n", err)
		}
	} else {
		verifyFile(args[0])
	}
}

func verifyFile(path string) bool {
	original, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[FAIL] %s: read error: %v\n", path, err)
		return false
	}

	y, err := ybn.Read(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[FAIL] %s: parse error: %v\n", path, err)
		return false
	}

	tmpPath := path + ".tmp"
	if err := y.Write(tmpPath); err != nil {
		fmt.Fprintf(os.Stderr, "[FAIL] %s: write error: %v\n", path, err)
		os.Remove(tmpPath)
		return false
	}

	rebuilt, err := os.ReadFile(tmpPath)
	os.Remove(tmpPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[FAIL] %s: readback error: %v\n", path, err)
		return false
	}

	if len(original) != len(rebuilt) {
		fmt.Fprintf(os.Stderr, "[FAIL] %s: size mismatch %d vs %d\n", path, len(original), len(rebuilt))
		return false
	}

	diff := 0
	for i := 0; i < len(original); i++ {
		if original[i] != rebuilt[i] {
			diff++
		}
	}
	if diff > 0 {
		fmt.Fprintf(os.Stderr, "[FAIL] %s: %d bytes differ\n", path, diff)
		return false
	}

	fmt.Printf("[PASS] Round-trip: %s (%d bytes)\n", filepath.Base(path), len(original))
	return true
}



// ── extract-text ──

func cmdExtractText(args []string) {
	input := ""
	output := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-o":
			if i+1 < len(args) { output = args[i+1]; i++ }
		default:
			if input == "" { input = args[i] }
		}
	}
	if input == "" {
		fmt.Fprintln(os.Stderr, "Usage: yrt extract-text <input.ybn|dir/> [-o out.json]")
		os.Exit(1)
	}
	if output == "" { output = "translations.json" }

	info, err := os.Stat(input)
	if err != nil { fmt.Fprintf(os.Stderr, "Error: %v\n", err); os.Exit(1) }

	var entries []ybn.TextEntry
	if info.IsDir() {
		entries, err = ybn.ExtractDir(input)
	} else {
		entries, err = ybn.ExtractText(input)
	}
	if err != nil { fmt.Fprintf(os.Stderr, "Error: %v\n", err); os.Exit(1) }

	if err := ybn.SaveTextJSON(entries, output); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err); os.Exit(1)
	}

	texts := make(map[string]bool)
	for _, e := range entries {
		if e.Text != "" { texts[e.Text] = true }
	}
	fmt.Printf("Extracted %d lines (%d unique) → %s\n", len(entries), len(texts), output)
}

// ── inject-text ──

func cmdInjectText(args []string) {
	ybnDir, transPath, namesPath, outputDir := "", "", "", ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-t": if i+1 < len(args) { transPath = args[i+1]; i++ }
		case "-n": if i+1 < len(args) { namesPath = args[i+1]; i++ }
		case "-o": if i+1 < len(args) { outputDir = args[i+1]; i++ }
		default: if ybnDir == "" { ybnDir = args[i] }
		}
	}
	if ybnDir == "" || transPath == "" {
		fmt.Fprintln(os.Stderr, "Usage: yrt inject-text <ybn_dir> -t <trans.json> [-n names.json] [-o out_dir]")
		os.Exit(1)
	}
	if outputDir == "" { outputDir = "ybn_translated" }

	// Load translated entries
	transEntries, err := ybn.LoadTextJSON(transPath)
	if err != nil { fmt.Fprintf(os.Stderr, "Error: %v\n", err); os.Exit(1) }

	// Load names
	var nameMap map[string]string
	if namesPath != "" {
		nameMap, err = ybn.LoadNamesJSON(namesPath)
		if err != nil { fmt.Fprintf(os.Stderr, "Warning: names: %v\n", err) }
	}

	// Build map: {filename: {seq: translated_text}}
	transMap := make(map[string]map[int]string)
	for _, e := range transEntries {
		if _, ok := transMap[e.File]; !ok {
			transMap[e.File] = make(map[int]string)
		}
		transMap[e.File][e.Seq] = e.Text
	}

	os.MkdirAll(outputDir, 0755)
	ybnFiles, _ := os.ReadDir(ybnDir)

	// Copy all original YBNs as base (non-YSTB files included)
	for _, f := range ybnFiles {
		if !strings.HasSuffix(strings.ToLower(f.Name()), ".ybn") { continue }
		src, _ := os.ReadFile(filepath.Join(ybnDir, f.Name()))
		if src == nil { continue }
		os.WriteFile(filepath.Join(outputDir, f.Name()), src, 0644)
	}

	tunnel := ybn.NewSjisTunnel()
	td1, tn1, files, skipped := 0, 0, 0, 0
	for _, f := range ybnFiles {
		if !strings.HasSuffix(strings.ToLower(f.Name()), ".ybn") { continue }
		ybnPath := filepath.Join(ybnDir, f.Name())

		y, err := ybn.Read(ybnPath)
		if err != nil || y.Magic != "YSTB" { continue }

		dCount, nCount := 0, 0

		// Dialogue injection
		origEntries, err := ybn.ExtractText(ybnPath)
		if err == nil {
			tmap, ok := transMap[f.Name()]
			if ok {
				diff := make(map[string]string)
				for _, oe := range origEntries {
					if t, ok := tmap[oe.Seq]; ok && t != "" && t != oe.Text {
						diff[oe.Text] = t
					}
				}
				if len(diff) > 0 {
					dCount = y.ReplaceText(diff, tunnel)
					td1 += dCount
				}
			}
		}

		// Name patching (using same tunnel)
		if len(nameMap) > 0 {
			strSec := []byte(y.StrSec)
			for jp, cn := range nameMap {
				jpBytes, _ := ybn.SjisEncoder.Bytes([]byte(jp))
				if len(jpBytes) == 0 { continue }
				cnBytes := tunnel.Encode(cn)
				if len(cnBytes) > len(jpBytes) { continue }
				pos := 0
				for {
					idx := indexBytes(strSec[pos:], jpBytes)
					if idx < 0 { break }
					copy(strSec[pos+idx:], cnBytes)
					for k := pos + idx + len(cnBytes); k < pos+idx+len(jpBytes); k++ {
						strSec[k] = 0
					}
					nCount++
					pos += idx + len(cnBytes)
				}
			}
			y.StrSec = strSec
			tn1 += nCount
		}

		if dCount > 0 || nCount > 0 {
			if dCount > 0 || nCount > 0 {
				fmt.Printf("  [OK] %s: %d dialogue + %d names\n", f.Name(), dCount, nCount)
			}
			y.Write(filepath.Join(outputDir, f.Name()))
			files++
		} else {
			skipped++
		}
	}

	// Generate sjis_ext.bin from the same tunnel instance
	tunnelPath := filepath.Join(outputDir, "sjis_ext.bin")
	if len(tunnel.Mappings) > 0 {
		var u16 []byte
		for _, ch := range tunnel.Mappings {
			for _, r := range ch {
				u16 = append(u16, byte(r), byte(r>>8))
			}
		}
		os.WriteFile(tunnelPath, u16, 0644)
		fmt.Printf("  sjis_ext.bin: %d tunnel chars\n", len(tunnel.Mappings))
	}

	// Auto-patch yscfg.ybn for no-pack
	yscfgPath := filepath.Join(outputDir, "yscfg.ybn")
	if data, err := os.ReadFile(yscfgPath); err == nil && len(data) >= 0x48 {
		patched := make([]byte, len(data))
		copy(patched, data)
		for _, off := range []int{0x3C, 0x40, 0x44} {
			patched[off] = 1
			patched[off+1] = 0
			patched[off+2] = 0
			patched[off+3] = 0
		}
		os.WriteFile(yscfgPath, patched, 0644)
		fmt.Printf("  yscfg.ybn no-pack flags set\n")
	}

	fmt.Printf("\nDone. %d files → %s/\n", files, outputDir)
}

func indexBytes(s, sub []byte) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if bytes.Equal(s[i:i+len(sub)], sub) {
			return i
		}
	}
	return -1
}
