# YU-RIS TOOLS

YU-RIS/ERIS 引擎视觉小说翻译工具集（Go 实现）。

## 功能

```
yrt <命令> [参数]

命令:
  extract      <文件.ypf> [输出目录]                 YPF 封包解包
  extract-text <文件.ybn|目录> [-o out.json]          提取对话文本到 JSON
  inject-text  <ybn目录> -t <翻译.json> [-o out_dir]  注入翻译到 YBN
  decompile    <文件.ybn|目录> [-o 输出]               YBN → 可读 YST 脚本
  compile      <yst目录> -o <ybn目录> --original <源ybn目录> [--original-yst <源yst目录>]
                                                      YST 翻译 → YBN
  stats        <文件.ybn>                             指令统计
  keyfind      <文件.ybn|目录>                         XOR 密钥恢复
  verify       <文件.ybn|目录>                         Round-trip 验证
```

## 翻译流程

### 1. 从游戏封包提取脚本

```bash
yrt extract 游戏/pac/ysbin.ypf original_ybn/
```

### 2. 提取对话文本

```bash
yrt extract-text original_ybn/ -o texts.json
```

输出 JSON：
```json
[
  {"file": "yst00066.ybn", "seq": 2, "opcode": "0x6A", "offset": 617, "length": 62, "text": "ふふっ、センパイ♪　それじゃあ……"},
  {"file": "yst00066.ybn", "seq": 3, "opcode": "0x36", "offset": 1207, "length": 74, "text": "そんな色気もない言葉を…"}
]
```

### 3. 翻译

编辑 JSON 中的 `text` 字段为中文。可以批量替换、AI 翻译或手动编辑。

### 4. 注入翻译

```bash
yrt inject-text original_ybn/ -t texts.json -o ybn_translated/
```

- 对比原文和译文，仅注入有变化的行
- 自动生成 `sjis_ext.bin`（隧道字符映射表）
- 自动设置 `yscfg.ybn` 免封包标志
- 输出可以直接部署到游戏 `ysbin/` 目录

### 5. 部署到游戏

```bash
cp ybn_translated/*.ybn 游戏/ysbin/
cp ybn_translated/sjis_ext.bin 游戏/
```

将 `version.dll`（VNTextProxy，从 [VNTranslationTools](https://github.com/arcusmaximus/VNTranslationTools) 获取）和 CJK 字体放到游戏 exe 同目录。

> **注意**：译文 SJIS 隧道编码后的字节数不能超过原文，否则注入会被跳过。中文通常比日文短，大多数情况满足。

### 备选：YST 编辑流程

如果需要在脚本级别查看/编辑（非纯文本），可以用反编译→编辑→编译流程：

```bash
# 反编译
yrt decompile original_ybn/ -o yst_original/
cp -r yst_original/ yst_translated/

# 编辑 yst_translated/ 中的 WORD["..."] / SPEAKER["..."] 文本

# 编译
yrt compile yst_translated/ -o ybn_out/ --original original_ybn/ --original-yst yst_original/
```

## YBN 格式

- **魔数**: `YSTB`
- **加密**: 4 字节 XOR 循环，密钥从文件自动检测
- **结构**: 32 字节头 + cmd_sec + para_sec + str_sec + other_sec
- **关键 opcode**:
  - `0x6A` WORD — 对话框文本
  - `0x36` SPEAKER — 说话人文本
  - `0x2D` SCRIPT_DUMP — 字节码块（不可安全修改）

## SJIS 隧道编码

YU-RIS 引擎内部使用 Shift-JIS 编码，约 23% 简体中文汉字无法表示。

**原理**：`inject-text` 注入时将中文映射到未使用的 SJIS 码点，同时生成 `sjis_ext.bin` 映射表。运行时由代理 DLL 将隧道码点还原为正确字形。

**所需文件**：

| 文件 | 来源 |
|------|------|
| `version.dll` | [VNTranslationTools](https://github.com/arcusmaximus/VNTranslationTools)（可改名为 d3d9.dll/winmm.dll 等） |
| `sjis_ext.bin` | `inject-text` 自动生成 |
| CJK 字体 | NotoSansCJK-Regular.ttc 等 |

全部放到游戏 exe 同目录即可。

## 快速验证

```bash
# 查看指令分布
./yrt stats original_ybn/yst00066.ybn

# 恢复 XOR 密钥
./yrt keyfind original_ybn/

# 批量 round-trip 验证
./yrt verify original_ybn/
```

## 架构

```
├── main.go              # CLI 入口，6 个子命令
├── internal/
│   ├── ybn/
│   │   ├── ybn.go       # YBNFile: 读写/解析/XOR
│   │   ├── key.go       # recoverKey: 密钥自动恢复
│   │   ├── tunnel.go    # SjisTunnel: SJIS 隧道编码
│   │   ├── opcodes.go   # 操作码表
│   │   ├── decompile.go # 反编译器\n│   │   ├── compile.go   # 编译器 (YST→YBN)\n│   │   └── extract.go   # 文本提取/注入
│   └── ypf/
│       └── ypf.go       # YPF 封包提取
```

## 许可证

MIT License — 详见 [LICENSE](LICENSE)。
