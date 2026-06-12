# YU-RIS TOOLS

YU-RIS/ERIS 引擎视觉小说翻译工具集（Go 实现）。

## 功能

```
yu-ris-tools-go <命令> [参数]

命令:
  extract   <文件.ypf> [输出目录]     YPF 封包解包
  decompile <文件.ybn|目录> [-o 输出]  YBN → 可读 YST 脚本
  compile   <yst目录> -o <ybn目录> --original <源ybn目录> [--original-yst <源yst目录>]
                                      翻译后 YST → 翻译后 YBN
  stats     <文件.ybn>                 指令统计
  keyfind   <文件.ybn|目录>            XOR 密钥恢复
  verify    <文件.ybn|目录>            Round-trip 验证
```

## 翻译流程

### 1. 从游戏封包提取脚本

```bash
./yu-ris-tools-go extract 游戏/pac/ysbin.ypf original_ybn/
```

### 2. 反编译为可读脚本

```bash
# 批量反编译
./yu-ris-tools-go decompile original_ybn/ -o yst_original/

# 单个文件
./yu-ris-tools-go decompile original_ybn/yst00066.ybn -o script.yst
```

输出示例：

```
WORD["ふふっ、センパイ♪"]     // line 30
SPEAKER["おつかれさま"]        // line 183
// === SCRIPT_DUMP // line 180 ===
    PSTR("es.SND")
```

### 3. 编辑翻译

复制原始 YST 并编辑：

```bash
cp -r yst_original/ yst_translated/
```

修改 `WORD["日文"]` 和 `SPEAKER["日文"]` 为中文：

```diff
- WORD["ふふっ、センパイ♪"]
+ WORD["嘻嘻、前辈♪"]
```

### 4. 编译注入

```bash
# diff 模式（推荐）：对比原始和翻译后的 YST，仅注入变更
./yu-ris-tools-go compile yst_translated/ \
    -o ybn_output/ \
    --original original_ybn/ \
    --original-yst yst_original/
```

编译器使用**骨架重建**策略：保留原始 YBN 的指令结构，仅将翻译文本原位注入字符串池。

### 5. 部署到游戏

```bash
cp ybn_output/*.ybn 游戏/ysbin/
```

`yscfg.ybn` 需设置免封包标志：偏移 0x3C、0x40、0x44 为 `01 00 00 00`。

### 6. 测试

```bash
cd 游戏目录 && LC_ALL=ja_JP.UTF-8 wine 游戏.exe
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

**原理**：不可编码的汉字映射到未使用的 SJIS 码点，运行时由代理 DLL 还原。

**所需文件**（需从其他项目获取，非本仓库提供）：

| 文件 | 用途 | 来源 |
|------|------|------|
| `version.dll` | VNTextProxy，挂钩 GDI/Direct2D 文本渲染 | [VNTranslationTools](https://github.com/arcusmaximus/VNTranslationTools) |
| `sjis_ext.bin` | 隧道字符映射表 | 由本工具编译时自动生成 |

部署方式：将 `version.dll`（可改名为 `d3d9.dll`、`ddraw.dll` 等）、`sjis_ext.bin` 和 CJK 字体（如 NotoSansCJK-Regular.ttc）放到游戏 exe 同目录。

## 快速验证

```bash
# 查看指令分布
./yu-ris-tools-go stats original_ybn/yst00066.ybn

# 恢复 XOR 密钥
./yu-ris-tools-go keyfind original_ybn/

# 批量 round-trip 验证
./yu-ris-tools-go verify original_ybn/
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
│   │   ├── decompile.go # 反编译器
│   │   └── compile.go   # 编译器 (YST→YBN)
│   └── ypf/
│       └── ypf.go       # YPF 封包提取
```

## 许可证

MIT License — 详见 [LICENSE](LICENSE)。
