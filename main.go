package main

import (
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
	fmt.Println(`YU-RIS Tools (Go) - YBN/YPF manipulation tool

Usage:
  yu-ris-tools-go <command> [options]

Commands:
  extract   <input.ypf> [output_dir]   Extract YPF archive
  decompile <input.ybn> [-o output.yst]  Decompile YBN to YST
  compile   <yst_dir> -o <ybn_dir> --original <ybn_dir> [--original-yst <yst_dir>]
  stats     <input.ybn>                Show opcode statistics
  keyfind   <file.ybn|dir/>            Recover XOR key
  verify    <file.ybn|dir/>            Verify round-trip

Examples:
  yu-ris-tools-go extract ysbin.ypf extracted/
  yu-ris-tools-go decompile yst00066.ybn -o out.yst
  yu-ris-tools-go decompile original_ybn/ -o decompiled/
  yu-ris-tools-go compile yst/ -o ybn_out/ --original original_ybn/
  yu-ris-tools-go stats yst00066.ybn
  yu-ris-tools-go keyfind original_ybn/
  yu-ris-tools-go verify original_ybn/
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


