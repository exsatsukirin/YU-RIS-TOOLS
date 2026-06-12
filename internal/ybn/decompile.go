package ybn

import (
	"encoding/binary"
	"fmt"
	"strings"
)

type Instr struct {
	CmdIdx     int
	Opcode     byte
	ParamCount byte
	LineNum    uint32
	Mnemonic   string
	Params     []ParaEntry
	TextRefs   []TextRef
	EntryIdx   int
}

type BytecodeInstr struct {
	Mnemonic string
	Args     []string
	Offset   int
}

type Disassembler struct {
	*YBNFile
}

func NewDisassembler(y *YBNFile) *Disassembler {
	return &Disassembler{y}
}

func (d *Disassembler) Disassemble() []Instr {
	cmds := d.GetCmdList()
	entries := d.GetParaEntries()
	var result []Instr
	entryIdx := 0

	for cmdIdx, cmd := range cmds {
		lineNum := uint32(0)
		if cmdIdx*4+3 < len(d.OtherSec) {
			lineNum = binary.LittleEndian.Uint32(d.OtherSec[cmdIdx*4:])
		}

		var cmdParams []ParaEntry
		pc := int(cmd.ParamCount)
		if pc > 0 && entryIdx+pc <= len(entries) {
			cmdParams = entries[entryIdx : entryIdx+pc]
		}

		textRefs := resolveTexts(d.StrSec, cmd.Opcode, cmdParams)

		mnemonic := OpcodeNames[cmd.Opcode]
		if mnemonic == "" {
			mnemonic = fmt.Sprintf("UNK_%02X", cmd.Opcode)
		}

		result = append(result, Instr{
			CmdIdx:     cmdIdx,
			Opcode:     cmd.Opcode,
			ParamCount: cmd.ParamCount,
			LineNum:    lineNum,
			Mnemonic:   mnemonic,
			Params:     cmdParams,
			TextRefs:   textRefs,
			EntryIdx:   entryIdx,
		})

		entryIdx += pc
	}

	return result
}

func resolveTexts(strSec []byte, opcode byte, params []ParaEntry) []TextRef {
	var refs []TextRef
	jpLeft := []byte{0x81, 0x75}
	jpRight := []byte{0x81, 0x76}

	if opcode == 0x6A || opcode == 0x36 {
		// For SPEAKER (0x36): param_count=2, dialogue is in the LAST para
		// For WORD (0x6A): param_count=1, the only para
		idx := 0
		if opcode == 0x36 && len(params) >= 2 {
			idx = len(params) - 1 // use last para (dialogue text)
		}
		if idx < len(params) {
			p := params[idx]
			if p.Length > 0 && p.Length < 0x10000 && int(p.Offset+p.Length) <= len(strSec) {
				raw := strSec[p.Offset : p.Offset+p.Length]
				dialogueText := extractDialogue(raw, jpLeft, jpRight)
				if dialogueText == "" {
					dialogueText = decodeSJIS(raw)
				}
				hexStr := rawHex(raw, 80)
				refs = append(refs, TextRef{
					Type:   "TEXT",
					Offset: p.Offset,
					Length: p.Length,
					Text:   dialogueText,
					RawHex: hexStr,
				})
			}
		}
	} else if len(params) > 0 {
		for _, p := range params {
			if p.Length > 0 && p.Length < 0x100000 && int(p.Offset+p.Length) <= len(strSec) {
				data := strSec[p.Offset : p.Offset+p.Length]
				strEntries := extractBytecodeStrings(data)
				hexStr := ""
				for i := 0; i < len(data) && i < 80; i++ {
					hexStr += fmt.Sprintf("%02x ", data[i])
				}
				refs = append(refs, TextRef{
					Type:    "BYTECODE",
					Offset:  p.Offset,
					Length:  p.Length,
					Pre:     p.Pre,
					RawHex:  strings.TrimSpace(hexStr),
					Strings: strEntries,
				})
			}
		}
	}

	return refs
}

func extractBytecodeStrings(data []byte) []StringEntry {
	var result []StringEntry
	i := 0
	for i < len(data)-4 {
		if data[i] == 0x4D && i+3 <= len(data) {
			strlen := int(binary.LittleEndian.Uint16(data[i+1:]))
			if strlen > 0 && strlen < 2000 && i+4+strlen <= len(data) {
				payloadStart := i + 3
				if payloadStart < len(data) && data[payloadStart] == 0x22 {
					end := -1
					for j := payloadStart + 1; j < len(data); j++ {
						if data[j] == 0x22 {
							end = j
							break
						}
					}
					if end > payloadStart {
						inner := data[payloadStart+1 : end]
						s := decodeSJIS(inner)
						result = append(result, StringEntry{i, s, "PSTR"})
					}
				}
			}
		}
		i++
	}
	return result
}

func rawHex(data []byte, maxLen int) string {
	var b strings.Builder
	for i := 0; i < len(data) && i < maxLen; i++ {
		fmt.Fprintf(&b, "%02x ", data[i])
	}
	return strings.TrimSpace(b.String())
}

// ── ERIS Bytecode Parser ──

type ERISOp struct {
	Mnemonic      string
	FixedArgBytes int
	HasVarString  bool
}

var erisOps = map[byte]ERISOp{
	0x4D: {"PSTR", 2, true},
	0x56: {"PUSH_VAR", 3, false},
	0x57: {"POP_VAR", 3, false},
	0x42: {"BR", 3, false},
	0x29: {"CALL", 4, false},
	0x21: {"ENDSTMT", 0, false},
	0x2B: {"END_BLOCK", 0, false},
	0x3D: {"ASSIGN", 0, false},
	0x13: {"SEP", 0, false},
	0x22: {"QUOTE", 0, false},
	0x40: {"IMM", 1, false},
}

type BytecodeParser struct{}

func (p *BytecodeParser) Parse(data []byte, baseOffset int) []BytecodeInstr {
	var instrs []BytecodeInstr
	i := 0
	maxIter := len(data) * 3

	for i < len(data) && maxIter > 0 {
		maxIter--
		op := data[i]
		info, ok := erisOps[op]
		if !ok {
			i++
			continue
		}

		var args []string

		if info.HasVarString && i+3 <= len(data) {
			strlen := int(binary.LittleEndian.Uint16(data[i+1:]))
			if strlen > 0 && strlen < 5000 && i+4+strlen <= len(data) {
				payloadStart := i + 3
				if payloadStart < len(data) && data[payloadStart] == 0x22 {
					end := -1
					for j := payloadStart + 1; j < len(data); j++ {
						if data[j] == 0x22 {
							end = j
							break
						}
					}
					if end > payloadStart {
						inner := data[payloadStart+1 : end]
						s := decodeSJIS(inner)
						args = append(args, fmt.Sprintf("%q", s))
						i = end + 1
						instrs = append(instrs, BytecodeInstr{info.Mnemonic, args, baseOffset + i})
						continue
					}
				}
				raw := data[payloadStart : payloadStart+strlen]
				args = append(args, fmt.Sprintf("0x%x", raw))
				i = payloadStart + strlen
				instrs = append(instrs, BytecodeInstr{info.Mnemonic, args, baseOffset + i})
				continue
			}
		}

		if info.FixedArgBytes > 0 && i+1+info.FixedArgBytes <= len(data) {
			for j := 0; j < info.FixedArgBytes; j++ {
				args = append(args, fmt.Sprintf("0x%02X", data[i+1+j]))
			}
			i = i + 1 + info.FixedArgBytes
		} else {
			i++
		}

		instrs = append(instrs, BytecodeInstr{info.Mnemonic, args, baseOffset + i})
	}

	if maxIter <= 0 && i < len(data) {
		instrs = append(instrs, BytecodeInstr{"...", nil, i})
	}

	return instrs
}

func (p *BytecodeParser) ToPseudoERIS(instrs []BytecodeInstr, indent string) string {
	var lines []string
	var current []string
	pendingAssign := false

	for _, inst := range instrs {
		switch inst.Mnemonic {
		case "ENDSTMT":
			if len(current) > 0 {
				lines = append(lines, indent+strings.Join(current, " ")+";")
				current = nil
			}
		case "SEP":
			if len(current) > 0 {
				current[len(current)-1] += ","
			}
		case "QUOTE":
			current = append(current, `"`)
		case "PSTR":
			if len(inst.Args) > 0 {
				current = append(current, inst.Args[0])
			} else {
				current = append(current, `""`)
			}
		case "PUSH_VAR":
			vid := strings.Join(inst.Args, "_")
			if vid == "" {
				vid = "?"
			}
			current = append(current, fmt.Sprintf("@var_%s", vid))
		case "POP_VAR":
			vid := strings.Join(inst.Args, "_")
			if vid == "" {
				vid = "?"
			}
			pendingAssign = true
			current = append([]string{fmt.Sprintf("@var_%s = ", vid)}, current...)
		case "CALL":
			fid := strings.Join(inst.Args, "_")
			if fid == "" {
				fid = "?"
			}
			current = append(current, fmt.Sprintf("CALL_%s()", fid))
		case "END_BLOCK":
			if len(current) > 0 {
				lines = append(lines, indent+strings.Join(current, " "))
				current = nil
			}
			lines = append(lines, "")
		case "ASSIGN":
			if len(current) > 0 && !pendingAssign {
				current = append(current, "=")
			}
		case "BR":
			target := strings.Join(inst.Args, "_")
			if target == "" {
				target = "?"
			}
			current = append(current, fmt.Sprintf("BR[%s]", target))
		case "...":
			lines = append(lines, indent+"// ...TRUNCATED")
		default:
			current = append(current, fmt.Sprintf("%s[%s]", inst.Mnemonic, strings.Join(inst.Args, ",")))
		}
	}

	if len(current) > 0 {
		lines = append(lines, indent+strings.Join(current, " "))
	}

	return strings.Join(lines, "\n")
}

// ── YST Formatter ──

type YSTFormatter struct {
	*Disassembler
	indentStr string
	parser    BytecodeParser
}

func NewYSTFormatter(d *Disassembler) *YSTFormatter {
	return &YSTFormatter{
		Disassembler: d,
		indentStr:    "\t",
	}
}

func (f *YSTFormatter) Format() string {
	instrs := f.Disassemble()
	var lines []string

	lines = append(lines, "//=========================================================================")
	lines = append(lines, fmt.Sprintf("// Decompiled from YBN (version %d)", f.Version))
	lines = append(lines, fmt.Sprintf("// Instructions: %d total", len(instrs)))
	lines = append(lines, fmt.Sprintf("// Strings pool: %d bytes", f.StrLen))
	lines = append(lines, "// Opcode distribution:")
	stats := make(map[byte]int)
	for _, inst := range instrs {
		stats[inst.Opcode]++
	}
	for op := 0; op <= 0xFF; op++ {
		b := byte(op)
		if n, ok := stats[b]; ok {
			name := OpcodeNames[b]
			if name == "" {
				name = fmt.Sprintf("UNK_%02X", b)
			}
			desc := OpcodeDescriptions[b]
			comment := ""
			if desc != "" {
				comment = fmt.Sprintf("\t// %s", desc)
			}
			lines = append(lines, fmt.Sprintf("//   0x%02X (%s): %d%s", b, name, n, comment))
		}
	}
	lines = append(lines, "//=========================================================================")
	lines = append(lines, "")

	lastLine := uint32(0)

	for _, inst := range instrs {
		ln := inst.LineNum

		if ln > 0 && ln > lastLine+5 {
			lines = append(lines, "")
		}

		formatted := f.formatInstr(inst)
		if formatted != "" {
			lines = append(lines, formatted)
		}

		lastLine = ln
	}

	return strings.Join(lines, "\n") + "\n"
}

func (f *YSTFormatter) formatInstr(inst Instr) string {
	tag := ""
	if inst.LineNum > 0 {
		tag = fmt.Sprintf("\t// line %d", inst.LineNum)
	}

	switch inst.Opcode {
	case 0x6A:
		text := ""
		if len(inst.TextRefs) > 0 {
			text = inst.TextRefs[0].Text
		}
		escaped := strings.ReplaceAll(text, "\\", "\\\\")
		escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
		return fmt.Sprintf("%sWORD[%q]%s", f.indentStr, escaped, tag)

	case 0x36:
		text := ""
		if len(inst.TextRefs) > 0 {
			text = inst.TextRefs[0].Text
		}
		escaped := strings.ReplaceAll(text, "\\", "\\\\")
		escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
		return fmt.Sprintf("%sSPEAKER[%q]%s", f.indentStr, escaped, tag)

	case 0x2D:
		return f.formatScriptDump(inst, tag)

	case 0x2C:
		if len(inst.Params) > 0 {
			p := inst.Params[0]
			return fmt.Sprintf("%sGOSUB[%d, %d, %d]%s", f.indentStr, p.Pre, p.Length, p.Offset, tag)
		}
		return fmt.Sprintf("%sGOSUB[]%s", f.indentStr, tag)

	case 0x0E:
		return fmt.Sprintf("%sRETURN[]%s", f.indentStr, tag)

	default:
		if len(inst.Params) > 0 {
			var parts []string
			for _, p := range inst.Params {
				parts = append(parts, fmt.Sprintf("[%d, %d, %d]", p.Pre, p.Length, p.Offset))
			}
			return fmt.Sprintf("%s%s(%s)%s", f.indentStr, inst.Mnemonic, strings.Join(parts, "; "), tag)
		}
		return fmt.Sprintf("%s%s[]%s", f.indentStr, inst.Mnemonic, tag)
	}
}

func (f *YSTFormatter) formatScriptDump(inst Instr, tag string) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("%s// === SCRIPT_DUMP %s ===", f.indentStr, strings.TrimLeft(tag, "\t")))
	indent2 := f.indentStr + f.indentStr

	for _, ref := range inst.TextRefs {
		if ref.Type == "BYTECODE" {
			lines = append(lines, fmt.Sprintf("%s// type_pre=0x%04X, %d bytes", indent2, ref.Pre, ref.Length))

			for _, s := range ref.Strings {
				escaped := strings.ReplaceAll(s.Text, "\\", "\\\\")
				lines = append(lines, fmt.Sprintf(`%sPSTR("%s")`, indent2, escaped))
			}

			data := f.StrSec[ref.Offset : ref.Offset+ref.Length]
			parsed := f.parser.Parse(data, int(ref.Offset))
			if len(parsed) > 0 {
				lines = append(lines, indent2+"// --- pseudo-ERIS ---")
				pseudo := f.parser.ToPseudoERIS(parsed, indent2)
				if strings.TrimSpace(pseudo) != "" {
					lines = append(lines, pseudo)
				}
			}

			if len(ref.RawHex) <= 160 {
				lines = append(lines, fmt.Sprintf("%s// hex: %s", indent2, ref.RawHex))
			} else {
				totalLen := len(ref.RawHex) / 3
				lines = append(lines, fmt.Sprintf("%s// (%d raw bytes omitted)", indent2, totalLen))
			}
		}
	}

	lines = append(lines, fmt.Sprintf("%s// === END SCRIPT_DUMP ===", f.indentStr))
	return strings.Join(lines, "\n")
}

// ── Text replacement ──

func (y *YBNFile) ReplaceText(translationMap map[string]string, tunnel *SjisTunnel) int {
	cmds := y.GetCmdList()
	entries := y.GetParaEntries()
	replaced := 0
	entryIdx := 0
	strData := make([]byte, len(y.StrSec))
	copy(strData, y.StrSec)

	jpLeft := []byte{0x81, 0x75}
	jpRight := []byte{0x81, 0x76}

	for _, cmd := range cmds {
		pc := int(cmd.ParamCount)
		if pc > 0 && entryIdx+pc <= len(entries) {
			if cmd.Opcode == 0x6A || cmd.Opcode == 0x36 {
				for ei := 0; ei < pc; ei++ {
					e := entries[entryIdx+ei]
					oldLen := e.Length
					offset := e.Offset
					if !(oldLen > 0 && oldLen < 0x10000 && int(offset+oldLen) <= len(strData)) {
						continue
					}
					oldRaw := strData[offset : offset+oldLen]

					newRaw, nQuote, newLen := replaceInBlockRaw(oldRaw, translationMap, tunnel, jpLeft, jpRight)
					off := int(offset)
					oL := int(oldLen)
					nL := int(newLen)
					if nQuote > 0 {
						if nL <= oL {
							copy(strData[off:off+nL], newRaw[:nL])
							if nL < oL {
								for k := off + nL; k < off+oL; k++ {
									strData[k] = 0
								}
							}
							entries[entryIdx+ei].Length = newLen
							replaced += nQuote
						}
					} else if cmd.Opcode == 0x6A {
						oldText := decodeSJIS(oldRaw)
						if cn, ok := translationMap[oldText]; ok {
							nb := tunnel.Encode(cn)
							if len(nb) <= oL {
								copy(strData[off:], nb)
								if len(nb) < oL {
									for k := off + len(nb); k < off+oL; k++ {
										strData[k] = 0
									}
								}
								entries[entryIdx+ei].Length = uint32(len(nb))
								replaced++
							}
						}
					}
				}
			}
		}
		entryIdx += pc
	}

	y.StrSec = strData
	y.SetParaEntries(entries)
	return replaced
}

func replaceInBlockRaw(block []byte, translationMap map[string]string, tunnel *SjisTunnel, jpLeft, jpRight []byte) ([]byte, int, uint32) {
	result := make([]byte, len(block))
	copy(result, block)
	replaced := 0
	lastEnd := -1
	i := 0

	for i < len(result) {
		if i == 0 && (len(result) < 2 || result[0] != jpLeft[0] || result[1] != jpLeft[1]) {
			rp := bytesIndexOf(result, jpRight, 0)
			if rp >= 0 {
				dialogue := result[:rp]
				dt := strings.TrimRight(decodeSJIS(dialogue), " \t\n\r")
				if cn, ok := translationMap[dt]; ok {
					nd := tunnel.Encode(cn)
					if len(nd) < len(dialogue) {
						copy(result[:len(nd)], nd)
						copy(result[len(nd):len(nd)+2], jpRight)
						replaced++
						lastEnd = len(nd) + 2
						i = rp + 2
						continue
					} else if len(nd) == len(dialogue) {
						copy(result[:len(nd)], nd)
						replaced++
						lastEnd = len(result)
						i = rp + 2
						continue
					}
				}
			}
		}

		if i+1 < len(result) && result[i] == jpLeft[0] && result[i+1] == jpLeft[1] {
			found := false
			for j := i + 2; j < len(result)-1; j++ {
				if result[j] == jpRight[0] && result[j+1] == jpRight[1] {
					dialogue := result[i+2 : j]
					dt := strings.TrimRight(decodeSJIS(dialogue), " \t\n\r")
					if cn, ok := translationMap[dt]; ok {
						nd := tunnel.Encode(cn)
						if len(nd) < len(dialogue) {
							copy(result[i+2:i+2+len(nd)], nd)
							copy(result[i+2+len(nd):i+2+len(nd)+2], jpRight)
							replaced++
							lastEnd = i + 2 + len(nd) + 2
						} else if len(nd) == len(dialogue) {
							copy(result[i+2:i+2+len(nd)], nd)
							replaced++
							lastEnd = len(result)
						}
					}
					i = j + 2
					found = true
					break
				}
			}
			if !found {
				dialogue := result[i+2:]
				dt := strings.TrimRight(decodeSJIS(dialogue), " \t\n\r")
				if cn, ok := translationMap[dt]; ok {
					nd := tunnel.Encode(cn)
					if len(nd) < len(dialogue) {
						copy(result[i+2:i+2+len(nd)], nd)
						copy(result[i+2+len(nd):i+2+len(nd)+2], jpRight)
						replaced++
						lastEnd = i + 2 + len(nd) + 2
					} else if len(nd) == len(dialogue) {
						copy(result[i+2:i+2+len(nd)], nd)
						replaced++
					}
				}
				i = len(result)
			}
		} else if result[i] == 0x22 {
			found := false
			for j := i + 1; j < len(result); j++ {
				if result[j] == 0x22 {
					newEnd, n := replaceInASCIIQuotes(result, i, j, translationMap, tunnel, jpLeft, jpRight)
					if n > 0 {
						replaced += n
						lastEnd = newEnd
					}
					i = j + 1
					found = true
					break
				}
			}
			if !found {
				i++
			}
		} else {
			i++
		}
	}

	truncatedLen := uint32(len(block))
	if replaced > 0 && lastEnd >= 0 {
		truncatedLen = uint32(lastEnd)
	}
	return result, replaced, truncatedLen
}

func replaceInASCIIQuotes(result []byte, i, j int, translationMap map[string]string, tunnel *SjisTunnel, jpLeft, jpRight []byte) (int, int) {
	inner := result[i+1 : j]
	lp := bytesIndexOf(inner, jpLeft, 0)
	rp := -1
	if lp >= 0 {
		rp = bytesIndexOf(inner, jpRight, lp+2)
	}

	if lp >= 0 && rp >= 0 {
		dialogue := result[i+1+lp+2 : i+1+rp]
		dt := decodeSJIS(dialogue)
		if cn, ok := translationMap[dt]; ok {
			nd := tunnel.Encode(cn)
			if len(nd) < len(dialogue) {
				absLp := i + 1 + lp
				copy(result[absLp+2:absLp+2+len(nd)], nd)
				copy(result[absLp+2+len(nd):absLp+2+len(nd)+2], jpRight)
				result[absLp+2+len(nd)+2] = 0x22
				return absLp + 2 + len(nd) + 3, 1
			}
		}
	} else {
		dialogue := result[i+1 : j]
		dt := decodeSJIS(dialogue)
		if cn, ok := translationMap[dt]; ok {
			nd := tunnel.Encode(cn)
			if len(nd) < len(dialogue) {
				copy(result[i+1:i+1+len(nd)], nd)
				result[i+1+len(nd)] = 0x22
				return i + 1 + len(nd) + 1, 1
			}
		}
	}
	return -1, 0
}

func bytesIndexOf(data []byte, needle []byte, start int) int {
	for i := start; i <= len(data)-len(needle); i++ {
		match := true
		for j := 0; j < len(needle); j++ {
			if data[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
