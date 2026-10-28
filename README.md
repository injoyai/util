# util

从 `github.com/injoyai/goutil` 和 `github.com/injoyai/base` 筛选、整理的通用 Go 工具库。核心模块按功能独立分包，仅依赖标准库，最低 Go 1.25。模块名沿用旧项目的作者命名：`github.com/injoyai/util`。

迁移保留功能意图，不维持旧 API 兼容性。旧项目未修改；goutil 的取舍见 [MIGRATION.md](MIGRATION.md)，base 的完整范围与 API 对照见 [BASE_MIGRATION.md](BASE_MIGRATION.md)。本目录尚未发布，使用其他项目时可先通过本地 `replace` 引用。

| 包 | 功能 |
| --- | --- |
| `str` | Unicode 反转、截断、分段、裁剪、无共享随机源的随机字符串 |
| `sn` | 自动读取 Linux、macOS、Windows 硬件标识，生成 16 字符序列号 |
| `slice` | 泛型去重、交集、负索引、基于 offset 的分页 |
| `times` | 保留时区的秒、分钟、小时、日、周、月、季度、年边界 |
| `cache` | 泛型环形缓存、JSON 文件缓存，支持同一实例并发使用 |
| `maps` | 泛型并发 Map、过期、原子操作、快照、事件与订阅、稀疏位图 |
| `maps/wait` | 按 key 排队等待或共享响应，有限次数回调及关闭清理 |
| `maps/keep`、`maps/timeout` | 连续心跳检查、可取消的超时处理循环 |
| `chans` | 安全关闭通道、主题订阅、消息内存流、覆盖队列、一次性结果 |
| `safe` | 可重置的关闭状态、关闭原因、通知通道及关闭回调管理 |
| `pool` | 有界闲置对象复用池 |
| `crc` | CRC-8/CRC-16 参数表、校验与增量计算 |
| `oss` | 用户数据目录、一步建目录/新建或覆盖文件、临时文件替换、流式复制、JSON 保存 |
| `bytesize` | 1024 进制大小解析、格式化、每秒速率 |
| `csv` | UTF-8 BOM 识别、CSV 流式读取、正确检查写入及 Flush 错误 |
| `archive` | ZIP 打包/解压、tar.gz 解压、gzip 流式编解码 |
| `httputil` | JSON 请求、流式下载、上下文取消、非 2xx 错误、大小限制 |
| `netutil` | IPv4 端点解析、本机 IPv4、明确代理信任规则的客户端 IP |
| `task` | 可取消重试、有限并发批量任务、可重启任务及取消前次执行的单任务管理 |
| `geo` | 点到点距离、线段相交、简单多边形包含和相交 |

## SQLite 时序存储子模块

[`sqlitets`](sqlitets/README.md) 是独立 Go 子模块 `github.com/injoyai/util/sqlitets`，提供 JSON 字段时序存储、UTC 月分库、跨月查询、过滤、分页、聚合和过期清理。它保留自己的 `go.mod`、`go.sum` 和 SQLite 驱动依赖，核心模块不依赖它。

使用时引入 `github.com/injoyai/util/sqlitets` 和 `github.com/injoyai/util/sqlitets/sqlite`；共享月分库管理器位于 `github.com/injoyai/util/sqlitets/monthly`。本地引用需单独将子模块路径 replace 到本项目的 `sqlitets` 目录，完整示例见子模块 README。

## 使用

只引入所需的包，标准库已有的功能直接使用标准库。

## base 迁入能力

并发 Map 与文件缓存各自独立：`maps` 存储内存中的原始类型值，支持 TTL 与原子操作；`cache.File` 负责 JSON 编码与显式保存。

```go
m := maps.NewGeneric[string, int]()
defer m.Close() // 停止自动清理并关闭订阅，保留已有值。
m.Set("count", 1, time.Minute)
value, exists := m.Get("count")

sub, err := m.Subscribe("count", 8)
if err != nil {
    return err
}
defer sub.Close()
m.Set("count", 2)
next := <-sub.Chan // 只读通道；Map 订阅满时丢弃新通知。
```

`chans.NewSafe`、`NewIO`、`NewQueueCover`、`pool.New`、`task.NewRunner`、`task.NewOneRun` 返回 `(对象, error)`。任务的 `Stop()` 取消并等待退出，处理函数必须响应上下文，且不得在处理函数内部调用同一实例的 `Stop()`。

```go
r, err := task.NewRunner(func(ctx context.Context) error {
    <-ctx.Done()
    return ctx.Err()
})
if err != nil {
    return err
}
if err := r.Start(ctx); err != nil {
    return err
}
r.Stop()
err = r.Err() // 获取本轮执行结果。
```

CRC 使用 `crc.Checksum16(data, crc.MakeTable16(crc.CRC16_MODBUS))`。返回数值的协议字节序由调用方用 `encoding/binary` 显式编码。CRC 算法和向量的上游归属及 MIT 许可见 `crc/LICENSE.crc8` 与 `crc/LICENSE.crc16`。

`github.com/injoyai/util/safe` 保留 `Closer` 的完整接口，适合嵌入连接、连接池等资源对象：

```go
c := safe.NewCloser().SetCloseFunc(func(reason error) error {
    return resource.Close()
})
err := c.CloseWithErr(reason) // 返回资源关闭回调的错误。
cause := c.Err()             // 保存的是首次关闭原因。
```

`Done()` 在执行关闭回调前关闭，只表示已发起关闭；重复 Close 不等待回调。`CloseWithErr(nil)` 沿用旧约定，不关闭实例。`Reset()` 保留回调、创建新的通知通道；调用方必须协调旧资源及回调退出，旧 Done 通道保持关闭。

## 其他能力示例

使用 `github.com/injoyai/util/sn` 自动获取本机 SN，无需传参：

```go
serial, err := sn.Get()
if err != nil {
    return err
}
serial = "EDGE-" + serial // 显示前缀可自行添加
```

| 系统 | 硬件来源 |
| --- | --- |
| Linux | 按顺序检查 SMBIOS `product_uuid`、设备树 `serial-number`、`/proc/cpuinfo` 的 `Serial`、MMC CID，不按 CPU 架构或机型分支 |
| macOS | 使用系统自带的 `/usr/sbin/ioreg` 读取 `IOPlatformUUID` |
| Windows | 使用 `%SystemRoot%\System32\WindowsPowerShell\v1.0\powershell.exe` 和 CIM 读取 `Win32_ComputerSystemProduct.UUID` |

Linux 仅在某来源不存在时检查后续来源；来源存在但读取失败、为空或为占位值时返回错误，不切换到其他编号。CPU 的 `Serial` 缺失视为不支持，不同 CPU Serial 同时出现视为歧义。MMC 扫描所有 host，按 `type=MMC` 排除 SD；必须恰好存在一个 MMC 设备。MMC CID 标识存储器，不能单凭该值保证对应主板或焊接式 eMMC，更换该存储器会改变 SN。

UUID 去除标准连字符并统一为大写十六进制；MMC CID 固定为 32 个十六进制字符并统一大写；其他序列号保留大小写和内部分隔符。空值、全零、全 F、常见 OEM 占位值以及格式无效的 UUID/CID 会被拒绝。Windows 和 macOS 的外部命令有 10 秒超时，执行失败时返回错误；不要求安装第三方依赖。来源依据见 [Microsoft SMBIOS UUID](https://learn.microsoft.com/en-us/windows/win32/cimwin32prov/win32-computersystemproduct)、[Apple IOPlatformUUID](https://developer.apple.com/documentation/iokit/kioplatformuuidkey)、[Linux MMC 属性](https://docs.kernel.org/driver-api/mmc/mmc-dev-attrs.html)。

编码契约为 `SHA-256("sn:v1\x00" + source + "\x00" + normalizedIdentity)` 的前 8 字节，输出 16 个大写十六进制字符，由固定样例测试锁定。函数只读取硬件信息，不写文件，不使用随机值、MAC 地址或操作系统安装 ID。硬件来源与编号保持不变时，SN 保持不变；更换硬件、修改固件、改变可见硬件来源或克隆虚拟机可能影响稳定性与唯一性，不保证不同操作系统在同一设备上读取到相同身份。64 位摘要不能消除原始硬件编号重复，也不保证摘要绝对唯一。

原 `sn.Derive(key)` 已替换为 `sn.Get()`，身份来源参与摘要；此前版本生成的 SN 与当前契约不兼容，已有存储和引用需要由应用制定迁移策略。

```go
package main

import (
    "fmt"
    "github.com/injoyai/util/cache"
)

func main() {
    recent, err := cache.NewCycle[int](3)
    if err != nil {
        panic(err)
    }
    for _, n := range []int{1, 2, 3, 4} {
        recent.Append(n)
    }
    fmt.Println(recent.List(0)) // [2 3 4]
}
```

批量处理会保留结果顺序并汇总错误；处理函数需要响应上下文。

```go
results, err := task.Map(ctx, inputs, 4,
    func(ctx context.Context, input string) (string, error) {
        if err := ctx.Err(); err != nil {
            return "", err
        }
        return strings.ToUpper(input), nil
    })
// results[i] 对应 inputs[i]；err 可通过 errors.Is/As 检查。
```

HTTP 使用标准库 `http.Client` 配置代理、Transport、CookieJar、超时：

```go
client, err := httputil.New(&http.Client{Timeout: 15 * time.Second}, 2<<20)
if err != nil {
    return err
}
err = client.JSON(ctx, http.MethodGet, endpoint, nil, &result, nil)
// 下载需要显式指定上限；失败保留已有目标文件。
_, err = client.Download(ctx, downloadURL, filename, 100<<20)
```

文件操作使用 `github.com/injoyai/util/oss`，保留旧 `oss.New` 的一步调用方式：

```go
err := oss.New("data/logs")                            // 无内容：创建目录
err = oss.New("data/config.txt", "hello")              // 新建文件，已有则覆盖
err = oss.New("data/config.txt", nil)                  // 新建空文件，已有则清空
err = oss.New("data/result.txt", "header\n", data, r)   // 拼接字符串、[]byte、io.Reader
// 每次调用都应检查 err；reader 由调用方关闭。
```

`oss.New` 自动创建父目录（0755），文件权限固定为 0644；需要指定权限时使用 `oss.WriteFile` 或 `oss.Write`。内容写入成功后才替换目标，读取或写入失败保留原文件。传入不支持的类型会在创建目录或读取流前报错；结构化数据请显式使用 `oss.WriteJSON`。这里借鉴旧接口的使用方式，独立实现，不引入 `conv` 或其他依赖。

用户数据目录可追加应用名等路径片段，目录无法确定时返回错误：

```go
dir, err := oss.UserDataDir("myapp", "data")
if err != nil {
    return err
}
err = oss.New(dir) // 需要时显式创建目录
```

| 系统 | `oss.UserDataDir()` 基础目录 |
| --- | --- |
| macOS | `$HOME/Library/Application Support` |
| Windows | `%LOCALAPPDATA%` |
| Linux 及其他平台 | `$XDG_DATA_HOME`；未设置、为空或为相对路径时使用 `$HOME/.local/share` |

路径规则参考 [Apple Application Support](https://developer.apple.com/documentation/foundation/url/applicationsupportdirectory)、[Windows LocalAppData](https://learn.microsoft.com/en-us/windows/win32/shell/knownfolderid) 和 [XDG 规范](https://specifications.freedesktop.org/basedir/0.8/)。函数只返回路径，不创建目录或迁移旧数据；`parts` 按 `filepath.Join` 拼接，不作为安全路径边界。Linux 不再返回旧版的 `/var/lib`；Windows 尊重 `%LOCALAPPDATA%`，不再硬编码用户目录下的 `AppData/Local`。基础目录缺失或非绝对路径时返回错误。

## 约定及限制

- 参数错误通过 `error` 或 `(value, ok)` 返回；不使用 `Must*`、隐式错误吞掉或自动恢复 panic。
- `cache.Cycle.List` 返回独立切片，但元素内的指针仍由调用方管理。`slice.Limit` 是共享视图，需要独立结果时使用 `slices.Clone`。
- `maps.GetOrSet/GetOrSetByHandler/GetOrSetByHandler2` 返回本次选定或生成的值；即使返回前 TTL 已过期，也不会将本次结果替换成零值。过期只影响后续读取。
- `chans.IO.Close` 解除阻塞写入并等待在途写入结束，读取端排空已接受的消息后才返回 EOF。
- `maps/timeout.Run` 在每次超时回调前复核心跳及当前超时配置；已确认的回调仍在锁外执行，之后的 Keep 不会撤回该回调。
- 文件缓存的值在 `Set` 时编码，在 `Get` 时解码。只有显式 `Save` 才落盘。同一实例支持并发；不同实例/进程之间无写入协调，最后保存者覆盖。
- `oss.Write` 先写临时文件、Sync、Close，再同目录 Rename。Unix 提供原子可见性，其他平台依赖 `os.Rename` 的语义；未 Sync 父目录，不承诺掉电后的目录项持久化。替换会采用传入权限。
- 解压默认最多 10,000 条目、1 GiB 文件正文。拒绝父路径、绝对路径、链接、特殊文件及已有文件覆盖；使用 `os.Root` 限制路径范围。tar 元数据另有流大小限制。失败会删除当前半成品文件，已成功条目保留；建议解压到独立空目录，成功后再使用。
- HTTP 默认 JSON 正文上限 4 MiB。默认客户端超时 30 秒；自定义客户端的超时由调用方决定。下载必须指定上限。不会隐式重试 POST 等请求。
- `ClientIP` 默认只认直接对端。只有明确列入受信代理范围时才解析 `X-Forwarded-For`，前提是代理正确追加或清洗该头。
- Unicode 操作按码点进行，不按完整字素簇进行；组合字符和 emoji 序列可能被拆开。
- `bytesize.Parse` 的 `KB/MB` 沿用旧项目的 1024 进制，输出统一为 `KiB/MiB`。非整数总字节数和 uint64 溢出返回错误。
- `geo` 只处理有限坐标的简单二维多边形；边界接触与包含都算相交。绝对容差为 `1e-9` 坐标单位，不支持自相交、孔洞或布尔裁剪。

## 验证

```sh
go test -race -cover ./...
go vet ./...
go build ./...
# 独立子模块必须单独验证，根目录的 ./... 不包含它。
(cd sqlitets && go test -race ./...)
(cd sqlitets && go vet ./...)
(cd sqlitets && go build ./...)
```

核心模块测试只使用临时文件及本机 HTTP 服务，无外部服务、凭据或第三方依赖。网络受限沙箱需要允许测试监听本机端口。Linux/Windows 可用 `GOOS=linux go build ./...` 和 `GOOS=windows go build ./...` 检查核心模块编译；跨平台编译不等于运行验证。子模块测试使用临时 SQLite 数据库，首次运行需要下载其已有驱动依赖。
