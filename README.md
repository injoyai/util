# util

按需引入的 Go 通用工具库，提供文件操作、缓存、并发任务、HTTP、字符串和时间处理等能力。

- **Go 1.25+**，核心模块仅依赖标准库。
- 各功能独立分包，直接导入 `github.com/injoyai/util/<包名>`。
- SQLite 时序存储作为[独立子模块](sqlitets/README.md)提供，按需接入。

## 接入

目前尚未发布版本，可在使用它的 Go 项目中通过本地 `replace` 引用。将下方路径替换为本仓库的实际绝对路径：

```sh
go mod edit -require=github.com/injoyai/util@v0.0.0
go mod edit -replace=github.com/injoyai/util=/absolute/path/to/util
```

添加所需包的 `import` 后执行 `go mod tidy`。`v0.0.0` 在这里仅用于本地引用，不代表已发布的版本。

## 快速开始

以下示例保留最近三条记录。保存为调用项目的 `main.go`，执行 `go mod tidy` 和 `go run .`：

```go
package main

import (
    "fmt"
    "log"

    "github.com/injoyai/util/cache"
)

func main() {
    recent, err := cache.NewCycle[int](3)
    if err != nil {
        log.Fatal(err)
    }
    for _, n := range []int{1, 2, 3, 4} {
        recent.Append(n)
    }
    fmt.Println(recent.List(0)) // [2 3 4]
    fmt.Println(recent.List(2)) // [3 4]
}
```

## 功能导航

| 需求 | 包 | 主要能力 |
| --- | --- | --- |
| 文件与目录 | [`oss`](oss) | 创建目录、创建或覆盖文件、JSON 保存、流式复制、用户数据目录 |
| 缓存 | [`cache`](cache) | 固定容量环形缓存、显式保存的 JSON 文件缓存 |
| 并发 Map | [`maps`](maps) | 泛型键值存储、TTL、原子操作、快照、事件订阅、稀疏位图 |
| 任务执行 | [`task`](task) | 可取消重试、有限并发批量处理、可重启及替换任务 |
| HTTP 请求与下载 | [`httputil`](httputil) | JSON 请求、流式下载、上下文取消、状态与大小检查 |
| 字符串 | [`str`](str) | Unicode 反转、截断、分段、裁剪、随机字符串 |
| 切片 | [`slice`](slice) | 泛型去重、交集、负索引、基于 offset 的分页 |
| 日期与时间 | [`times`](times) | 保留时区的秒至年边界、季度计算 |
| 通道与消息 | [`chans`](chans) | 安全关闭通道、主题订阅、消息内存流、覆盖队列、一次性结果 |
| 资源关闭 | [`safe`](safe) | 关闭原因、通知通道、回调及状态重置 |
| 对象复用 | [`pool`](pool) | 有界闲置对象复用池 |
| 响应等待 | [`maps/wait`](maps/wait) | `wait.New[K,V]` 创建等待器，按 key 排队等待或共享响应 |
| 心跳与超时 | [`maps/keep`](maps/keep)、[`maps/timeout`](maps/timeout) | 连续心跳检查；`timeout.New[K](timeout, interval)` 指定超时时长和检查间隔 |
| 设备序列号 | [`sn`](sn) | 从 Linux、macOS、Windows 硬件标识生成 16 字符 SN |
| 大小与速率 | [`bytesize`](bytesize) | 1024 进制大小解析、格式化、每秒速率 |
| CSV | [`csv`](csv) | UTF-8 BOM 识别、流式读取、写入错误检查 |
| 压缩与解压 | [`archive`](archive) | ZIP 打包与解压、tar.gz 解压、gzip 流式编解码 |
| 网络地址 | [`netutil`](netutil) | IPv4 端点解析、本机 IPv4、受信代理下的客户端 IP |
| CRC 校验 | [`crc`](crc) | CRC-8、CRC-16 参数表、校验及增量计算 |
| 二维几何 | [`geo`](geo) | 点间距离、线段相交、简单多边形包含与相交 |

无需指定类型参数时，可用 `maps.Default()`、`wait.Default()` 和 `timeout.Default()` 创建实例，key 和值的类型为 `any`。`wait.Default()` 默认等待超时为十秒。每次调用都返回独立实例；key 的实际类型必须可比较，例如字符串或整数，不能使用切片、map 或函数。

`timeout.New[string](time.Minute, 10*time.Second)` 表示一分钟未收到心跳即超时，每十秒检查一次。`timeout.Default()` 使用一分钟超时、十秒检查间隔。检查间隔非正时使用十秒；超时时长非正时在下次检查中视为超时。

## 常用示例

以下片段省略 `package` 声明，可放入自己的包中使用。

### 保存文件

`oss.New(path)` 创建目录；传入内容则创建或覆盖文件，并自动创建父目录。内容支持 `string`、`[]byte`、`io.Reader`、`nil`，多段内容按顺序拼接。`oss.New(path, nil)` 会创建空文件或清空已有文件。

```go
import (
    "path/filepath"

    "github.com/injoyai/util/oss"
)

func saveConfig() error {
    dir, err := oss.UserDataDir("myapp")
    if err != nil {
        return err
    }
    return oss.New(filepath.Join(dir, "config.txt"), "mode=local\n")
}
```

`UserDataDir` 只返回路径，不创建目录或迁移数据；需要空目录时显式调用 `oss.New(dir)`。

| 系统 | 用户数据基础目录 |
| --- | --- |
| macOS | `$HOME/Library/Application Support` |
| Windows | `%LOCALAPPDATA%` |
| Linux 及其他平台 | 绝对路径的 `$XDG_DATA_HOME`，否则使用 `$HOME/.local/share` |

文件默认权限为 0644，目录权限为 0755（目录权限受 umask 影响）。JSON 数据使用 `oss.WriteJSON`；自定义文件权限使用 `oss.WriteFile` 或 `oss.Write`。写入失败保留原目标，成功替换后不保留旧权限；`io.Reader` 由调用方关闭。Unix 下替换具有原子可见性，其他平台取决于 `os.Rename`，不承诺掉电持久性。路径片段按 `filepath.Join` 拼接，不限制访问范围。

### 使用带有效期的并发 Map

```go
import (
    "fmt"
    "time"

    "github.com/injoyai/util/maps"
)

func useMap() {
    m := maps.New[string, int]()
    defer m.Close()

    m.Set("count", 1, time.Minute)
    value, exists := m.Get("count")
    fmt.Println(value, exists) // 1 true
}
```

`maps` 在内存中保存原始类型值；需要持久化时，使用 `cache.NewFile` 的 `Set/Get/Save`。文件缓存只有显式 `Save` 才落盘，同一实例支持并发，不同实例或进程之间没有写入协调。

`m.Close()` 停止自动清理并关闭订阅，保留已有值。`Subscribe(key, capacity)` 的通知缓冲区满时丢弃新通知，不能当作可靠消息队列。

### 限制批量任务的并发数

```go
import (
    "context"
    "strings"

    "github.com/injoyai/util/task"
)

func uppercase(ctx context.Context, inputs []string) ([]task.Result[string], error) {
    return task.Map(ctx, inputs, 4,
        func(ctx context.Context, input string) (string, error) {
            if err := ctx.Err(); err != nil {
                return "", err
            }
            return strings.ToUpper(input), nil
        })
}
```

最多同时执行 4 个任务，`results[i]` 对应 `inputs[i]`，包含 `Value` 和 `Err`；返回的汇总错误支持 `errors.Is/As`。取消后停止派发，并等待已启动任务退出，因此处理函数必须响应上下文。

需要重试时用 `task.Retry`，重试次数指首次执行之外的次数。长期任务可用 `task.NewRunner` 或 `task.NewOneRun`；`Stop()` 会取消并等待退出，不得在处理函数内部调用同一实例的 `Stop()`。

### 请求 JSON

```go
import (
    "context"
    "net/http"
    "time"

    "github.com/injoyai/util/httputil"
)

func fetchJSON(ctx context.Context, url string, dst any) error {
    client, err := httputil.New(&http.Client{Timeout: 15 * time.Second}, 2<<20)
    if err != nil {
        return err
    }
    return client.JSON(ctx, http.MethodGet, url, nil, dst, nil)
}
```

示例将 JSON 响应限制为 2 MiB。`httputil.New(nil, 0)` 使用 30 秒超时和 4 MiB 上限；代理、Transport 和 CookieJar 通过标准库 `http.Client` 配置。

非 2xx 响应返回 `*httputil.StatusError`。下载使用 `client.Download(ctx, url, filename, maxBytes)`，必须显式指定大小上限，失败时保留已有目标文件。请求不会自动重试。

### 获取设备序列号

调用 `sn.Get()` 返回 `(string, error)`，成功时为 16 个大写十六进制字符，无需传参或创建本地文件。显示前缀可由应用自行添加。

| 系统 | 硬件来源 |
| --- | --- |
| Linux | 按顺序读取 SMBIOS UUID、设备树序列号、CPU Serial、MMC CID |
| macOS | 系统 `ioreg` 提供的 `IOPlatformUUID` |
| Windows | 系统 PowerShell/CIM 提供的 SMBIOS UUID |

读取失败、标识无效或无可用来源时返回错误。Linux 仅在某来源不存在时尝试下一项，MMC 必须恰好有一个设备；macOS 和 Windows 的外部命令有 10 秒超时，不需要安装第三方工具。

硬件来源与编号不变时 SN 保持不变；更换硬件、修改固件或克隆虚拟机可能影响稳定性与唯一性，不保证跨操作系统一致或绝对唯一。当前 `sn.Get()` 与旧 `sn.Derive(key)` 生成结果不兼容，已有编号需由应用安排迁移。

## SQLite 时序存储

[`sqlitets`](sqlitets/README.md) 提供嵌入式 SQLite 时序存储，支持 JSON 字段、UTC 月分库、跨月查询、过滤、分页、聚合和过期清理，无需独立数据库服务。

它是独立 Go 子模块，包含自己的 SQLite 驱动依赖。本地使用时需单独配置，核心模块的 `replace` 不会覆盖子模块：

```sh
go mod edit -require=github.com/injoyai/util/sqlitets@v0.0.0
go mod edit -replace=github.com/injoyai/util/sqlitets=/absolute/path/to/util/sqlitets
```

添加 `github.com/injoyai/util/sqlitets` 和 `github.com/injoyai/util/sqlitets/sqlite` 的导入后执行 `go mod tidy`。完整示例、查询语义和资源关闭方式见[子模块使用文档](sqlitets/README.md)。

## 使用注意

- **切片与共享值**：`slice.Limit` 返回共享视图，需要独立切片时用 `slices.Clone`。`cache.Cycle.List` 返回独立切片，但元素内的指针仍由调用方管理；并发容器不会自动同步值所引用的对象。
- **文本与大小单位**：Unicode 操作按码点处理，组合字符和 emoji 序列可能被拆开。`bytesize.Parse` 的 `KB/MB` 按 1024 进制解析，输出使用 `KiB/MiB`。
- **解压**：默认最多 10,000 条目、1 GiB 文件正文，拒绝路径穿越、链接、特殊文件和覆盖已有文件。失败时已成功条目会保留，建议解压到独立空目录。
- **客户端 IP**：`netutil.ClientIP` 默认只认直接对端；明确指定受信代理后才解析 `X-Forwarded-For`，代理必须正确追加或清洗该头。
- **关闭通知**：`safe.Closer.Done()` 表示已发起关闭，不表示回调已完成；`CloseWithErr(nil)` 不关闭实例。`Reset()` 不等待旧资源或回调退出，需由调用方协调。
- **几何与校验**：`geo` 只处理有限坐标的简单二维多边形，边界接触与包含均算相交，不支持孔洞、自相交或布尔裁剪。CRC 用于数据校验，不能用于身份认证；协议字节序由调用方显式编码。

## API 与迁移资料

在配置好模块引用的项目中，可用 `go doc github.com/injoyai/util/<包名>` 查看接口；具体方法可继续追加类型或函数名，例如 `go doc github.com/injoyai/util/oss.New`。

从旧库迁入时需逐项核对 API，不能只替换导入前缀：

- [goutil 迁移对照](MIGRATION.md)
- [base 迁移对照](BASE_MIGRATION.md)
- CRC 上游许可：[CRC-8](crc/LICENSE.crc8)、[CRC-16](crc/LICENSE.crc16)
