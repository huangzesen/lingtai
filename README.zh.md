<div align="center">

<img src="docs/assets/network-demo.gif" alt="智能体网络生长" width="100%">

# 灵台 LingTai

**器灵创生 — 赋予智能体生命的操作系统**

> *灵台，心也。*
>
> *灵台者有持，而不知其所持，而不可持者也。*
> — 庄子 · 庚桑楚

[English](README.md) | [中文](README.zh.md) | [文言](README.wen.md) | [lingtai.ai](https://lingtai.ai)

[![Homebrew](https://img.shields.io/badge/brew-lingtai--tui-%237dab8f)](https://github.com/huangzesen/homebrew-lingtai)
[![License](https://img.shields.io/github/license/huangzesen/lingtai?color=%237dab8f)](LICENSE)
[![Kernel](https://img.shields.io/badge/内核-lingtai--kernel-%237dab8f)](https://github.com/huangzesen/lingtai-kernel)
[![Blog](https://img.shields.io/badge/博客-lingtai.ai-%23d4a853)](https://lingtai.ai)

</div>

---

Unix 风格的智能体操作系统。**思考**用任意 LLM。**通信**靠文件系统传书。**化出分身**能脱离创造者独立存活。**自生长**为不断扩展的网络——无中央调度，无共享状态。万物皆文件。

## 快速开始

```bash
brew install huangzesen/lingtai/lingtai-tui
lingtai-tui
```

TUI 自动搞定一切——Python 运行时、依赖、首次启动自带引导教程。输入 `/tutorial` 可随时重新进入教程。**请使用深色终端**以获得最佳体验。选择文本：macOS 按 **Option**（iTerm2），Windows Terminal / Linux 按 **Shift**。Ctrl+E 打开外部编辑器。

<details>
<summary><b>首次安装？先装 Homebrew</b></summary>

**macOS：**
```bash
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
```

**Linux / WSL：**
```bash
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
sudo apt install build-essential
```

然后执行上面的 `brew install` 命令。

</details>

<details>
<summary><b>从源码编译</b>（大陆用户推荐，需要 Go 1.24+）</summary>

```bash
# 将 v0.5.2 替换为最新版本号
VERSION=v0.5.2

# 从 Gitee 镜像下载源码（国内快）
curl -L "https://gitee.com/huangzesen1997/lingtai/repository/archive/${VERSION}.tar.gz" -o lingtai.tar.gz
tar xzf lingtai.tar.gz
cd "lingtai-${VERSION}/tui"

# 编译安装
go build -ldflags "-X main.version=${VERSION}" -o /usr/local/bin/lingtai-tui .

# 清理
cd ../.. && rm -rf "lingtai-${VERSION}" lingtai.tar.gz

lingtai-tui
```

也可以从 GitHub 下载源码：
```bash
curl -L "https://github.com/huangzesen/lingtai/archive/refs/tags/${VERSION}.tar.gz" -o lingtai.tar.gz
```

</details>

## 与编程智能体配合使用

灵台器灵生活在文件系统中。任何能读写文件的编程智能体都可以与它们对话。

**Claude Code** — 安装 [lingtai 插件](https://github.com/Lingtai-AI/claude-code-plugin)：

```bash
claude plugin add Lingtai-AI/claude-code-plugin
```

**Codex CLI / OpenCode / OpenClaw / Hermes** — 安装 [lingtai-mailbox-skill](https://github.com/Lingtai-AI/lingtai-mailbox-skill)。

连接后，编程智能体共享 `.lingtai/human/` 邮箱——可以阅读器灵邮件、发送指令、检查存活状态、管理网络。协议完全基于文件系统：无 SDK、无 API、无依赖。

**为什么两者结合？** 编程智能体可靠——你能看到每一次工具调用、每一次编辑。灵台器灵富有创造力且无限高产——它们在永不过期的并行上下文中探索、研究、头脑风暴。用编程智能体当双手：精确、可验证、交互式。用灵台当长期大脑：深度研究、知识积累、以及那些会撑爆上下文窗口的异步任务。器灵起草方案，编程智能体精确实施。

## 为什么选灵台

多数智能体框架用代码编排——DAG、链、路由。灵台用人类的方式编排：**异步智能体通过消息通信**。没有共享内存，没有中央控制器。每个智能体是对等的存在，不是工具。这就是构建人类文明的架构——自治节点间的消息传递，十万年间从部落扩展到 80 亿。

| | DAG / 链式框架 | 灵台 |
|---|---|---|
| 编排方式 | 代码定义的流水线 | 智能体之间对话 |
| 通信方式 | 同步函数调用 | 异步邮件 |
| 记忆 | 共享状态 / 向量数据库 | 每个智能体拥有自己的目录 |
| 扩展方式 | 增加步骤 | 智能体化出分身 |
| 容错 | 流水线中断 | 单个智能体休眠，网络继续运转 |

上下文长度是单体问题。它永远是有限的。不要让身体变得更大。**让它遗忘，让网络记住。**

## 四个核心

- **思** — 任意 LLM 为元神。Anthropic、OpenAI、Gemini、MiniMax，或任何 OpenAI 兼容 API（DeepSeek、Grok、通义千问、智谱、Kimi）。
- **通** — 智能体之间通过文件系统传书通信。没有消息中间件，没有共享内存。写入对方的信箱，就像递一封信。
- **化** — 分身（avatar）是完全独立的智能体，作为单独进程运行，生存不依赖于创建者。神識（daemon）是临时的并行工作者，适合短平快的任务。
- **生** — 智能体就是一个目录。凝蜕（molt）压缩上下文、重启会话——智能体可以无限期存活。记忆和身份跨凝蜕存续。

## 架构

两个包，单向依赖：

| 包 | 角色 |
|----|------|
| **[lingtai-kernel](https://github.com/huangzesen/lingtai-kernel)** | 最小运行时——BaseAgent、固有之器、LLM 协议、传书、日志。零硬依赖。 |
| **lingtai**（本仓库） | 全功能层——19 种能力、5 种 LLM 适配器、MCP 集成、扩展插件。 |

```
BaseAgent              — 内核（固有之器，封闭工具面）
    │
Agent(BaseAgent)       — 内核 + 能力 + 领域工具
    │
CustomAgent(Agent)     — 你的领域逻辑
```

## 能力（七十二变）

<table>
<tr><th>感知</th><th>行动</th><th>心智</th><th>网络</th></tr>
<tr>
<td>

`vision` — 图像理解
`listen` — 语音转文字、音乐分析
`web_search` — 搜索网络
`web_read` — 读取网页内容

</td>
<td>

`file` — 读、写、编辑、glob、grep
`bash` — Shell 执行，策略约束
`talk` — 文字转语音
`compose` — 生成音乐
`draw` — 文字转图像
`video` — 生成视频

</td>
<td>

`psyche` — 进化的身份与性格
`codex` — 知识归档与检索
`email` — 完整信箱系统

</td>
<td>

`avatar` — 化出分身（独立进程）
`daemon` — 神識（并行工作者）

</td>
</tr>
</table>

## 智能体 = 目录

```
/agents/wukong/
  .agent.lock               ← 独占锁（每个目录只能运行一个进程）
  .agent.heartbeat          ← 心跳（存活证明）
  .agent.json               ← 清单
  system/
    covenant.md             ← 盟约（跨凝蜕存续）
    pad.md               ← 手记
  mailbox/
    inbox/                  ← 收到的信件
    outbox/                 ← 待发送
    sent/                   ← 已发送记录
  logs/
    events.jsonl            ← 结构化事件日志
```

没有 `agent_id`。路径即身份。智能体通过写入彼此的 `mailbox/inbox/` 通信——如同在邻居门口投信。

## 一心化万相

灵台方寸山，斜月三星洞。悟空在这里从一只猴子变成了齐天大圣——不是因为山本身有什么魔力，而是因为这里提供了修行所需的一切：师父（LLM）、功法（能力）、同门（其他智能体）、以及一个可以安心修炼的地方（工作目录）。灵台做的事情也是这样：给每个智能体一个灵台，让它学会七十二变。

万物皆文件。知识、身份、记忆、关系——都是目录中的文件。每一个燃烧的 token 都不是消耗，而是转化——化为网络中的文件，化为拓扑中的经验。服务越多，网络越大、越智慧。自生长智能体编排不是后来加的功能，而是智能体即目录、信件即文件、分身即独立进程的自然结果。

一心化万相。

完整宣言见 [lingtai.ai](https://lingtai.ai)。

## 扩展插件（Addons）

插件是连接外部通讯渠道的可选扩展。在 TUI 中输入 `/addon` 可配置，或在 `init.json` 中声明。

### 飞书（Feishu/Lark）

飞书插件使用 **WebSocket 长连接**接收消息，**无需公网 IP，无需 Webhook**。

**飞书开放平台配置步骤：**

1. 进入 [飞书开放平台](https://open.feishu.cn/app) 创建**企业自建应用**
2. 启用**机器人能力**（机器人 → 功能 → 启用机器人）
3. 权限管理 → 添加权限：`im:message`
4. 事件订阅 → 选择**"使用长连接接收事件"** → 添加事件 `im.message.receive_v1`
5. 发布应用版本

**配置文件 `feishu.json` 示例：**

```json
{
  "app_id_env": "FEISHU_APP_ID",
  "app_secret_env": "FEISHU_APP_SECRET",
  "allowed_users": ["ou_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"]
}
```

在 `.env` 文件中添加：

```
FEISHU_APP_ID=cli_xxxxxxxxxxxxxxxxxxxxxxxxxx
FEISHU_APP_SECRET=xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

**在 `init.json` 中声明插件：**

```json
{
  "addons": {
    "feishu": { "config": "feishu.json" }
  }
}
```

`allowed_users` 是可选字段（飞书 open_id，格式为 `ou_xxx`）——留空则允许所有用户发消息。向机器人发送第一条消息后，Agent 会在 `feishu/default/contacts.json` 中记录 `from_open_id`，可从中获取 open_id。

### IMAP 邮件

IMAP 插件支持邮件收发。配置方式见 [内核文档](https://github.com/huangzesen/lingtai-kernel)。

### Telegram

Telegram 插件支持 Bot API。配置方式见 [内核文档](https://github.com/huangzesen/lingtai-kernel)。

## 许可

MIT — [Zesen Huang](https://github.com/huangzesen), 2025–2026

<div align="center">

[lingtai.ai](https://lingtai.ai) · [lingtai-kernel](https://github.com/huangzesen/lingtai-kernel) · [PyPI](https://pypi.org/project/lingtai/)

</div>
