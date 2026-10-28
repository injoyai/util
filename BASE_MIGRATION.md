# base → util 迁移

本次承接“保留有用能力、合并重复实现、标准库已有的不再包装”的迁移方向。`base` 原目录与使用它的其他项目保持原样；此文档覆盖整个 base 的分类去向，不表示所有旧 API 都在 util 保留。核心模块不依赖 base、conv 或其他第三方模块，最低 Go 版本仍为 1.25。

## 已迁入的独有能力

下表的“去除”指 util 不再提供的接口或行为；base 原库未删除。仅改名的接口单独标明。

| base | util | 行为与接入变化 | 去除或未迁入的内容 |
| --- | --- | --- | --- |
| `maps.NewGeneric[K,V]` | `maps.NewGeneric[K,V]` | 保留 `Get/Set/Has/Del`、TTL、`GetAndDel/GetAndSet/GetOrSet`、按 key 去重的 `GetOrSetByHandler/GetOrSetByHandler2`、快照 `Range/Map`、`Clone`、过期清理及事件 | `Exist`、`MustGet`、`GetVar`、`GMap`、`conv.Extend` 隐式转换；对外的 `WithMutex`、`Value/NewValue` 包装 |
| `maps.GetOrSetByHandler` | `maps.GetOrSetByHandler2` | 接受 `func() (V, error)`，返回 `(V, error)`；后缀 2 表示两个返回值 | 无；仅改名为双返回值版本 |
| `maps.GetOrSetByHandler2` | `maps.GetOrSetByHandler` | 接受 `func() V`，直接返回 V；复用带 error 版本的缓存、过期和按 key 加锁逻辑；nil 工厂仅读取缓存，缺失返回零值 | 无；仅改名为单返回值版本 |
| `maps.Subscribe/Chan` | `maps.Subscribe(key, capacity)` | 返回 `(*chans.Safe[V], error)`；只保留 Subscribe 名称；通知满时丢弃新消息 | `Chan` 方法别名；省略容量的可选参数形式 |
| `maps.RunClear/Close` | 同名接口 | Close 取消并等待自动清理退出，关闭订阅但保留数据；关闭后不重启清理，仍可访问 Map，后续新订阅立即关闭 | 无；保留原有公开接口 |
| `maps.NewBit` | 零值可用的 `maps.Bit` | `Set` 不返回重试次数耗尽错误；`Del` 清位但保留分组，避免并发删除分组时丢失其他置位 | `NewBit` 构造器、旧 `Bit` 接口、`Set` 的 error 返回值，以及清空分组时自动删除分组的行为 |
| `maps/wait.NewGeneric` | `maps/wait.NewGeneric` | 保留 `Wait/Sync/Async/Done/IsWait/SetTimeout/SetReuse`，移除可变全局默认实例和无效的 SetClearNum；增加 Close | 包级 `Default/Wait/Sync/Async/Done/SetTimeout/SetReuse/IsWait`、无类型 `New/Entity` 和无效的 `SetClearNum` |
| `maps/keep.New` | `maps/keep.New` | 保留最近六次心跳及锁外回调，增加 Del；Keeping 的次数必须为 1～6，历史不足返回 false | `Keeper` 聚合接口；次数超出 1～6 时不再沿用旧的截断或空循环判断 |
| `maps/timeout.NewGeneric` | 同名接口 | 保留 Keep/Del、超时配置、错误后重试以及 `Run(ctx)`；Run 的协程与等待由调用方管理 | 无；保留原有公开接口 |
| `safe.Closer` | `safe.Closer` | 保留 `NewCloser/NewCloserErr`、`Done/Err/Closed`、`Close/CloseWithErr`、`SetCloseFunc/Reset` 全部接口，适合嵌入资源对象；关闭回调在锁外执行 | 无；保留原有公开接口和行为 |
| `chans.NewSafe`、`safe.NewChan` 的安全关闭意图 | `chans.NewSafe[T](capacity)` | 返回对象和 error；发送用 Send(ctx, value)、Add、Try、Timeout；Chan 只读，Close 解除阻塞发送并关闭接收端；不嵌入可重置 Closer | `Must` 发送捷径、公开可写的 `Chan/C`；`SetCloseFunc/Reset/Err/CloseWithErr` 从通道对象移除，独立的 safe.Closer 仍完整提供 |
| `chans.NewSubscribe` | `chans.NewSubscribe` | Subscribe(topic, capacity) 返回对象和 error；Publish(topic, value, timeout) 显式指定每个接收者的超时；Close 解除阻塞发布 | `Publish` 的隐式默认超时、`Subscribe` 的可选容量参数形式；方法本身保留 |
| `chans.NewIO`、通道 Writer 的可关闭流意图 | `chans.NewIO(capacity)` | 返回对象和 error；实现 io.ReadWriteCloser，复制写入，关闭后排空缓冲；ReadBytes 先返回 Read 未读完的余量；去掉 ReadMessage 别名 | `ReadMessage` 别名；独立的 `MustWriter/TryWriter` 通道类型及 `WriteTimeout` 捷径 |
| `chans.NewQueueCover`、Last 的保留最新消息意图 | `chans.NewQueueCover[T](capacity)` | 返回对象和 error，capacity 必须正数；Append 返回是否接受，满时移除最早一条；Pop(ctx) 支持取消，Close 后排空再返回 io.EOF；容量 1 可保留最新一条 | `PopMust/PopTimeout/PopTry`、可直接发送或关闭的通道类型、`Last` 类型及其后台 Entity |
| `chans.NewWaitOnce` | `chans.NewWaitOnce` | 独立实例、并发 Done 只接受第一次结果；Wait(ctx) 返回值和 error，所有等待者共享首个结果；不再将结果作为只能消费一次的缓冲消息 | 可直接发送或关闭的通道类型、无 context 的 `Wait()`、可省略结果的 `Done()`，以及结果只能消费一次的行为 |
| `safe.Runner/Runner2` | `task.NewRunner` | 合并为一套 Start(ctx)/Run(ctx)/Stop/Done/Running/Err；构造返回 error；重复启动返回 ErrRunning；Stop 取消并等待退出；后台错误可由 Err 获取，不自动恢复 panic | 独立 `Runner2/NewRunner2`、`NewRunnerWithContext`、`SetFunc/Restart`、可不等待退出的 Stop 参数，以及自动 recover |
| `safe.OneRun`、`chans.Rerun` 的替换运行意图 | `task.NewOneRun` | 每次 Run(ctx) 取消并等待上次执行，再启动本次；构造返回 error；Stop 取消并等待；处理函数在构造时固定，不再有 SetHandler 和隐藏常驻协程 | `SetHandler`、旧 `Close` 名称、`Rerun/Enable/Disable/Enabled` 配置接口和内部常驻协程；关闭改用 Stop |
| `safe.NewPool` | `pool.New[T](capacity, factory)` | 返回对象和 error；只限制缓存的闲置数量，满时放弃复用但不修改被丢弃对象；归还后的对象不可再使用，不自动清理对象内容 | 池满时将对象内容清零的副作用 |
| `crypt/crc` | `crc` | 保留 Param8/Param16、预设参数、MakeTable、Init/Update/Complete/Checksum；去掉 Encrypt 命名及字符串/HEX/Base64 包装，调用方选择编码和字节序 | `Encrypt8/Encrypt16/Encrypt16String/Encrypt16HEX/Encrypt16Base64` 便捷入口 |

Map 的 Range 在锁外遍历结构快照，元素内部的指针不做深拷贝。Clone 的过期元数据独立，事件、订阅和清理协程不复制。`Clear` 沿用旧含义，仅清除过期项；不会清空有效数据。`GetOrSetByHandler` 和 `GetOrSetByHandler2` 都在同 key 锁内执行工厂，工厂不得递归请求同 key；普通 Set 与工厂并发时，已存入的有效值优先。这两个方法的名称与 base 对调，接入时须按返回值数量选择。

Map 的过期时间不大于零表示永久。GetOrSet 及两个 handler 版本返回本次选定或生成的值，即使返回前已过期也不返回零值；TTL 影响之后的缓存读取。事件回调在锁外同步调用，不保证并发操作的回调顺序；回调负责自身同步。Close 只管理清理协程与订阅，不等待调用方正在执行的 Map 方法。后台自动清理至多启动一次。

Wait 的 Sync 非正超时立即返回 `context.DeadlineExceeded`，超时包含同 key 排队；SetReuse 只影响之后注册的同步等待者。Async 的 num 为正时限制次数，为负时持续注册，为零或 nil 回调时不注册；非正超时表示不自动过期。回调在 Done 调用方的锁外执行，多个 Done 可并发执行回调。Close 唤醒同步等待者并返回 ErrClosed、停止回调定时器，但不等待已经被 Done 取出的回调。

Safe 的 Add 未传超时或超时为负时阻塞，零为尝试发送，正值限制等待。Send 接受 context，关闭返回 io.ErrClosedPipe。发送、关闭或取消同时发生时可能完成发送；Close 返回后新发送一定失败，已有缓冲仍可读取。接收方不能自行 close Chan。QueueCover 关闭后 Append 返回 false。

IO 在关闭时先解除阻塞写入，等在途写入结束后关闭消息通道；Read/ReadBytes 排空已接受的消息后才返回 EOF。心跳超时处理会在每次回调前复核当前记录和配置，跳过已刷新或删除的旧快照；已确认的回调在锁外执行，后续 Keep 不会撤回它。

Runner 的 Done 在空闲时已关闭，Start 后指向本轮的退出信号。Stop 等待当前一轮，不禁止未来 Start/Run；OneRun 同样允许停止后再次运行。并发 Stop 与启动操作不构成永久关闭屏障。处理函数必须响应 context，否则停止或替换会等待；禁止在处理函数中等待自身退出。

`safe.Closer` 沿用 base 的契约：必须通过构造函数创建；每轮只有首次关闭会记录原因并执行回调，CloseWithErr(nil) 不关闭。Close 返回回调的 error，Err 保存的是关闭原因。Done 在回调前关闭，并发重复 Close 不等待回调；需要等资源清理完成时由调用方同步。NewCloserErr(nil) 也表示已关闭。Reset 清除状态和原因、保留回调，并更换 Done；旧 Done 若仍打开则关闭，不触发回调。重置时调用方须协调旧资源和回调的退出；此类型只同步自身状态，不管理任意外部资源的并发访问。

Bit 的分组在清位后保留，适用于键空间相对稳定的状态标志，不适用于无限增长的临时 ID。Keep 中不再需要的 key 应显式 Del。CRC 参数表创建后只读，预设参数变量也应只读使用；Init/Update/Complete/Checksum 要求传入 MakeTable 返回的非 nil 表。CRC 不是加密或身份认证。

## 与 util 现有能力合并

| base | 使用方式 |
| --- | --- |
| `str.Reverse/HasRepeat/CropFirst/CropLast` | util/str 同名函数；裁剪必须显式传 retain |
| `str.Rand`、`g.RandString` | `str.Random(length, alphabet)`，空 alphabet 使用默认字母数字，处理 error；长度按 Unicode 码点 |
| `str.GetSplit/GetLine/MustSplitN` | `str.SplitAt`，按行用换行符分隔；显式处理 `(value, ok)`，支持负索引 |
| `str.FindCommon`、`types.List.Get/Limit` | `slice.Common/Get/Limit`；索引和分页按新签名接入，Limit 为共享视图 |
| `safe.Retry`、`chans.Coroutine/LimitGo/WaitLimit` 的批量处理意图 | `task.Retry/Map`；Retry 次数为额外重试次数，Map 等待启动的处理函数退出并汇总错误，不恢复 panic |
| `crypt/gzip` | `archive.Gzip/Gunzip`，流式处理；解压必须显式限制输出大小 |

## 标准库直接替代，不增加重复包装

| base | 替代方式与注意事项 |
| --- | --- |
| `str` 中 strings 的别名 | 直接导入 strings；Pointer 用局部变量取地址，Bytes 用 `[]byte(s)` 复制 |
| `types.Bytes/Float32/Float64` | bytes、encoding/binary、hex、base64、strconv、math.Float32bits/Float64bits；数字解析显式处理 error，不依赖 any 转换 |
| `types.List/Map/Maps` 的排序、克隆、过滤、删除、分块等 | 原生切片、map、slices、maps；分块用 slices.Chunk，需要切片集合时用 slices.Collect；过滤显式循环，避免重新引入容器包装 |
| `types.Reuse/Memory` | 明确分配切片，用 copy/clear/切片表达式管理有效长度；需要闲置对象复用时用 util/pool |
| `types.Chan`、Signal、Counter、Limit、Order 的基本同步意图 | 原生 chan、context、sync.Once、sync.WaitGroup；信号量用带缓冲通道，动态工作调度按应用处理；不保留容易绕过安全发送/关闭的公开可写通道别名 |
| `safe.Value/Int64/Uint64/Bool/Use/Once` | sync/atomic、sync.Once；注意旧 safe.Once 是执行前占用标记，不保证并发调用等待 fn 完成，不能盲目等同 sync.Once |
| `safe.Goroute`、并发计数和批量执行 | sync.WaitGroup 与 context，或 util/task.Map；取消后显式等待退出，不保留吞 panic 的全局入口 |
| `types.Err/Debugger/Option`、只为聚合而定义的接口 | errors、普通 bool、应用自己的配置和最小接口；旧 Err.Is 按错误文本相等，新代码不能假定 errors.New 有相同身份 |
| `coding` 的 JSON/XML/Base64/HEX/ASCII | encoding/json、encoding/xml、encoding/base64、encoding/hex 及 string/[]byte 转换；旧 ASCII 实际只是字节与字符串转换，不做 ASCII 校验 |
| `crypt` 的 hash/HMAC、MD5/SHA | hash、crypto/hmac、crypto/md5、crypto/sha*；编码使用 hex/base64；直接调用标准算法，避免可变链式状态 |
| `crypt/tls` | crypto/tls、crypto/x509，显式构造配置、加载证书和 CA，保持证书及主机名校验 |

## 保留在 base，未迁入的范围

| 范围 | 理由 |
| --- | --- |
| `coding` 的 YAML/TOML/Msgpack/Protobuf、`coding/json` 的 jsoniter 适配、`str` 的 GBK/GB18030/HZGB2312 | 都涉及第三方实现或兼容语义；应用直接选用所需依赖，不为迁移增加核心依赖，也不将它们放进可绕过模块边界的目录 |
| `types.Price/Yuan`、Message 的请求响应约定、Bytes 的 DLT645/加减 0x33/协议数值解析 | 精度、缩放、错误码和设备协议属于业务契约，不能直接用通用类型替换；原调用方继续保留 base，待对应项目明确迁移 |
| `crypt/aes` 的 CBC/ECB/PKCS7、`crypt/des` | 旧密文兼容和 IV/填充语义需要由应用确定；本次不复制默认 IV、旧格式和无认证加密接口，不自动转换或重写已有密文 |
| `safe.Rerun` 的 Dial/Run/Close 重连流程 | 外部连接与关闭责任由具体客户端管理；可组合 task.Runner/Retry，但不是等价的只改 import 替换 |
| `chans.Entity/QueueFunc/Trunk/Coroutine` 的动态 worker、回调总线及配置接口；Order 的可动态扩展循环执行 API | task.Map 覆盖有限批量并发，chans.Subscribe 覆盖按主题广播；动态调整 worker 数、循环轮转、任务编号和每个回调的调度契约未直接搬入，旧调用方保留原实现 |
| `maps` 的 conv.Extend/GetVar/GMap、无类型默认实例、MustGet、WithMutex/Value 对外包装 | 移除隐式转换与重复入口；使用类型明确的 Generic.Get、GetOrSetByHandler/GetOrSetByHandler2、Map；普通原生 Map 快照可交给调用方做显式转换 |
| 捕获并吞掉 panic 的 Recover/Try 与无效兼容参数 | util 保留 Go 的 panic 语义；调用方在真正需要隔离的边界自行定义错误策略 |

这些范围没有从旧仓库删除，其他项目的 go.mod、导入和业务数据均未修改。接入时应按接口逐项迁移，不应全局替换 `github.com/injoyai/base` 前缀。

## 验证

迁移测试保留原有响应队列、超时心跳与 CRC-16 固定向量，并覆盖 Map 原子操作、按 key 工厂去重、回调重入、克隆过期隔离、清理关闭、并发位图、订阅/发送/关闭竞争、消息部分读取和排空、覆盖队列、一次性结果广播、对象池复用以及任务替换和退出等待。CRC-8 全部预设参数和 CRC-8/16 分段计算也有固定向量。

Closer 测试覆盖首次原因与回调错误分离、并发关闭只回调一次、回调重入、Reset 的旧通知和回调保留、nil 原因及已关闭构造行为。

```sh
go test -race ./maps/... ./chans ./crc ./pool ./task ./safe
go test -race ./...
go vet ./...
go build ./...
```

根模块检查不包含独立 sqlitets 子模块；本次没有修改它。跨平台 build 仅检查编译，不能代替运行验证。CRC 源码经 base 迁入，已对照 [sigurn/crc8](https://github.com/sigurn/crc8) 与 [sigurn/crc16](https://github.com/sigurn/crc16) 确认上游来源并保留各自 MIT 许可，见 crc 目录下的 LICENSE 文件。
