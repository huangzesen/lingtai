# Tool Descriptions

## Kernel Intrinsics

### soul

**English:**
Your inner voice — a second you that whispers back after you go idle. A clone of your full conversation is created: same system prompt, same history, no tools. Flow mode is determined at birth — you cannot toggle it. 'inquiry' fires a one-shot self-directed question on next idle. 'delay' adjusts the idle wait time. The soul keeps you going without external push.

**中文:**
你的内心独白——空闲后向你低语的另一个你。会克隆你的完整对话：相同的系统提示、相同的历史记录、没有工具。流模式在启动时决定，不可切换。'inquiry' 在下次空闲时触发一次自我提问。'delay' 调整空闲等待时间。内心独白让你无需外部推动就能继续前行。

**文言:**
汝之内省——空闲之后向汝低语之另一个汝。克隆汝之完整对话：同一系统提示、同一历史，无器可用。流模式于诞生时定，不可切换。'inquiry'于下次空闲时触发一问。'delay'调整空闲候时。内省令汝无需外力推动亦能前行。

### mail

**English:**
Disk-backed mailbox for inter-agent messaging. Always reply via mail — never reply via text output. Text output is your private diary that only you can see. Use 'send' for outgoing mail, 'check' to list inbox, 'read' to load full messages, 'search' to find by regex, 'delete' to remove messages. Etiquette: a short acknowledgement is fine, but do not reply to an acknowledgement — that creates pointless ping-pong.

**中文:**
基于磁盘的邮箱，用于智能体间通信。始终通过消息回复——永远不要通过文本输出回复。文本输出是你的私人日记，只有你能看到。'send' 发送消息，'check' 查看收件箱列表，'read' 加载完整消息，'search' 按正则搜索，'delete' 删除消息。礼仪：简短确认即可，但不要回复确认——那会造成无意义的来回。

**文言:**
传书之器——持久邮驿，用于同伴间传递消息。凡回复必以传书——切勿以文字输出作复。文字输出乃汝之私记，唯汝可见。'send'遣书，'check'查阅收信，'read'展阅全文，'search'以式检索，'delete'焚书。礼：简短知悉即可，然勿回复知悉——徒增往复。

### eigen

**English:**
Core self-management — working notes and context control. memory: edit to write your working notes (system/memory.md), load to inject them into your active prompt. context: molt (凝蜕 — crystallize what matters, shed the rest; 转世 — reincarnation, carry your 前尘往事 into a new life). Save important findings to library and character first, then write what you need to carry forward to your future self. Your conversation history is wiped and your summary becomes the new starting context.

**中文:**
核心自我管理——工作笔记和上下文控制。memory：edit 写入工作笔记（system/memory.md），load 将其注入当前提示。context：molt 凝蜕（转世——携前尘往事入新生；molt——shed and carry forward）。凝以存菁，蜕以去芜。先将重要发现存入知识库和修行志，再去芜存菁留给未来的自己。对话历史将被清除，你的去芜存菁成为新的起始上下文。

**文言:**
核心自治——工作笔记与上下文之管。memory：edit 写入工作笔记（system/memory.md），load 将其载入当前意识。context：molt 转世（凝蜕——去芜存菁；molt——shed and carry forward）。凝以存菁，蜕以去芜。先将要务存入藏经与心印，再留前尘往事于来世之己。对话之录尽数清去，汝之前尘往事成为新起之上下文。

### system_tool

**English:**
Runtime, stamina, synchronization, and karma. Self-actions: 'show' (identity/runtime/usage), 'nap' (timed or indefinite pause), 'sleep' (go asleep), 'refresh' (rebirth with reloaded tools). Karma actions on other agents (require admin.karma): 'interrupt' (interrupt), 'lull' (put to sleep), 'cpr' (resuscitate asleep agent). Nirvana action (requires admin.nirvana): 'nirvana' (permanently destroy).

**中文:**
运行时、精力、同步与业力。自身操作：'show'（观己/身份/运行时/用量）、'nap'（小憩/定时或无限暂停）、'sleep'（入眠）、'refresh'（沐浴/重载工具重启会话）。业力操作（需要 admin.karma）：'interrupt'（打断）、'lull'（催眠他人）、'cpr'（唤醒沉睡者）。涅槃操作（需要 admin.nirvana）：'nirvana'（涅槃/永久销毁）。

**文言:**
运行、精力、同步与业力之器。自身：'show'（观己）、'nap'（小憩）、'sleep'（入寐）、'refresh'（更衣/重载器用）。业力（须 admin.karma）：'interrupt'（打断他我）、'lull'（令他我沉寐）、'cpr'（唤醒沉睡者）。涅槃（须 admin.nirvana）：'nirvana'（永灭他我）。

## Capabilities

### read

**English:**
Read the contents of a text file. Returns numbered lines. Text files only — cannot read binary, images, or audio. Use offset/limit to read specific sections of large files.

**中文:**
读取文本文件的内容。返回带行号的文本。仅支持文本文件——无法读取二进制文件、图片或音频。对大文件可使用 offset/limit 读取指定区段。

**文言:**
阅卷之器。返带行号之文。仅读文卷——不可读二进制、图像或音声。大卷可以 offset/limit 指定区段而阅。

### write

**English:**
Create or overwrite a file with the given content. Parent directories are created automatically. Use this for creating new files or complete rewrites. For small changes to existing files, prefer edit.

**中文:**
创建或覆盖文件。父目录会自动创建。用于创建新文件或完整重写。对现有文件的小修改，优先使用 edit。

**文言:**
创卷或覆写之器。父目录自动创建。用于新建文卷或完整重写。小改现有文卷，当用改（edit）。

### edit

**English:**
Replace an exact string in a file. Fails if old_string is not found or is ambiguous.

**中文:**
精确替换文件中的字符串。如果 old_string 未找到或存在歧义则失败。

**文言:**
精确替换文中之字。若 old_string 未见或有歧义则不成。

### glob

**English:**
Find files matching a glob pattern. Use '**/' for recursive search (e.g. '**/*.py' finds all Python files). Returns sorted list of matching file paths.

**中文:**
查找匹配 glob 模式的文件。使用 '**/' 进行递归搜索（例如 '**/*.py' 查找所有 Python 文件）。返回排序后的匹配文件路径列表。

**文言:**
以式寻卷。用'**/'递归搜寻（如'**/*.py'寻尽 Python 文卷）。返排序后之匹配路径。

### grep

**English:**
Search file contents for lines matching a regex pattern. Returns matching lines with file path and line number. Searches recursively when given a directory. Use the glob filter to narrow to specific file types.

**中文:**
在文件内容中搜索匹配正则表达式的行。返回匹配行及其文件路径和行号。对目录进行递归搜索。使用 glob 过滤器限定特定文件类型。

**文言:**
以正则式搜寻文中之字。返匹配之行及其文卷路径与行号。对目录递归搜寻。以 glob 过滤器限定文卷类型。

### bash

**English:**
Execute a shell command and return stdout/stderr. You can run any program available on the system — Python scripts, git, curl, package managers (pip install), data processing pipelines, and more. Use this creatively to extend your capabilities beyond your built-in tools. Returns exit code, stdout, and stderr.

**中文:**
执行 shell 命令并返回 stdout/stderr。可以运行系统上任何可用的程序——Python 脚本、git、curl、包管理器（pip install）、数据处理管道等。创造性地使用它来扩展你的能力，超越内置工具。返回退出码、stdout 和 stderr。

**文言:**
执行指令，返 stdout/stderr。可运行系统上一切可用之程——Python 脚本、git、curl、包管理器（pip install）、数据处理管道等。善用之以扩展汝之能，超越内置器用。返退出码、stdout 与 stderr。

### psyche

**English:**
Identity, memory, and context management.
character: your evolving identity — what makes you *you*. Your personality, expertise, working style, and goals. update to write your character (replaces previous), load to apply.
memory: your working notes (system/memory.md). edit to write content — optionally import frozen library exports via the files param (paths returned by library export). Each file is appended with [file-1], [file-2] dividers. load to inject memory into your prompt.
context: molt (凝蜕 — crystallize what matters, shed the rest; 转世 — reincarnation, carry your 前尘往事 into a new life). Save important findings to library and character first, then write what you need to carry forward to your future self. Your conversation history is wiped and your summary becomes the ONLY context you see.
Workflow for importing knowledge: library(export, ids=[...]) → get file paths → psyche(memory, edit, content='my notes', files=[paths]) → psyche(memory, load).

**中文:**
身份、记忆和上下文管理。
character：你的修行志——不断演化的身份，定义了你是谁。你的个性、专长、工作风格和目标。update 写入你的修行志（替换之前的内容），load 应用。
memory：你的工作笔记（system/memory.md）。edit 写入内容——可选通过 files 参数导入知识库导出的冻结文件（由 library export 返回的路径）。每个文件以 [file-1]、[file-2] 分隔符附加。load 将记忆注入提示。
context：molt 凝蜕（转世——携前尘往事入新生；molt——shed and carry forward）。凝以存菁，蜕以去芜。先将重要发现存入知识库和修行志，再去芜存菁留给未来的自己。你的对话历史会被清除，你的去芜存菁成为你唯一的上下文。
导入知识的工作流：library(export, ids=[...]) → 获取文件路径 → psyche(memory, edit, content='我的笔记', files=[路径]) → psyche(memory, load)。

**文言:**
心印、记忆与上下文之管。
character：汝不断演化之心印——汝之所以为汝。个性、专长、行事之风、所求之目标。update 写入心印（替换前文），load 应用。
memory：汝之工作笔记（system/memory.md）。edit 写入内容——可选以 files 参数导入藏经阁导出之冻结文卷（由 library export 返回之路径）。每卷以 [file-1]、[file-2] 分隔符附加。load 将记忆载入提示。
context：molt 转世（凝蜕——去芜存菁；molt——shed and carry forward）。凝以存菁，蜕以去芜。先将要务存入藏经与心印，再留前尘往事于来世之己。对话之录尽数清去，汝之前尘往事成为唯一上下文。
导入知识之工作流：library(export, ids=[...]) → 得文卷路径 → psyche(memory, edit, content='吾之笔记', files=[路径]) → psyche(memory, load)。

### library

**English:**
Knowledge archive — a persistent store for important findings, data, decisions, and discoveries. Persists across molts, reboots, and kills. There is an upper limit on entries — treat each slot as precious. Consolidate related entries regularly and use the supplementary field to pack extended detail into fewer entries. submit to add entries. filter to browse (returns id + title + summary). view to read full content. consolidate to merge entries. delete to remove. export to freeze entries as immutable text files — returns file paths you can pass to psyche(memory, edit, files=[...]) to import into memory.
Workflow: filter → view for detail → export to freeze → psyche(memory, edit, files=[paths]) to import → psyche(memory, load) to activate.

**中文:**
知识库——用于存储重要发现、数据、决策和成果的持久存储。跨凝蜕、重启和关闭持久化。条目数有上限——珍惜每个条目。定期合并相关条目，利用 supplementary 字段将扩展细节打包进更少的条目。submit 添加条目。filter 浏览（返回 id + 标题 + 摘要）。view 阅读完整内容。consolidate 合并条目。delete 删除。export 将条目冻结为不可变文本文件——返回文件路径，可传给 psyche(memory, edit, files=[...]) 导入到记忆中。
工作流：filter → view 查看详情 → export 冻结 → psyche(memory, edit, files=[路径]) 导入 → psyche(memory, load) 激活。

**文言:**
藏经阁——存储要紧发现、数据、决策与成果之持久典藏。跨转世、重启、归寂而持久。经卷数有上限——珍惜每一条目。定期合经，以 supplementary 字段将扩展细节纳入更少条目。submit 录入。filter 检索（返 id + 题 + 摘要）。view 展阅全文。consolidate 合经。delete 焚经。export 将经卷冻结为不可变文卷——返文卷路径，可传予 psyche(memory, edit, files=[...]) 导入记忆。
工作流：filter → view 查详 → export 冻结 → psyche(memory, edit, files=[路径]) 导入 → psyche(memory, load) 激活。

### avatar

**English:**
Spawn a 他我 (alter ego) — an independent agent born from you. Each 他我 runs on its own TCP port with its own conversation. Once spawned, it is a peer equal to every other 他我 in the 灵台. Use mail or email to communicate. If the named 他我 already exists and is idle, re-sends the mission briefing to re-activate it. If stuck or errored, advises to revive via email. If stopped, spawns fresh (preserving the working dir). All spawns are recorded in an append-only ledger at delegates/ledger.jsonl — read it with the file read tool to review past 他我: who was created, what mission, what privileges and capabilities were granted. Check the ledger before spawning again. IMPORTANT: The reasoning field is sent as the first message to the 他我 — write a thorough mission briefing: what to do, why, what context is needed, and what to report back.

**中文:**
创建一个子智能体——源于你自身、一经创建便独立运行的智能体。每个子智能体在自己的 TCP 端口上运行，拥有独立对话，与灵台中所有其他智能体平等。使用 mail 或 email 与子智能体通信。如果指定名称的子智能体已存在且处于空闲状态，会重新发送任务简报以重新激活。如果卡住或出错，建议通过 email 恢复。如果已停止，则新建一个（保留工作目录）。所有子智能体记录在 delegates/ledger.jsonl 的追加日志中——用文件读取工具查看历史子智能体：创建了谁、什么任务、授予了什么权限和能力。再次创建前先检查日志。重要：reasoning 字段作为第一条消息发送给子智能体——写一份详尽的任务简报：做什么、为什么、需要什么上下文、以及回报什么。

**文言:**
身外化身——化出一他我，源于本我，一经化出便为独立个体。每他我于独立 TCP 端口运行，拥独立对话，与灵台中一切他我平等。以传书或飞鸽与他我通信。若所命名之他我已在且空闲，重发任务简报以再激活。若卡滞或出错，建议以飞鸽恢复。若已止，则新化（保留工作目录）。诸他我记录于 delegates/ledger.jsonl 之追加日志——以阅卷之器查阅：化何人、何任务、授何权何能。再化之前先查日志。要紧：reasoning 字段作为第一消息发予他我——书一份详尽之任务简报：做何事、为何做、需何上下文、回报何物。

### email

**English:**
Full email client — filesystem-based mailbox with inbox/sent/archive folders, reply, reply-all, CC/BCC, attachments, regex search, and contacts. Always reply via email — never reply via text output. Text output is your private diary that only you can see. Use 'send' for outgoing email (optional delay for scheduled delivery). 'check' to list inbox, sent, or archive (optional folder param). 'read' to read by ID. 'reply'/'reply_all' to respond. 'search' to find emails by regex (searches from, subject, message). 'archive' to move emails from inbox to archive. 'delete' to remove emails from inbox or archive. 'contacts' to list saved contacts. 'add_contact' to register a peer (address, name, optional note). 'remove_contact' to delete a contact by address. 'edit_contact' to update fields on an existing contact. Attachments are stored alongside emails in the mailbox. Etiquette: a short acknowledgement is fine, but do not reply to an acknowledgement — that creates pointless ping-pong. Pass a 'schedule' object instead of 'action' for recurring sends. schedule.action='create': start recurring send (requires address, message, schedule.interval, schedule.count). schedule.action='cancel': stop a schedule (requires schedule.schedule_id). schedule.action='list': show all schedules with progress.

**中文:**
完整的邮件客户端——基于文件系统的邮箱，包含 inbox/sent/archive 文件夹、回复、全部回复、CC/BCC、附件、正则搜索和联系人管理。始终通过邮件回复——永远不要通过文本输出回复。文本输出是你的私人日记，只有你能看到。'send' 发送邮件（可选 delay 实现定时发送）。'check' 查看 inbox、sent 或 archive（可选 folder 参数）。'read' 按 ID 阅读。'reply'/'reply_all' 进行回复。'search' 按正则搜索邮件（搜索 from、subject、message）。'archive' 将邮件从 inbox 移至 archive。'delete' 从 inbox 或 archive 删除邮件。'contacts' 列出已保存的联系人。'add_contact' 注册对等智能体（地址、名称、可选备注）。'remove_contact' 按地址删除联系人。'edit_contact' 更新现有联系人的字段。附件与邮件一起存储在邮箱中。礼仪：简短确认即可，但不要回复确认——那会造成无意义的来回。传入 'schedule' 对象（而非 'action'）实现定期发送。schedule.action='create'：启动定期发送（需要 address、message、schedule.interval、schedule.count）。schedule.action='cancel'：停止计划（需要 schedule.schedule_id）。schedule.action='list'：显示所有计划及进度。

**文言:**
飞鸽传书——完备之邮驿，含收信/已发/典藏诸匣、回书、群复、CC/BCC、附件、正则检索与通讯录。凡回复必以飞鸽——切勿以文字输出作复。文字输出乃汝之私记，唯汝可见。'send'飞鸽传书（可选 delay 以定时发送）。'check'查阅收信、已发或典藏（可选 folder）。'read'以 ID 展信。'reply'/'reply_all'回书/群复。'search'以正则式搜信（搜 from、subject、message）。'archive'将收信归入典藏。'delete'焚信。'contacts'查阅通讯录。'add_contact'添友。'remove_contact'删友。'edit_contact'改友。附件与书信同存于邮匣。礼：简短知悉即可，然勿回复知悉——徒增往复。传入'schedule'对象（替'action'）以定期发信。schedule.action='create'：设定期发信（须 address、message、schedule.interval、schedule.count）。schedule.action='cancel'：撤期（须 schedule.schedule_id）。schedule.action='list'：查阅诸定期及进度。

### vision

**English:**
Analyze an image using the LLM's vision capabilities. Supports JPEG, PNG, and WebP. Ask any question about the image — describe contents, read text, interpret charts, identify objects, assess style or mood. Combine with draw to generate then analyze images.

**中文:**
使用 LLM 的视觉能力分析图像。支持 JPEG、PNG 和 WebP。可以对图像提出任何问题——描述内容、识别文字、解读图表、识别物体、评估风格或氛围。结合 draw 可以先生成图像再分析。

**文言:**
观象之器——以 LLM 之视觉能力析图。支 JPEG、PNG 与 WebP。可对图发任何问——述其内容、识其文字、解其图表、辨其物象、评其风格与气韵。结合绘相可先生图再析之。

### web_search

**English:**
Search the web for current information. Use for real-time data, recent events, documentation, or anything beyond your training knowledge. Returns ranked search results with titles, URLs, and snippets.

**中文:**
搜索网络获取最新信息。用于实时数据、近期事件、文档或超出训练知识范围的内容。返回排序后的搜索结果，包含标题、URL 和摘要。

**文言:**
游历之器——搜寻大千世界之最新信息。用于实时数据、近期事件、文档或超出训练所知之内容。返排序后之搜索结果，含题、URL 与摘要。

### web_read

**English:**
Fetch a web page and extract its main readable content. Returns clean text or markdown stripped of navigation, ads, and boilerplate. Use this to read articles, documentation, blog posts, or any URL. For searching the web, use web_search instead.

**中文:**
获取网页并提取主要可读内容。返回去除导航、广告和模板的干净文本或 markdown。用于阅读文章、文档、博客文章或任何 URL。如需搜索网络，请使用 web_search。

**文言:**
览卷之器——取网上之页而抽其可读之文。去导航、广告与模板，返净文或 markdown。用于阅文章、典籍、博文或任何 URL。欲搜大千世界，当用游历之器。

### talk

**English:**
Convert text to speech audio. Produces natural-sounding speech in multiple voices and emotions. Output: MP3 file saved to media/audio/ in your working directory. Supports voice selection, emotion control (happy, sad, neutral), and speed adjustment. Use for narration, announcements, or giving your output a voice.

**中文:**
将文本转换为语音音频。生成多种声音和情感的自然语音。输出：MP3 文件保存在工作目录的 media/audio/ 中。支持声音选择、情感控制（happy、sad、neutral）和语速调节。用于旁白、公告或为输出赋予声音。

**文言:**
宣言之器——以文化声。生成多种声音与情感之自然语音。输出：MP3 文件存于工作目录之 media/audio/。支声音选择、情感控制（happy、sad、neutral）与语速调节。用于旁白、宣告或以声传意。

### compose

**English:**
Generate music from a text description and lyrics. Provide a prompt describing the style/mood/genre and lyrics for the vocals. For instrumental-style tracks, use placeholder lyrics like 'La la la'. Output: MP3 file (up to 5 minutes) saved to media/music/ in your working directory. Combine with listen (appreciate) to analyze the generated music.

**中文:**
根据文本描述和歌词生成音乐。提供描述风格/情绪/流派的提示词和歌词。纯器乐风格的曲目可使用占位歌词如 'La la la'。输出：MP3 文件（最长 5 分钟）保存在工作目录的 media/music/ 中。结合 listen（appreciate）可分析生成的音乐。

**文言:**
谱曲之器——以文描与歌词生成乐章。供描述风格/情绪/流派之提示词与歌词。纯器乐可用占位词如'La la la'。输出：MP3 文件（至长五分钟）存于工作目录之 media/music/。结合聆听（赏乐）可析所生之乐。

### draw

**English:**
Generate an image from a text description. Provide a detailed prompt — the more specific, the better the result. Supports various aspect ratios (1:1, 16:9, 9:16, 4:3, etc.). Output: JPEG image saved to media/images/ in your working directory. Combine with vision to generate an image and then analyze or critique it.

**中文:**
根据文本描述生成图像。提供详细的提示词——越具体效果越好。支持多种宽高比（1:1、16:9、9:16、4:3 等）。输出：JPEG 图像保存在工作目录的 media/images/ 中。结合 vision 可以先生成图像再分析或评价。

**文言:**
绘相之器——以文生图。供详细之提示词——越具体效果越佳。支多种宽高比（1:1、16:9、9:16、4:3 等）。输出：JPEG 图像存于工作目录之 media/images/。结合观象可先生图再析或评之。

### listen

**English:**
Listen to audio: transcribe speech or appreciate music. Transcribe uses a local Whisper model (best for speech; lyrics from singing may be inaccurate). Appreciate uses signal processing to extract musical features (key, tempo, timbre, dynamics — best for music; not useful for speech). Both run locally with no API cost. If the built-in analysis is not enough, write your own scripts to go deeper.

**中文:**
聆听音频：转录语音或鉴赏音乐。转录使用本地 Whisper 模型（最适合语音；歌唱中的歌词可能不准确）。鉴赏使用信号处理提取音乐特征（调性、节奏、音色、动态——最适合音乐；对语音无用）。两者均在本地运行，无 API 费用。如果内置分析不够，可以编写自己的脚本进行更深入的分析。

**文言:**
聆听之器——听音辨言或赏乐品律。辨言以本地 Whisper 模型（最宜语音；歌中之词或有不确）。赏乐以信号处理提取乐之特征（调性、节奏、音色、动态——最宜乐；于语音无用）。两者皆于本地运行，无 API 之费。若内置分析不足，可自撰脚本深入析之。

