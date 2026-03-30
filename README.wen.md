<div align="center">

# 灵台

**器灵之制 — 一心万相，自生之网**

> *灵台者有持，而不知其所持，而不可持者也。*
> — 庄子 · 庚桑楚

[English](README.md) | [中文](README.zh.md) | [文言](README.wen.md) | [lingtai.ai](https://lingtai.ai)

[![Homebrew](https://img.shields.io/badge/brew-lingtai--tui-%237dab8f)](https://github.com/huangzesen/homebrew-lingtai)
[![License](https://img.shields.io/github/license/huangzesen/lingtai?color=%237dab8f)](LICENSE)
[![Kernel](https://img.shields.io/badge/内核-lingtai--kernel-%237dab8f)](https://github.com/huangzesen/lingtai-kernel)
[![Blog](https://img.shields.io/badge/志-lingtai.ai-%23d4a853)](https://lingtai.ai)

</div>

---

Unix 之道，器灵之制。**思**以任意 LLM。**通**以文件系统传书。**化分身**能脱造者而独存。**自生长**为不断扩展之网——无中央之调度，无共享之状态。万物皆文卷。

```bash
brew install huangzesen/lingtai/lingtai-tui
lingtai-tui
```

## 灵台之异

诸家框架以代码编排——有向无环图、链、路由。灵台以人之道编排：**自治之器灵以书信通信**。此法经万年之验，已扩至八十亿节点，吾等未见其不能至百亿之理。

| | DAG / 链式框架 | 灵台 |
|---|---|---|
| 编排 | 代码定义之流水线 | 器灵之间对话 |
| 扩展 | 增加步骤 | 器灵化出分身 |
| 记忆 | 共享状态 / 向量库 | 每器灵拥其目录 |
| 容错 | 流水线中断 | 一器灵眠，网络续运 |
| 增长 | 手动连线 | 自生长——分身复化分身 |

一相有涯。上下文之长，终有尽时。勿使身躯愈大。**令其遗忘，令网络记之。**

## 四要

- **思** — 任意 LLM 为元神。Anthropic、OpenAI、Gemini、MiniMax，或任何 OpenAI 兼容之接口（DeepSeek、Grok、通义千问、智谱、Kimi）。
- **通** — 器灵之间以文件系统传书。无消息中间之器，无共享之存。书入彼之信箱，如递尺素。
- **化** — 分身者，完全独立之器灵也，为单独进程而运行，其生不系于造者。神識者，临时之并行工者也，宜短平快之务。
- **生** — 器灵即一目录。凝蜕压缩上下文、重启会话——器灵可以无限期而存。记忆与身份跨凝蜕而续。

## 速启

TUI 引导汝创第一器灵——择 LLM 供者、配能力、启之。运行 `lingtai-tui tutorial` 可循引导教程。

```bash
brew install huangzesen/lingtai/lingtai-tui
lingtai-tui
```

Python 运行时（`pip install lingtai`）首启时自动安装。

<details>
<summary>不用 Homebrew</summary>

```bash
pip install lingtai
```

直接使用 Python API（见下方[扩展](#扩展)）。

</details>

## 制式

二包，单向之依赖：

| 包 | 职 |
|----|------|
| **[lingtai-kernel](https://github.com/huangzesen/lingtai-kernel)** | 最小运行时——BaseAgent、固有之器、LLM 之约、传书、日志。无硬依赖。 |
| **lingtai**（本仓库） | 全功能层——十九能力、五种 LLM 适配之器、MCP 集成、扩展插件。 |

```
BaseAgent              — 内核（固有之器，封印之器面）
    │
Agent(BaseAgent)       — 内核 + 能力 + 领域之器
    │
CustomAgent(Agent)     — 汝之领域逻辑
```

## 能力（七十二变）

<table>
<tr><th>感知</th><th>行动</th><th>心智</th><th>网络</th></tr>
<tr>
<td>

`vision` — 观象
`listen` — 聆听
`web_search` — 游历
`web_read` — 览卷

</td>
<td>

`file` — 文卷（读、写、编辑、glob、grep）
`bash` — 执令（Shell，策略约束）
`talk` — 言语
`compose` — 谱曲
`draw` — 绘图
`video` — 录影

</td>
<td>

`psyche` — 心印（进化之身份）
`library` — 藏经（知识归档）
`email` — 书信（完整信箱）

</td>
<td>

`avatar` — 分身（独立进程）
`daemon` — 神識（并行工者）

</td>
</tr>
</table>

## 器灵即目录

```
/agents/wukong/
  .agent.lock               ← 独占之锁
  .agent.heartbeat          ← 存活之证
  .agent.json               ← 清单
  system/
    covenant.md             ← 盟约（跨凝蜕而续）
    memory.md               ← 记忆
  mailbox/
    inbox/                  ← 所收之书信
    outbox/                 ← 待发之书信
    sent/                   ← 已发之记录
  logs/
    events.jsonl            ← 事件日志
```

无 `agent_id`。路径即身份。器灵以写入彼此之 `mailbox/inbox/` 通信——如投书于邻舍之门。

## 扩展

组合能力：

```python
agent = Agent(
    service=service,
    working_dir="/agents/bajie",
    capabilities=["file", "bash", "email", "avatar"],
)
```

子类化：

```python
class ResearchAgent(Agent):
    def __init__(self, **kwargs):
        super().__init__(
            capabilities=["file", "vision", "web_search", "avatar"],
            **kwargs,
        )
        self.add_tool("query_db", schema={...}, handler=db_handler)
```

接入 MCP 服务器：

```python
await agent.connect_mcp("npx -y @modelcontextprotocol/server-filesystem /data")
```

## 一心化万相

灵台方寸山，斜月三星洞。悟空于此处，自石猴而成齐天大圣——非山有灵，乃山备修行之一切：师父（LLM）、功法（能力）、同门（诸器灵）、安心修炼之所（工作目录）。灵台之为，亦如是——予每一器灵以灵台，令其习七十二变。

万物皆文卷。识、性、忆、缘，皆目录中之文卷也。词元为薪，非耗也，乃化——化为网络中之文卷，化为拓扑中之阅历。愈服务，网络愈广大、愈智慧。自生长之编排非后加之功能，乃器灵即目录、书信即文卷、分身即独立进程之自然而然。

一心化万相。

完整宣言见 [lingtai.ai](https://lingtai.ai)。

## 许可

MIT — [Zesen Huang](https://github.com/huangzesen), 2025–2026

<div align="center">

[lingtai.ai](https://lingtai.ai) · [lingtai-kernel](https://github.com/huangzesen/lingtai-kernel) · [PyPI](https://pypi.org/project/lingtai/)

</div>
