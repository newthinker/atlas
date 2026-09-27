# Deployment Guide

This guide covers deploying ATLAS in various environments.

## Table of Contents

- [Local Development](#local-development)
- [Docker Deployment](#docker-deployment)
- [Production Deployment](#production-deployment)
- [Environment Variables](#environment-variables)
- [Database Setup](#database-setup)
- [Reverse Proxy](#reverse-proxy)

---

## Local Development

### Prerequisites

- Go 1.21+
- Git

### Setup

```bash
# Clone repository
git clone https://github.com/newthinker/atlas.git
cd atlas

# Install dependencies
go mod download

# Build
go build -o bin/atlas ./cmd/atlas

# Create configuration
cp configs/config.example.yaml config.yaml

# Run
./bin/atlas serve -c config.yaml --debug
```

### Development with Hot Reload

Use [air](https://github.com/cosmtrek/air) for automatic reloading:

```bash
# Install air
go install github.com/cosmtrek/air@latest

# Create .air.toml
cat > .air.toml << 'EOF'
[build]
cmd = "go build -o ./tmp/atlas ./cmd/atlas"
bin = "./tmp/atlas serve -c config.yaml --debug"
include_ext = ["go", "yaml", "html"]
exclude_dir = ["tmp", "vendor", ".git", ".worktrees"]
EOF

# Run with hot reload
air
```

---

## Docker Deployment

### Dockerfile

Create `Dockerfile`:

```dockerfile
# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o atlas ./cmd/atlas

# Runtime stage
FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app
COPY --from=builder /app/atlas .
COPY --from=builder /app/internal/api/templates ./internal/api/templates

EXPOSE 8090

ENTRYPOINT ["./atlas"]
CMD ["serve", "-c", "/config/config.yaml"]
```

### Docker Compose

Create `docker-compose.yml`:

```yaml
version: '3.8'

services:
  atlas:
    build: .
    ports:
      - "8090:8090"
    volumes:
      - ./config.yaml:/config/config.yaml:ro
      - atlas-data:/data
    environment:
      - TELEGRAM_BOT_TOKEN=${TELEGRAM_BOT_TOKEN}
      - TELEGRAM_CHAT_ID=${TELEGRAM_CHAT_ID}
      - ANTHROPIC_API_KEY=${ANTHROPIC_API_KEY}
    restart: unless-stopped

  # Optional: TimescaleDB for production storage
  timescaledb:
    image: timescale/timescaledb:latest-pg15
    ports:
      - "5432:5432"
    environment:
      - POSTGRES_USER=atlas
      - POSTGRES_PASSWORD=${DB_PASSWORD}
      - POSTGRES_DB=atlas
    volumes:
      - timescale-data:/var/lib/postgresql/data
    restart: unless-stopped

volumes:
  atlas-data:
  timescale-data:
```

### Build and Run

```bash
# Build image
docker build -t atlas:latest .

# Run with docker-compose
docker-compose up -d

# View logs
docker-compose logs -f atlas

# Stop
docker-compose down
```

---

## Production Deployment

### System Requirements

| Component | Minimum | Recommended |
|-----------|---------|-------------|
| CPU | 1 core | 2+ cores |
| Memory | 512 MB | 2 GB |
| Disk | 1 GB | 10 GB (with history) |

### Systemd Service

Create `/etc/systemd/system/atlas.service`:

```ini
[Unit]
Description=ATLAS Trading Signal System
After=network.target

[Service]
Type=simple
User=atlas
Group=atlas
WorkingDirectory=/opt/atlas
ExecStart=/opt/atlas/bin/atlas serve -c /opt/atlas/config.yaml
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal

# Security hardening
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=/opt/atlas/data

# Environment
EnvironmentFile=/opt/atlas/.env

[Install]
WantedBy=multi-user.target
```

### Installation Steps

```bash
# Create user
sudo useradd -r -s /bin/false atlas

# Create directories
sudo mkdir -p /opt/atlas/{bin,data}

# Copy binary and config
sudo cp bin/atlas /opt/atlas/bin/
sudo cp config.yaml /opt/atlas/
sudo cp -r internal/api/templates /opt/atlas/

# Create environment file
sudo cat > /opt/atlas/.env << 'EOF'
TELEGRAM_BOT_TOKEN=your_token
TELEGRAM_CHAT_ID=your_chat_id
ANTHROPIC_API_KEY=your_api_key
EOF

# Set permissions
sudo chown -R atlas:atlas /opt/atlas
sudo chmod 600 /opt/atlas/.env

# Enable and start service
sudo systemctl daemon-reload
sudo systemctl enable atlas
sudo systemctl start atlas

# Check status
sudo systemctl status atlas
sudo journalctl -u atlas -f
```

---

## macOS launchd Deployment（本机生产实况）

生产实例以用户级 LaunchAgent 运行在 runtime 目录（默认 `/Users/zuowei/workspace/runtime/atlas`，二进制与配置由代码目录投递，数据本地持有）。

### 部署流程

```bash
# 1. 构建并同步运行时产物（幂等；data/ logs/ 等本地数据受 --delete 保护）
bash scripts/ops/deploy.sh            # ATLAS_RUNTIME=<path> 可覆盖目标

# 2. 安装/重载全部 LaunchAgent（幂等：bootout → bootstrap）
bash scripts/ops/install-services.sh

# 3. 仅更新二进制后重启常驻服务
launchctl kickstart -k gui/$(id -u)/com.newthinker.atlas.serve
```

### 🔴 手工替换二进制：必须 `rm` + `cp`，不能原地 `cp` 覆盖（2026-09-19 实测事故）

只想换 `bin/atlas`、不跑 `deploy.sh` 时（例如 sprint M4-crisis 的迁移那种「先迁移后部署」场景）：

```bash
# ✅ 正确：先删再拷，换掉 inode
rm -f  "$ATLAS_RUNTIME/bin/atlas"
cp -p  <新二进制> "$ATLAS_RUNTIME/bin/atlas"

# ❌ 错误：原地覆盖，inode 不变
cp -p  <新二进制> "$ATLAS_RUNTIME/bin/atlas"
```

**实测症状**：原地 `cp` 覆盖后，`bin/atlas` 的**任何**子命令都以 `rc=137`（`128+9` = **SIGKILL**）退出，
**stdout 与 stderr 全空**，不产生任何日志。

**成因**：`cp` 原地覆盖保持 inode 不变，而 macOS 内核对该 inode 缓存了**旧二进制的代码签名**；
新内容与缓存不符 ⇒ 内核直接 kill 进程。`rm` 释放旧 inode、`cp` 分配新 inode 即解决
（实测 inode `216433589` → `220870847`）。

⚠️ **这是本仓库记录过的最难归因的部署失效形态**，理由是它与所有常规核实项都不冲突：

| 核实项 | 事故当时 |
| --- | --- |
| `sha256` 与源二进制一致 | ✅ 通过 |
| `codesign -dv` 与源二进制逐字段相同（adhoc / linker-signed） | ✅ 通过 |
| 可执行位 `-rwxr-xr-x` | ✅ 通过 |
| **实际运行** | 🔴 `rc=137`，无输出 |

⇒ **`sha256` + 签名 + 可执行位全对，不足以证明二进制能运行。**
若只核实这三项就宣布部署完成，第一个撞上的会是**无人值守的定时任务**
（本次是 `crisis-daily` 22:45），而排查者看到的是「二进制一切正确、一跑就死、没有任何日志」。

### 部署后必须实际运行一次（不只核实文件属性）

```bash
cd "$ATLAS_RUNTIME"          # ⚠️ 必须在此目录：configs/*.yaml 的 storage.path 是**相对路径**
./bin/atlas crisis status --config configs/config.yaml --crisis-config configs/crisis-monitor.yaml
```

- 🔴 **判据是首行为 `system state: …`，不是 `rc=0`。** `rc=0` 不足以区分「守卫在生产库上通过」
  与「守卫在一个刚被自己建出来的空库上空转」——见下一条。`rc=137` 见上一节。
- 🔴 **忘了 `cd` 不报错，它是假成功。** `crisis.NewStore` 对 `storage.path` 先
  `os.MkdirAll` 再让 sqlite 建库，而 `verifyBitemporalShape` 对全新空库**刻意放行**
  （`internal/crisis/store.go`：表不存在 ⇒ `return nil`，否则谁都建不了新库）。
  ⇒ 在错误目录下会**当场新建一个空 `data/crisis.db`**，`rc=0`、有输出、守卫「通过」
  ——而验的是那个空库，不是生产库。

  实测（2026-09-19，临时目录，未碰生产）：

  | 场景 | rc | 首行 | 副作用 |
  | --- | --- | --- | --- |
  | 在 `$ATLAS_RUNTIME` 下（正确） | 0 | `system state: NORMAL (as of …, 44 eval days)` | 无 |
  | 错目录 + **绝对**配置路径 | **0** | `no evaluations yet — …` | **新建 24576 字节空库，主键正确** |
  | 错目录 + 相对配置路径 | 1 | — | `Error: reading crisis config: open configs/crisis-monitor.yaml: …` |

  ⇒ 第三行那个 `rc=1` 的成因是**配置文件路径**、不是 `storage.path`：`LoadConfig` 先失败，
  根本走不到 `storage.path`。**换成绝对配置路径就只剩第二行那条假成功路径。**
  ⚠️ `system state:` 这一行在全新空库上**构造上不可能出现**（空库恒为 `no evaluations yet — …`），
  所以它是这里唯一可用的判据。**判性质，不判退出码。**
- **为什么用 `crisis status` 而不是重启 `serve`**：`crisis.NewStore`（含双时态形状守卫
  `verifyBitemporalShape`）的唯一非测试调用点是 `openCrisisStore`，**只服务 `atlas crisis *` 命令族**，
  `serve.go` 无引用。⇒ **`serve` 起来了不构成守卫通过的证据。**
  ⚠️ 但上一条说明：**「跑了 `crisis status` 且 rc=0」同样不构成证据**。本小节最初写的判据
  （「rc 必须为 0 且有实际输出」）恰恰无法区分「真通过」与「在空库上空转」——
  **它是它自己要防的那个失效的同形版本**。这一处由 code-simplifier 的受控实验查出并订正。

### 🔴 仓库里的 `bin/atlas` 可能是陈旧构建，不含当前分支的代码

`bin/atlas` 在 `.gitignore` 里，**它不随 `git checkout` 变化**。你切到一个新分支、甚至刚提交完新代码，那个文件仍然是上一次 `go build` 或 `deploy.sh` 留下的产物。

⚠️ **它的危害形态不是「不告诉你」，是「把你推向相反的结论」。**

2026-09-19 实撞一次：用 9/18 构建的 `bin/atlas` 复核一条「在源码目录误跑 `crisis replay` 会读到旧库、输出完全正常、不报错」的说法，拿到 `rc=0` + `no evaluations yet` —— 一个看起来毫无问题的输出，**于是那条说法被判成「成立」**。而真相相反：当前分支的二进制含 `verifyBitemporalShape` 守卫，同一条命令会 **`rc=1` 响亮拦住**并指路 `migrate-bitemporal`。

⇒ 差别不是「少验了一项」，是**结论的方向反了**。如果没人去查二进制版本，那个假结果会作为证据进报告。

**验二进制身份的自检命令**（正对照 + 负对照一起看）：

```bash
# 挑一个只存在于当前分支的符号，再挑一个任何版本都有的做正对照
for s in verifyBitemporalShape migrate-bitemporal crisis; do
  printf '%-26s %s\n' "$s" "$(strings "$ATLAS_RUNTIME/bin/atlas" | grep -c -- "$s")"
done
# 2026-09-19 实测三份二进制：
#   含守卫的新构建        1 / 2 / 355（线上那个）、1 / 2 / 360（当日更晚的构建）
#   9/18 的旧构建         0 / 0 / 318
```

⚠️ **正对照那一列不要拿具体数字当判据** —— 上面两个新构建的 `crisis` 计数就不同（355 vs 360），它随分支上的字符串增减而变。**判据是「守卫符号 ≥1 且正对照 ≫0」这个性质**，不是任何一个具体值。

🔴 **正对照那一行不可省**：只看守卫符号命中 0，无法区分「二进制是旧的」与「`strings`/`grep` 根本没在工作」（路径写错、文件不可读、`grep` 模式打错都会给 0）。`crisis` 那个数非 0，才说明工具链在工作、那个 0 是真的 0。

**要验当前分支的行为，就自己构建到别处，不要覆盖 `bin/atlas`**：

```bash
GOTOOLCHAIN=local go build -o /tmp/atlas-check ./cmd/atlas
```

（覆盖 `bin/atlas` 虽然不影响 `git status`，但它是线上正在用的那一个；而且覆盖前后它都"看起来正常"。）

### 🔴 运维手册的 `sqlite3` CLI 与应用的驱动是**语义不同的两个引擎**

手册里所有 `sqlite3 …` 命令跑的是系统 CLI；应用跑的是 `modernc.org/sqlite`。两者对
`ALTER TABLE … RENAME` 之后的**视图定义**处置相反：

| | `legacy_alter_table` | `RENAME` 之后视图定义 |
| --- | --- | --- |
| `/usr/bin/sqlite3` 3.51.0（手册 / 你手边） | **1** | **不改写**，仍指向旧名 |
| `modernc.org/sqlite`（应用） | **0** | **被静默改写**成新名 |

2026-09-19 实测（同一串 DDL：建表 `t` → 建视图 `v AS SELECT * FROM t` → `RENAME t TO t_v1`）：

```
CLI    ⇒ CREATE VIEW v AS SELECT * FROM t
驱动   ⇒ CREATE VIEW v AS SELECT * FROM "t_v1"     ← 视图跟着改了
```

**这直接决定哪些结论能用 CLI 验**：

| 要验的 | CLI 可信？ |
| --- | --- |
| 视图**存在性**（`COUNT(*) FROM sqlite_master WHERE type='view'`） | ✅ 两边一致 |
| 表/行数、主键形状、约束是否触发 | ✅ 两边一致 |
| 视图**指向哪张表**（定义文本里是原名还是新名） | 🔴 **两边结论相反，必须走 Go 驱动** |

⚠️ 这条实撞过一次：复核 `migrate.go` 里「`RENAME` 会静默改写视图定义」那条注释时，先用 CLI 测，得到「视图未被改写」，**已经准备报该注释为假**；换成驱动重测，结论相反 ⇒ **注释是对的**，`migrate.go` 里那步 `DROP VIEW` 是**承重的**，不是防御性的。

⇒ 通则：**跑出与既有注释/文档相反的结论时，先怀疑仪器，再怀疑对象。** 具体动作是换一个独立实现重测一次（CLI ↔ Go 驱动、线上二进制 ↔ 源码直跑）。

### ⚠️ `deploy.sh` 的 `--delete` 只保护排除表里的目录

`--exclude` 列表（`/data/` `/logs/` `/queue/` 等）之外、**仓库里不存在**的 runtime 目录，
会被 `rsync --delete` 清掉。已知需要留意的一类是**手工创建的迁移/回滚工件目录**：

- sprint M4-crisis 曾在 `$ATLAS_RUNTIME/m4c-migration/` 放两份库备份、迁移前 replay 基线、
  以及**旧二进制**。其中旧二进制是 7 月构建的、**无法从源码重建**，是回滚路径的一半。
- 处置：此类工件放在受保护路径下（本次放 `$ATLAS_RUNTIME/data/m4c-migration-safe/`，
  `/data/` 在排除表内），或部署前先搬走。**不要依赖「记得先搬」——放在受保护路径里才是设计。**
- 2026-09-27：`data/m4c-migration-safe/` 实测只有原目录 19 个文件中的 6 个 ⇒ 原目录已**整体挪到**
  `$ATLAS_RUNTIME/data/m4c-migration/`（sha256 逐一核对），历史日志 / checkpoint 里的旧路径
  `$ATLAS_RUNTIME/m4c-migration/` 以此为准。`deploy.sh` 排除表同时补了 `/m4c-migration/` 与
  `/aktools_log.log*`（aktools 侧车日志写在 runtime 根，原先也在删除清单里）。

### 服务清单（plist 真相源 `deploy/launchd/`）

| 服务 | 调度 | 职责 |
|---|---|---|
| serve | 常驻 | Web/API；`hestia.config_path` 设了则暴露 `hestia_*` 健康度指标（读 `data/hestia.db`），装不上即启动失败 |
| refresh-us / refresh-cnhk | 每日 08:00 / 20:00 | 行情刷新 + 仓库重建 |
| analysis | 每 30 分钟 | 信号分析 → 通知 |
| crisis-daily | 22:45 / 23:45 / 次日 07:30 | 危机监控每日评估（多时点覆盖 ET 发布，幂等空跑） |
| crisis-nfci | 周三 21:00 / 22:00 | 刷新周频 NFCI（不评估） |
| crisis-intraday-jpy | 每 30 分钟 | 盘中 JPY 检查（非 BREWING/CRISIS 态空跑近零） |
| prism-daily | 每日 08:30 | Prism 估值刷新（理杏仁增量 + 美股 yahoo 重算，在 refresh-us 08:00 之后） |
| aktools | 常驻 | AKShare HTTP 侧车（127.0.0.1:8180，Prism A/H 公司与指数兜底数据源） |
| baostock | 常驻 | Baostock HTTP 桥（127.0.0.1:8181，A 股行情第三跳备源；失败只影响该跳） |

### 危机监控（Cassandra）部署要点

- **首次部署前置**：runtime `data/crisis.db` 需含历史回填——在代码目录跑
  `bin/atlas crisis backfill -c configs/config.yaml --from 2006-01-01` 后把
  `data/crisis.db` 拷贝到 runtime `data/`（deploy.sh 不同步 data/）；HY OAS 2006 起
  历史需人工 CSV 快照补齐（FRED 只保留近三年）：
  `bin/atlas crisis backfill --csv <快照.csv> --indicator hy_oas --scale 100`。
- **FRED key**：runtime `configs/config.yaml` 的 `collectors.fred.api_key`
  （随 deploy.sh 同步并 chmod 600），env `FRED_API_KEY` 可覆盖；密钥不入 plist 不入日志。
- **验证**：`launchctl kickstart gui/$(id -u)/com.newthinker.atlas.crisis-daily`
  后查 `logs/crisis-daily.log`——当日已评估应打印 already evaluated（幂等空跑）；
  `bin/atlas crisis status` 查看当前系统状态与各指标读数。
- 状态语义与阈值调参见 `docs/plans/atlas-macro-crisis-monitor-design.md`（阈值全部在
  `configs/crisis-monitor.yaml`，调参不需发版，重跑 `atlas crisis replay` 验证）。
- 通知频率与机制（各状态收到什么消息、多久一条、排障速查）见
  `docs/ops/crisis-monitor-notifications.md`。

### Prism 估值面板部署要点

- **装载**：plist 随 `install-services.sh` 一并 bootstrap（幂等）；或单独
  `launchctl load ~/Library/LaunchAgents/com.newthinker.atlas.prism-daily.plist`。
  调度每日 08:30，刻意排在 refresh-us（08:00）之后——美股 engine 标的重算依赖当日行情已落库。
- **配置要点**：`configs/config.yaml` 的 `prism.enabled: true` 决定是否启用（同时
  Web 侧 `/prism/board` 路由才注册、estimate 阈值 `low_pct`/`high_pct` 生效）。
  **配置真相源在开发仓** `configs/config.yaml`（gitignored 物理存在，deploy.sh rsync 会
  用它覆盖 runtime 副本）——改配置必须落开发仓再 deploy，仅改 runtime 副本会被下次部署冲掉；理杏仁标的
  复用现有 `collectors.lixinger.api_key`（无需为 prism 单列密钥，env `LIXINGER_API_KEY` 可覆盖），
  密钥不入 plist 不入日志。`db_path`（默认 `data/prism.db`）随 runtime 本地持有，deploy.sh 不同步 data/。
- **EDGAR 配置（美股公司 source=edgar 必需）**：`prism.edgar_user_agent` 必须设为含**联系邮箱**的
  User-Agent（如 `atlas-prism/1.0 (you@example.com)`）——SEC 要求所有 companyfacts 请求携带可联系
  UA，缺失会被 403；存在 `source: edgar` 标的而该项为空时 `prism refresh` 直接报错退出。
  `prism.edgar_lookback_years`（默认 10）控制美股 PE/PB/PS 重建年数（价格仍走 yahoo）。EDGAR 免费、
  单公司单请求，礼貌限速 ≤10 req/s，20 家批量安全。
- **日志路径**：`logs/prism-daily.out.log` / `logs/prism-daily.err.log`。
- **手动触发/首跑**：在 runtime 目录（`/Users/zuowei/workspace/runtime/atlas`）执行
  `bin/atlas prism refresh -c configs/config.yaml`——首次全量回填（理杏仁近 10Y、美股近 5Y），
  之后每日只拉增量（理杏仁请求区间为 latest+1 起，控制理杏豆成本）。**须在 runtime 目录内运行**：
  `db_path`（默认 `data/prism.db`）是相对路径，按进程 cwd 解析，在代码目录跑会把库写错地方、
  与 launchd 任务读写的库不是同一个。
- **验证**：`launchctl kickstart gui/$(id -u)/com.newthinker.atlas.prism-daily` 后查
  `logs/prism-daily.out.log`；浏览器打开 `/prism/board` 目检卡片，`/prism/detail/<symbol>` 目检
  PE 与滚动分位曲线。设计与 M1 验收标准见 `docs/prism/atlas_prism_design.md`。
- **已知限制**：启用 `ATLAS_API_KEY`（API 鉴权）时，`/prism/detail` 的图表会 401 空白——
  页面 JS 用浏览器 `fetch` 调 `/api/prism/series` 不带 `X-API-Key` 头。这与既有 `/symbols/`
  详情页同源继承缺陷，后续与既有页统一修复（如同源会话票据或页面注入 key）；`/prism/board`
  为服务端渲染不受影响。

### 财报桥模板目录 `configs/prism/`（M3 新增）

财报桥（`/prism/sankey/<symbol>`）的分部结构由 YAML 配置驱动，**目录路径是代码常量、不在
`config.yaml` 里**（`cmd/atlas/serve.go` 与 `cmd/atlas/prism.go`），且为**相对路径**，按进程
cwd 解析——与 `db_path` 同样的约束，须在 runtime 目录内运行（launchd `WorkingDirectory` 已指向
runtime）。

| 目录 | 内容 | 缺失时的行为 |
|---|---|---|
| `configs/prism/templates/*.yaml` | 逐家的分部定义与 XBRL member 映射（schema 见设计 §7） | 目录不存在或无 yaml → **桑基服务不启用**，日志 `prism sankey disabled: no templates configured`；`/prism/sankey/*` 与 `/api/prism/sankey` 路由**仍注册**并返回 404（JSON API 返 JSON 404），不 panic、不影响其余 prism 页面 |
| `configs/prism/segments/*.yaml` | 自动解析失败公司的 manual 兜底分部数值（按 `{symbol}.yaml` 小写命名） | 目录/文件不存在是**合法状态**（返回空集，不报错）。**当前首批 5 家全部自动解析成功，故该目录尚未创建** |

- **rsync 白名单确认（结论：无需改 `deploy.sh`）**：`scripts/ops/deploy.sh` 的 rsync 用的是
  **exclude 黑名单**而非白名单，`configs/` 不在任何 `--exclude` 中，因此 `configs/prism/`
  **随 `deploy.sh` 自动同步到 runtime**，新增模板无需改部署脚本。三个连带事实：
  1. `--delete` 生效——**从开发仓删掉一个模板，下次 deploy 会同时删掉 runtime 副本**；反过来，
     只在 runtime 手写的模板会被下次 deploy 抹掉（与 `config.yaml` 同一纪律：**真相源在开发仓**）。
  2. rsync 带 `-m`（prune empty dirs），**空目录不会被同步**——建了 `configs/prism/segments/`
     却没放文件时，runtime 侧不会出现该目录。这不影响功能（缺目录是合法状态）。
  3. 模板是 `.yaml`、不受 `--exclude='*.go'` 影响；`configs/config.yaml` 的 600 权限收紧只作用于
     该文件，模板目录无密钥、保持默认权限。
- **改模板后必须重启 serve**：模板在**启动时读入一次**（`sankey.LoadTemplates`），常驻进程不会
  热加载。新增/修改模板后执行
  `launchctl kickstart -k gui/$(id -u)/com.newthinker.atlas.serve`。
  页面上「未配置模板」的 404 文案也明确写了「添加模板 YAML 后重启服务」。
- **模板写坏会导致整个桑基服务不启用（而非只坏一家）**：`LoadTemplates` 对**格式错误的 YAML**
  与**重复 company**返回 error，serve 记 `prism sankey disabled: loading templates failed` 并带
  文件名继续运行。**故 deploy 后应查一次启动日志**确认 `prism sankey enabled templates=N` 的 N
  与模板文件数一致——否则症状是「桑基页全体 404」，而不是某一家异常。
- **改模板后要重拉历史分部数据**：分部刷新默认走增量锚点，改了 member 映射不会自动重算历史，
  须显式跑 `bin/atlas prism refresh --full-segments -c configs/config.yaml`（AD-12）。
- **未映射的汇总 member 会进 Degraded 文本，属预期**：如 AAPL 的 `ProductMember`、NVDA 的
  `ComputeMember`/`NetworkingMember` 是明细项之和，模板刻意不映射（映射会重复计算，实测朴素合计
  达真实营收的 1.74x/1.90x）。**看到这类 Degraded 条目不必处置**；真正需要关注的是某家公司
  分部行数为 0（解析失败 → 应转 manual 兜底）。

### AKShare 侧车（aktools）部署要点

Prism 的 A/H 公司(茅台/腾讯)与 A 股指数兜底数据源经本地 aktools HTTP 侧车拉取
AKShare。侧车是独立 Python 进程（与 Go 主程序解耦，与 qlib_eval 的 venv 完全隔离）。

- **投递**：`scripts/akshare/`(setup.sh + requirements.txt + requirements.lock)随
  `deploy.sh` rsync 投递到 runtime 目录（`/Users/zuowei/workspace/runtime/atlas`），
  **setup.sh 须在 runtime 侧执行**——venv 落在 `runtime/scripts/akshare/.venv`,与 plist
  `ProgramArguments` 的绝对路径一致(`.venv/` 本身不入 git,仅 lock 追踪)。
- **建 venv**：runtime 侧执行 `bash scripts/akshare/setup.sh`（幂等，重复执行安全）——在
  `scripts/akshare/.venv` 建独立虚拟环境。**有 `requirements.lock` 时按 lock 复现安装**
  （锁定版本、不覆写 lock）；无 lock(首次)时按 `requirements.txt` 安装并 `pip freeze`
  生成 lock。默认解释器 `python3.11`，可传参覆盖（`bash scripts/akshare/setup.sh python3.12`）。
- **装载常驻**：`launchctl load ~/Library/LaunchAgents/com.newthinker.atlas.aktools.plist`
  （plist 真相源 `deploy/launchd/com.newthinker.atlas.aktools.plist`，`KeepAlive` 保活、
  `RunAtLoad` 开机自启）。侧车仅绑 `127.0.0.1:8180`，不对外暴露。
- **配置要点**：runtime `configs/config.yaml` 的 `prism.akshare_base_url`
  （默认 `http://127.0.0.1:8180`）须与 plist 的 `--host/--port` 一致；池内 source=akshare
  的标的（茅台/腾讯）走侧车，source=lixinger 且带 `fallback_source: akshare` 的指数
  （沪深300/中证500）在主源失败时自动降级到侧车。
- **验证**：`curl 127.0.0.1:8180` 侧车可达（返回 aktools 首页）；主程序侧
  `bin/atlas prism refresh -c configs/config.yaml` 后 `/prism/board` 目检茅台/腾讯上墙。
- **日志路径**：`logs/aktools.out.log` / `logs/aktools.err.log`。
- **升级流程**：显式跑 `bash scripts/akshare/setup.sh --upgrade`(忽略 lock、按
  `requirements.txt` 重装到最新并刷新 lock)——**须用 `--upgrade`**，否则默认路径按 lock
  复现安装、不会拉新版本，也不会顺手覆写 lock(升级对比的基准保持稳定)。对比
  `requirements.lock` 前后差异确认变更、验证 refresh 正常后提交更新的 lock 文件;异常可据
  旧 lock 回滚版本。
- **⚠ live 校验点**：AKShare 接口名/字段键（`stock_a_indicator_lg`/`stock_hk_valuation_baidu`/
  `stock_index_pe_lg` 及 `trade_date`/`pe_ttm`/`日期`/`滚动市盈率` 等）与 aktools CLI 旗标名
  （`--host`/`--port`）随上游变动是常态，首跑若不符以实际响应修正代码常量/plist 并同步测试。
- **口径说明**：AKShare 无官方分位，兜底期指数 5Y/10Y 分位为读回本地序列后用
  `valuation.RollingPercentile` 本地计算，与理杏仁官方 cvpos 有方法论差异（数值方向一致、
  绝对值允许差异）。**注意**:降级发生当日,该指数历史序列可能是**混源**的——早先由理杏仁
  写入(其 PE 为 mcw 加权口径)、当日由 akshare 补(乐咕滚动市盈率,亦加权但口径不完全一致),
  故兜底日的分位是该混源序列上的排位,跨源衔接处存在轻微不连续,主源恢复后次日续拉即回归
  单一口径。指数兜底仅有 PE（PB/PS 为空），HK 公司仅有 PE/PB（无 PS）。

### Baostock 桥部署要点

A 股行情降级链的**第三跳**（yahoo → akshare → baostock）经本地 Baostock HTTP 桥拉取。
桥是独立 Python 进程（stdlib `http.server`，不引 FastAPI），与 akshare 的 venv 完全隔离。
该跳失败只影响 A 股行情备源，不影响估值链与其余市场。

- **投递**：`scripts/baostock/`(setup.sh + bridge.py + requirements.txt + requirements.lock)
  随 `deploy.sh` rsync 投递到 runtime 目录，**setup.sh 须在 runtime 侧执行**——venv 落在
  `runtime/scripts/baostock/.venv`，与 plist `ProgramArguments` 的绝对路径一致
  (`.venv/` 本身不入 git，仅 lock 追踪，且已在 deploy.sh 的 `--delete` 排除表内)。
- **建 venv**：runtime 侧执行 `bash scripts/baostock/setup.sh`（幂等，重复执行安全）。
  幂等结构与升级流程与 akshare 完全一致：有 `requirements.lock` 时按 lock 复现安装，
  无 lock(首次)时按 `requirements.txt` 安装并 freeze 生成 lock；升级须显式
  `bash scripts/baostock/setup.sh --upgrade`。默认解释器 `python3.11`，可传参覆盖。
- **装载常驻**：`bash scripts/ops/install-services.sh` 会一并装载
  （plist 真相源 `deploy/launchd/com.newthinker.atlas.baostock.plist`，`KeepAlive` 保活、
  `RunAtLoad` 开机自启）。桥仅绑 `127.0.0.1:8181`，不对外暴露，**不含任何密钥**
  （baostock 匿名登录，无 token 概念）。
- **配置要点**：runtime `configs/config.yaml` 的 `prism.baostock_base_url`
  （默认 `http://127.0.0.1:8181`）须与 bridge.py 内硬编码的监听地址一致。**端口不是 CLI
  旗标**——改端口要同时改 `bridge.py` 与该配置项（这与 aktools 的 `--host/--port` 不同）。
- **验证**：`curl --noproxy '*' 'http://127.0.0.1:8181/daily?code=sh.600519&start=2026-07-01&end=2026-08-01'`
  应返回 `[{"date":"...","close":...}, ...]`。**`--noproxy '*'` 不能省**：本机 env 有
  `http_proxy=127.0.0.1:7897`，curl 会把 127.0.0.1:8181 也走代理，桥未就绪时返回代理的
  **502 而非真实错误**，足以把排障带偏（2026-08-02 实测踩过）。
- **日志路径**：`logs/baostock.out.log` / `logs/baostock.err.log`。
- **口径说明**：`adjustflag=3`（**不复权**，与 PE 计算口径一致）；桥只取 `date,close`
  两列，`close` 为空字符串的行由桥跳过。Go 客户端传 Atlas 形态 symbol（`600519.SH`），
  桥形态（`sh.600519`）的转换在 `internal/collector/baostock` 包内完成。

**排障（含 2026-08-02 实测行为）**：

- **启动慢是正常的，不是卡死**：`bs.login()` 在模块加载时执行，上游不可达时会先阻塞
  **约 30 秒**（socket 超时重试）才轮到 `HTTPServer(...)` 开始监听。故**健康检查必须探
  8181 端口**（`lsof -nP -i TCP:8181`）而非「进程存在」——进程起来了不等于端口已就绪。
- **上游挂了桥不会崩溃循环**：login 失败不抛异常，桥照常常驻并监听，只是每个 `/daily`
  返回 **500 + 错误文本**（如 `baostock query 10002007: 网络接收错误。`）。这是有意设计——
  桥把上游故障显式转 500，降级链才知道该跳失败；若静默返回 `200 []`，下游会误判成
  「该期无数据」而不触发下一跳。**看到 500 先查上游，别怀疑桥本身**。
- **真正会触发 KeepAlive 重启循环的是端口被占**：8181 被别的进程占用时
  `HTTPServer(("127.0.0.1", 8181))` 抛 `OSError: Address already in use` → 进程退出 →
  `KeepAlive` 拉起 → 再失败。症状是 `logs/baostock.err.log` 反复出现同一条 traceback。
  先 `lsof -i :8181` 找出占用者，再 `launchctl kickstart -k gui/$(id -u)/com.newthinker.atlas.baostock`。
- **⚠ 代理对 baostock 无效**：baostock 走**私有 TCP 协议连 `www.baostock.com:10030`**，
  不是 HTTP——`http_proxy`/`https_proxy` 环境变量对它**不起作用**，无法像 yahoo 那样靠给
  plist 注入代理绕过出口封锁（§10 那条 yahoo 403 的修复手段在这里用不上）。判别方法：
  `nc -z -v -w 10 www.baostock.com 10030`。2026-08-02 本机实测该端口**连接超时**（DNS 正常
  解析到 114.94.20.92、HTTP 80 正常返回 301，唯独 10030 不通），故该跳当时无法取到真实
  行情。若出口网络确实封 10030，只能走网络侧放通，桥侧无解。

---

## Environment Variables

| Variable | Description | Required |
|----------|-------------|----------|
| `FRED_API_KEY` | FRED API key（危机监控；覆盖主配置 collectors.fred.api_key） | If crisis monitor enabled |
| `TELEGRAM_BOT_TOKEN` | Telegram bot token for notifications | If Telegram enabled |
| `TELEGRAM_CHAT_ID` | Telegram chat ID for notifications | If Telegram enabled |
| `ANTHROPIC_API_KEY` | Claude API key for LLM features | If LLM enabled |
| `OPENAI_API_KEY` | OpenAI API key | If using OpenAI |
| `LIXINGER_API_KEY` | Lixinger API key for fundamentals | If Lixinger enabled |
| `FUTU_TRADE_PWD` | Futu trade password | If Futu broker enabled |

### Using .env File

Create `.env` in your project root:

```bash
TELEGRAM_BOT_TOKEN=123456:ABC-DEF...
TELEGRAM_CHAT_ID=-1001234567890
ANTHROPIC_API_KEY=sk-ant-...
```

Load in shell:

```bash
export $(cat .env | xargs)
./bin/atlas serve -c config.yaml
```

---

## Database Setup

### TimescaleDB (Recommended for Production)

```bash
# Install TimescaleDB
# Ubuntu/Debian
sudo apt install timescaledb-2-postgresql-15

# Create database
sudo -u postgres psql << 'EOF'
CREATE USER atlas WITH PASSWORD 'your_password';
CREATE DATABASE atlas OWNER atlas;
\c atlas
CREATE EXTENSION IF NOT EXISTS timescaledb;
EOF
```

Update `config.yaml`:

```yaml
storage:
  hot:
    dsn: "postgres://atlas:your_password@localhost:5432/atlas"
    retention_days: 90
  cold:
    type: localfs
    path: "/opt/atlas/data/archive"
```

### S3 Cold Storage

For S3-compatible storage (AWS S3, MinIO):

```yaml
storage:
  cold:
    type: s3
    s3:
      bucket: "atlas-archive"
      endpoint: "https://s3.amazonaws.com"  # or MinIO endpoint
      region: "us-east-1"
      access_key: "${AWS_ACCESS_KEY_ID}"
      secret_key: "${AWS_SECRET_ACCESS_KEY}"
      prefix: "atlas/"
```

---

## Reverse Proxy

### Nginx

Create `/etc/nginx/sites-available/atlas`:

```nginx
server {
    listen 80;
    server_name atlas.example.com;
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name atlas.example.com;

    ssl_certificate /etc/letsencrypt/live/atlas.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/atlas.example.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8090;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

Enable the site:

```bash
sudo ln -s /etc/nginx/sites-available/atlas /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx
```

### Caddy

Create `Caddyfile`:

```
atlas.example.com {
    reverse_proxy localhost:8090
}
```

---

## Health Checks

ATLAS exposes a health endpoint:

```bash
curl http://localhost:8090/api/health
# {"status":"ok"}
```

For Docker health checks:

```yaml
services:
  atlas:
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:8090/api/health"]
      interval: 30s
      timeout: 10s
      retries: 3
```

---

## Logging

Logs are written to stdout in JSON format. Use your preferred log aggregation tool:

```bash
# View logs with jq
./bin/atlas serve -c config.yaml 2>&1 | jq .

# With systemd
journalctl -u atlas -f | jq .
```

### Log Levels

Set debug mode for verbose logging:

```bash
./bin/atlas serve -c config.yaml --debug
```

---

## Backup

### Configuration Backup

```bash
# Backup config and environment
tar -czf atlas-config-$(date +%Y%m%d).tar.gz \
    /opt/atlas/config.yaml \
    /opt/atlas/.env
```

### Data Backup

```bash
# Backup archive data
tar -czf atlas-data-$(date +%Y%m%d).tar.gz \
    /opt/atlas/data/archive/

# For TimescaleDB
pg_dump -U atlas atlas > atlas-db-$(date +%Y%m%d).sql
```

---

## Troubleshooting

### Common Issues

**Port already in use:**
```bash
# Find process using port 8090
lsof -i :8090
# Kill it or change port in config
```

**Permission denied:**
```bash
# Fix file permissions
sudo chown -R atlas:atlas /opt/atlas
```

**Cannot connect to database:**
```bash
# Check PostgreSQL is running
sudo systemctl status postgresql
# Check connection
psql -U atlas -h localhost -d atlas
```

**Telegram notifications not working:**
```bash
# Test bot token
curl "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/getMe"
# Test sending message
curl "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/sendMessage?chat_id=${TELEGRAM_CHAT_ID}&text=test"
```
