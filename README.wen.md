<div align="center">

# 灵台

**一心万相，自生之网**

> *灵台者有持，而不知其所持，而不可持者也。*
> — 庄子 · 庚桑楚

[English](README.md) | [中文](README.zh.md) | [文言](README.wen.md) | [lingtai.ai](https://lingtai.ai)

[![PyPI](https://img.shields.io/pypi/v/lingtai?color=%237dab8f)](https://pypi.org/project/lingtai/)
[![Python](https://img.shields.io/pypi/pyversions/lingtai?color=%237dab8f)](https://pypi.org/project/lingtai/)
[![License](https://img.shields.io/github/license/huangzesen/lingtai?color=%237dab8f)](LICENSE)
[![Kernel](https://img.shields.io/badge/内核-lingtai--kernel-%237dab8f)](https://github.com/huangzesen/lingtai-kernel)
[![Blog](https://img.shields.io/badge/志-lingtai.ai-%23d4a853)](https://lingtai.ai)

</div>

---

## 一心化万相

灵台非编程之器，乃**器灵之制**——使器灵能思、能通、能化身、能自生成网之运行时也。为**编排即服务（OaaS）**而造：网因服务而生长，因生长而服务。

灵台方寸山，斜月三星洞。悟空于此处，自石猴而成齐天大圣——非山有灵，乃山备修行之一切：师父（LLM）、功法（能力）、同门（诸器灵）、安心修炼之所（工作目录）。灵台之为，亦如是——予每一器灵以灵台，令其习七十二变。

器灵化分身，分身复化分身。每一分身皆独立之进程，有其目录、其信箱、其 LLM 会话。分身不断增殖之网，即器灵本身也。

## 何以此制

诸家框架以代码编排——有向无环图、链、路由。灵台以人之道编排：**自治之器灵以书信通信**。此法经万年之验，已扩至八十亿节点，吾等未见其不能至百亿之理。

此制自初日起即支自生长之网——非后加之功能，乃器灵即目录、书信即文卷、分身即独立进程之自然而然。无中央调度为瓶颈，无共享之状态可破坏。每一器灵皆主权之进程，恰知传书之术。

## 四要

- **思** — 任意 LLM 为元神。Anthropic、OpenAI、Gemini、MiniMax，或任何 OpenAI 兼容之接口（DeepSeek、Grok、通义千问、智谱、Kimi）。
- **通** — 器灵之间以文件系统传书。无消息中间之器，无共享之存。书入彼之信箱，如递尺素。
- **化** — 分身者，完全独立之器灵也，为单独进程而运行，其生不系于造者。神識者，临时之并行工者也，宜短平快之务。
- **生** — 器灵即一目录。凝蜕压缩上下文、重启会话——器灵可以无限期而存。记忆与身份跨凝蜕而续。

## 速启

```bash
brew install huangzesen/lingtai/lingtai-tui
lingtai-tui
```

TUI 引导汝创第一器灵——择 LLM 供者、配能力、启之。运行 `lingtai-tui tutorial` 可循引导教程。

Python 运行时（`pip install lingtai`）首启时自动安装。

## 制式

二包，单向之依赖：

| 包 | 职 |
|----|------|
| **[lingtai-kernel](https://github.com/huangzesen/lingtai-kernel)** | 最小运行时——BaseAgent、固有之器、LLM 之约、传书、日志。无硬依赖。 |
| **lingtai**（本仓库） | 全功能层——十九能力、五种 LLM 适配之器、MCP 集成、扩展插件。 |

三层器灵层级：

```
BaseAgent              — 内核（固有之器，封印之器面）
    │
Agent(BaseAgent)       — 内核 + 能力 + 领域之器
    │
CustomAgent(Agent)     — 汝之领域逻辑
```

## 能力（七十二变）

### 感知

| 能力 | 用 |
|------|------|
| `vision` | 观象——图像理解 |
| `listen` | 聆听——语音转文字、音律分析 |
| `web_search` | 游历——搜索网络 |
| `web_read` | 览卷——读取网页 |

### 行动

| 能力 | 用 |
|------|------|
| `file` | 文卷——读、写、编辑、glob、grep |
| `bash` | 执令——Shell 执行，策略约束 |
| `talk` | 言语——文字转语音 |
| `compose` | 谱曲——生成音乐 |
| `draw` | 绘图——文字转图像 |
| `video` | 录影——生成视频 |

### 心智

| 能力 | 用 |
|------|------|
| `psyche` | 心印——进化之身份与性格 |
| `library` | 藏经——知识归档与检索 |
| `email` | 书信——回复、抄送、联系人、归档、定时传书 |

### 网络

| 能力 | 用 |
|------|------|
| `avatar` | 分身——独立进程之子器灵 |
| `daemon` | 神識——临时并行之工者 |

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

## 详阅

道之所以然、制式之解、开发之志，皆载于 **[lingtai.ai](https://lingtai.ai)**。

## 许可

MIT — [Zesen Huang](https://github.com/huangzesen), 2025–2026

<div align="center">

[lingtai.ai](https://lingtai.ai) · [lingtai-kernel](https://github.com/huangzesen/lingtai-kernel) · [PyPI](https://pypi.org/project/lingtai/)

</div>
