# goutil → util 迁移决策

`base` 的迁移单独记录在 [BASE_MIGRATION.md](BASE_MIGRATION.md)，包括新增并发组件和未原样搬入的设计。

本次按“可独立复用、功能边界清晰、不绑定业务或外部服务”筛选。`goutil` 保留原样；舍弃指不把旧设计迁入新库，并未删除源文件。外部集成仍可能有价值，但不纳入这次轻量通用库的范围。

本项目的文件工具包由 `fs` 更名为 `oss`，原 `github.com/injoyai/util/fs` 的调用方需改用 `github.com/injoyai/util/oss` 并更新调用前缀；函数签名和文件操作行为保持一致。

## 保留并重写

| 旧位置 | 新 API | 主要变化 |
| --- | --- | --- |
| `str` 的反转、重复检测、裁剪、分段、截断 | `str.Reverse/HasRepeat/CropFirst/CropLast/SplitAt/Truncate` | 反转只用一个 rune 切片；分段明确越界返回值并支持负索引；截断按码点，不再 NUL 填充；裁剪明确传 retain |
| `str.Rand` | `str.Random(length, alphabet)` | 使用 `crypto/rand`；正确字符表；支持 Unicode；拒绝负长度和重复字符；移除有数据竞争的全局 rand.Rand |
| `str.FindCommon`、`str.List` 的有效列表能力、`other/paging` | `slice.Common/Unique/Get/Limit` | 泛型、稳定去重、负索引、分页防整数溢出；排序/克隆直接使用 `slices` |
| `times` | `times.Integer*`、`times.Quarter`、格式常量 | 保留日历边界算法；使用原生 `time.Time`，避免包装类型改变 `Date` 返回约定和 Unix 秒乘纳秒溢出 |
| `cache`、`cache/v2` 环形缓存 | `cache.NewCycle[T]/Append/List/Padding/Clear` | 合并历史版本；验证容量；读写锁；快照不共享内部切片；移除混在缓存中的订阅、中间件和保存格式 |
| `cache`、`cache/v2`、`cache/v3` 文件缓存 | `cache.NewFile/Set/Get/Del/Clear/Save` | JSON 编码隔离值；显式错误；缺文件可初始化；损坏文件不吞错；同实例保存串行；临时文件替换 |
| `oss/oss_file.go` | `oss.Write/WriteFile/WriteJSON/Copy` | 分离目录和文件语义；不依赖 conv；默认目录 0755；关闭和同步后替换；失败不破坏原文件；同路径 Copy 不截断源 |
| `oss.New` | `oss.New(filename, contents...)` | 恢复一步建目录、新建/覆盖文件及多段内容拼接；无内容建目录，显式 nil 建空文件；支持 string/[]byte/io.Reader/nil，其他类型预先报错；目录 0755、文件 0644；复用临时文件替换，读写失败保留原文件 |
| `oss.UserDataDir` | `oss.UserDataDir(parts...) (string, error)` | 保留路径拼接；macOS 沿用 Application Support；Windows 读取 LOCALAPPDATA；Linux 采用 XDG_DATA_HOME 或 ~/.local/share，忽略相对的 XDG 路径；目录无法确定时显式报错；不创建目录或迁移旧数据 |
| `oss/oss_size.go` | `bytesize.Parse/Format/Speed` | 用有理数精确解析 uint64；不把负 int64 转成巨大的无符号大小；明确解析错误；速率先计算再选单位；拒绝零耗时 |
| `oss/csv`、`oss/oss_file_csv.go` | `csv.Read/Range/Write` | 小于三字节的 CSV 也能读取；识别 UTF-8 BOM；UTF-16 返回明确错误；检查 Flush；不做 any 的隐式类型转换 |
| `oss/compress/{zip,tar,gzip}` | `archive.CreateZIP/ExtractZIP/ExtractTarGZ/Gzip/Gunzip` | 流式 I/O；收尾错误传播；阻止解压路径逃逸和覆盖；实际正文和条目限制；ZIP 防自包含，含符号链接目录情况 |
| `net/http`、`net/http/v2`、下载封装 | `httputil.New/JSON/Download` | 标准 http.Client 注入；显式上下文；非 2xx 可 errors.As；不忽略 Bind 错误；不重复使用已消费请求正文；下载不再固定 `.downloading` 路径或 Sleep 后 Rename |
| `net/ip` | `netutil.ParseV4/PeerIP/ClientIP/LocalIPv4` | 标准 netip；真正验证 IPv4 和端口范围；不修改请求；明确代理信任；正确网卡标志判断；不依赖公网探测网站 |
| `g.Retry/Retry2`、`task.Range`、`oss/runtimes` 有限并发能力 | `task.Retry/Map` | 明确 retries 为额外重试次数；等待可取消；固定 worker 上限；结果顺序稳定；汇总错误；取消后等待已启动任务退出 |
| `task.Download`、`task.HHD` 的批量意图 | 组合 `task.Map`、`httputil.Download` 与原生 I/O | 移除流程耦合；旧 Download.GetSize 在未分配指针上赋值，HHD 未正常关闭结果通道；新接口保留错误和结束契约 |
| `math/geo` | `geo.Point/Line/Polygon`、距离、叉积、包含及相交 | `math.Hypot`；端点接触、共线重叠、退化线段；完整包含关系也视为相交；移除 polyclip 依赖 |

## 舍弃的设计及直接替代

| 旧设计 | 原因 / 使用方式 |
| --- | --- |
| `str.Bytes` 的 unsafe 零拷贝可写字节切片 | 对字符串底层存储的可写别名有风险；使用 `[]byte(s)` |
| `str`、`g`、`types` 大量标准库别名、重复类型和万能入口 | 隐藏实际依赖，增加维护面；直接使用 `strings`、`slices`、`maps`、`encoding/*`、`time`、内置 `min/max` |
| `str.CutLeast` 的隐式 NUL 填充 | 文本截断和二进制定长填充语义不同；文本用 Truncate，协议中显式分配字节缓冲 |
| 正则邮箱/IP/密码/身份证的笼统校验、`str/err` 错误字符串包装 | 通用正则无法表达业务验证；地址用 netip/mail，模式用 regexp，错误用 errors.New/Is/As/Join 与 fmt.Errorf |
| 多份 HTTP、缓存、框架版本及链式 any 转换 | 新库只保留一个功能实现，调用方显式处理类型和错误 |
| 无上限 ReadAll、默认 0777、隐式日志输出、错误忽略和随意 recover | 文件与网络 I/O 有明确错误及边界；panic 保持 Go 的正常语义；日志交给调用方 |
| `oss/ctx` 自定义可变 context、`g` 无限计数通道/全局 Wait、旧协程池 | 使用 context.WithCancel/WithValue、time.Ticker、sync.WaitGroup；批量处理用 task.Map。旧 GoPool 在新建 worker 的分支没有传入当前任务，会丢任务 |
| `oss` 个人目录、固定桌面路径、启动目录推断与简单系统别名（UserDataDir 除外） | 用户数据路径用 oss.UserDataDir；其余使用 os.UserHomeDir/UserConfigDir/UserCacheDir/Executable、filepath 和 os/signal；产品特有路径由应用指定 |
| 内嵌 logo、字体、Swagger HTML、图标资源 `i` | 属于产品资源，不进入通用基础库 |

## 未纳入新库的模块

这些模块需要专门的依赖、平台或产品语义，继续保留在 `goutil`，以后有明确需求时可独立重构为集成库。这里没有对全部集成执行功能验证，也不将其标为不可用。

| 范围 | 决策理由 |
| --- | --- |
| `database/*`（MySQL、SQLite、MSSQL、Redis、InfluxDB、ORM） | 驱动与连接生命周期由应用管理；不为少数薄封装引入整组驱动 |
| `disk`、`oss/fss`、`other/upload`、`oss/docker` | 云存储、SMB、FTP、网盘、Docker 是独立集成，包含凭据及远程副作用 |
| `frame/*`（gin/gf/mux/in/router）、中间件、Swagger、pprof、HTTP 文件服务 | 应用框架及服务器契约不统一；避免再造一个综合框架或隐式开放服务 |
| `script`、`spider`、`other/chrome`、`config` | 脚本 VM、DSL、Selenium、浏览器、GUI 需要独立运行时和依赖边界 |
| `4g`、`oss/linux`、`oss/win`、`oss/share`、`oss/thread`、`oss/process`、`oss/tray`、`other/gamepad`、`other/keyboard`、`oss/nac` | 硬件、系统控制、共享内存和 GUI 多为平台特定；不复制未跨平台验证的代码 |
| `ffmpeg`、`oss/img`（含二维码/验证码）、`other/excel`、`str/str_transform.go` | 外部可执行程序、字体、图片算法、Excel、旧编码转换应独立引入对应工具库 |
| `notice`、`oss/license`、`other/idcard`、`other/dept`、`other/command` | 通知渠道、授权、行政区划、部门模型、Cobra 命令构造含业务约定；旧行政区划表和校验语义不能当作通用可信数据 |
| `net/proxy`、`net/p2p`、`net/stun`、IP ping、`other/m3u8`、`other/tempmail`、`net/net_tools` | 协议或公网服务集成，不能靠本地单元测试验证；TCP 探测通常直接用 net.Dialer.DialContext |
| `task.Cron`、`oss/watch`、`str/bar`、`cache/file_log`、`other/trunk` | 调度、监听、防抖、终端渲染、日志查询和发布订阅仍有价值，但运行/关闭/重入/背压契约需要单独设计；不直接搬入现有实现。它们不包含在这次选定的 12 个分包中 |
| `math/polygon` 的三角形、四边形和多边形裁剪 | 专用几何能力；核心库保留基础空间判断，不自写通用布尔裁剪算法 |

## 独立扩展：sqlitets

用户提供的 `sqlitets` 时序库纳入本项目目录，保留为独立子模块 `github.com/injoyai/util/sqlitets`。它提取自 edge 历史存储，提供时序数据契约、SQLite 实现和 UTC 月分库管理，不是旧 `goutil/database/*` 薄封装的迁入。

- 核心模块继续只依赖标准库；SQLite 扩展保留自己的 `go.mod`、`go.sum` 和现有驱动版本，不在根模块添加驱动依赖。
- 原 `sqlitets`、`sqlitets/sqlite`、`sqlitets/monthly` import 分别改为 `github.com/injoyai/util/sqlitets` 及其对应子包；存储 API、数据目录和磁盘 schema 保持不变。
- 本地接入必须单独 replace 子模块；根目录的 `go test ./...`、`go vet ./...` 和 `go build ./...` 不跨越嵌套模块边界，需进入 `sqlitets` 单独执行。模块边界遵循 [Go Modules Reference](https://go.dev/ref/mod#modules-packages-versions)。
- 子模块入口、资源所有权、查询及数字精度语义见 [sqlitets/README.md](sqlitets/README.md)。业务模型、保存开关、定时清理调度和 HTTP 仍由调用方负责。

## 接入注意事项

1. 按表替换实际需要的 import 和调用。无需整项目一次切换，也不要只替换模块路径后假定兼容。
2. `cache.NewCycle` 返回 error；`List(0)` 表示全部，`List(n)` 为最近 n 条。分页 `slice.Limit` 与输入共享存储。
3. 文件缓存可读取旧版本最终保存为普通 JSON 对象的内容；旧环形缓存的保存格式、特殊类型转换及自动加载约定不兼容。迁移前保留数据副本，按业务类型解码，不会自动覆写旧缓存。
4. `time.Time` 的构造与 Unix 单位换算使用标准库 `time.Unix/UnixMilli/UnixMicro`，保留具体时区和日历加减语义。
5. `task.Map` 错误通过 results[i].Err 和汇总 error 可见；`Retry` 不自动判定错误是否可重试。对于写入、付款或 POST 等操作，调用方明确决定重试策略。
6. 提供标准 `http.Client` 的 Proxy、Transport 和 CookieJar 配置。原来的任意格式 Bind、默认重试、自动 Cookie 写回和隐式调试打印不迁移。
7. 解压拒绝覆盖，默认将目录权限设为 0755、文件设为 0644，不恢复归档中的 executable/setuid 等权限。ZIP 目录打包的是目录内容，不再依赖源路径末尾 `/` 改变行为。
8. `github.com/injoyai/goutil/oss` 的 `New` 调用迁到本库 `oss.New`。不再通过 `conv.Bytes` 隐式转换数字、结构体或 map；JSON 使用 `oss.WriteJSON`，其他内容显式编码。内容传给目录路径时返回错误，不再静默丢弃；替换目标（包括符号链接本身）沿用 `oss.Write` 语义，不保留旧权限或硬链接关系。reader 不会自动关闭，失败后不会回退已消费的输入。未迁移 `NewNotExist` 的先判断再写入逻辑。
9. `github.com/injoyai/goutil/oss` 的 `UserDataDir` 调用迁到本库 `oss.UserDataDir` 并处理 error。与旧函数一样只返回路径，需要目录时显式调用 `oss.New(dir)`。Linux 从 `/var/lib` 改为 XDG 用户数据路径；Windows 尊重 LOCALAPPDATA 的重定向。不会查找或搬移旧位置的数据，接入已有应用时由调用方显式决定旧数据位置和迁移方式。平台规则与参考来源见根 README。

## 验证范围

回归测试覆盖缓存并发与快照、编码/解码错误、保存失败保留原文件、Unicode、分页溢出、uint64 大小边界、短 CSV 和 Flush 错误、HTTP 状态/正文限制/截断/取消、代理信任、任务并发/错误/退出、几何边界以及解压路径、符号链接、资源限制和 gzip CRC。

运行环境为 Go 1.27.1、macOS arm64；模块最低版本标为 Go 1.25（使用该版本可用的 os.Root 目录方法）。Linux/Windows 编译检查仅证明编译通过，不代替各平台实际 I/O 行为验证，也未在 Go 1.25 运行测试。
