<div align="center">

# 灵台 LingTai

**智能体操作系统 — 自生长智能体编排**

> *灵台者有持，而不知其所持，而不可持者也。*
> — 庄子 · 庚桑楚

[English](README.md) | [中文](README.zh.md) | [文言](README.wen.md) | [lingtai.ai](https://lingtai.ai)

[![PyPI](https://img.shields.io/pypi/v/lingtai?color=%237dab8f)](https://pypi.org/project/lingtai/)
[![Python](https://img.shields.io/pypi/pyversions/lingtai?color=%237dab8f)](https://pypi.org/project/lingtai/)
[![License](https://img.shields.io/github/license/huangzesen/lingtai?color=%237dab8f)](LICENSE)
[![Kernel](https://img.shields.io/badge/内核-lingtai--kernel-%237dab8f)](https://github.com/huangzesen/lingtai-kernel)
[![Blog](https://img.shields.io/badge/博客-lingtai.ai-%23d4a853)](https://lingtai.ai)

</div>

---

## 一心化万相

灵台不是编程助手，而是一个**智能体操作系统**——让智能体思考、通信、化出分身、自己生长成网络的运行时。为**编排即服务（OaaS）**而生：网络因服务而生长，因生长而服务。

灵台方寸山，斜月三星洞。悟空在这里从一只猴子变成了齐天大圣——不是因为山本身有什么魔力，而是因为这里提供了修行所需的一切：师父（LLM）、功法（能力）、同门（其他智能体）、以及一个可以安心修炼的地方（工作目录）。灵台做的事情也是这样：给每个智能体一个灵台，让它学会七十二变。

智能体化出分身，分身再化分身。每个分身都是独立的进程，有自己的目录、自己的信箱、自己的 LLM 会话。分身不断增殖的网络，就是智能体本身。

## 编排即服务

上下文长度是单体问题。它永远是有限的。再怎么扩展也改变不了这一点——单个智能体终会遗忘。不要让身体变得更大，让它遗忘，让网络记住。

人之所以强大，不在个体，而在组织。平庸的个体组成的团体，其力量是相变式的——*more is different*。智能体亦然。多数智能体框架用代码编排——DAG、链、路由。灵台用人类的方式编排：**自治的智能体通过消息通信**。这套模式经过一万年的验证，已经扩展到 80 亿节点，我们没有理由认为它不能到 100 亿。

万物皆文件。知识、身份、记忆、关系——都是目录中的文件。每一个燃烧的 token 都不是消耗，而是转化——化为网络中的文件，化为拓扑中的经验。服务越多，网络越大、越智慧。自生长智能体编排不是后来加的功能，而是智能体即目录、信件即文件、分身即独立进程的自然结果。没有中央调度器成为瓶颈，没有共享状态会被破坏。网络即产品。

完整宣言见 [lingtai.ai](https://lingtai.ai)。

## 四个核心

- **思** — 任意 LLM 为元神。Anthropic、OpenAI、Gemini、MiniMax，或任何 OpenAI 兼容 API（DeepSeek、Grok、通义千问、智谱、Kimi）。
- **通** — 智能体之间通过文件系统传书通信。没有消息中间件，没有共享内存。写入对方的信箱，就像递一封信。
- **化** — 分身（avatar）是完全独立的智能体，作为单独进程运行，生存不依赖于创建者。神識（daemon）是临时的并行工作者，适合短平快的任务。
- **生** — 智能体就是一个目录。凝蜕（molt）压缩上下文、重启会话——智能体可以无限期存活。记忆和身份跨凝蜕存续。

## 快速开始

```bash
brew install huangzesen/lingtai/lingtai-tui
lingtai-tui
```

TUI 会引导你创建第一个智能体——选择 LLM 供应商、配置能力、启动。运行 `lingtai-tui tutorial` 可以体验引导式教程。

Python 运行时（`pip install lingtai`）会在首次启动时自动安装。

## 架构

两个包，单向依赖：

| 包 | 角色 |
|----|------|
| **[lingtai-kernel](https://github.com/huangzesen/lingtai-kernel)** | 最小运行时——BaseAgent、固有之器、LLM 协议、传书、日志。零硬依赖。 |
| **lingtai**（本仓库） | 全功能层——19 种能力、5 种 LLM 适配器、MCP 集成、扩展插件。 |

三层智能体层级：

```
BaseAgent              — 内核（固有之器，封闭工具面）
    │
Agent(BaseAgent)       — 内核 + 能力 + 领域工具
    │
CustomAgent(Agent)     — 你的领域逻辑
```

## 能力（七十二变）

### 感知

| 能力 | 用途 |
|------|------|
| `vision` | 图像理解 |
| `listen` | 语音转文字、音乐分析 |
| `web_search` | 搜索网络（DuckDuckGo、MiniMax、Gemini 等） |
| `web_read` | 读取网页内容 |

### 行动

| 能力 | 用途 |
|------|------|
| `file` | 读、写、编辑、glob、grep（组合简写） |
| `bash` | Shell 执行，基于策略的安全限制 |
| `talk` | 文字转语音 |
| `compose` | 生成音乐 |
| `draw` | 文字转图像 |
| `video` | 生成视频 |

### 心智

| 能力 | 用途 |
|------|------|
| `psyche` | 进化的身份与性格 |
| `library` | 知识归档与检索 |
| `email` | 完整信箱——回复、抄送、联系人、归档、定时发送 |

### 网络

| 能力 | 用途 |
|------|------|
| `avatar` | 化出分身——独立进程的子智能体 |
| `daemon` | 神識——临时并行工作者 |

## 智能体 = 目录

```
/agents/wukong/
  .agent.lock               ← 独占锁（每个目录只能运行一个进程）
  .agent.heartbeat          ← 心跳（存活证明）
  .agent.json               ← 清单
  system/
    covenant.md             ← 盟约（跨凝蜕存续）
    memory.md               ← 记忆
  mailbox/
    inbox/                  ← 收到的信件
    outbox/                 ← 待发送
    sent/                   ← 已发送记录
  logs/
    events.jsonl            ← 结构化事件日志
```

没有 `agent_id`。路径即身份。智能体通过写入彼此的 `mailbox/inbox/` 通信——如同在邻居门口投信。

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

## 了解更多

设计哲学、架构解析、开发笔记，尽在 **[lingtai.ai](https://lingtai.ai)**。

## 许可

MIT — [Zesen Huang](https://github.com/huangzesen), 2025–2026

<div align="center">

[lingtai.ai](https://lingtai.ai) · [lingtai-kernel](https://github.com/huangzesen/lingtai-kernel) · [PyPI](https://pypi.org/project/lingtai/)

</div>
