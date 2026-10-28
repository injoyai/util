# sqlitets

基于 SQLite 的嵌入式 Go 时序库，提取自 edge 项目的历史存储。支持字符串、布尔、精确 JSON 数字、null、对象和数组，以及 UTC 月分库、跨月查询、过滤、分页、聚合和过期清理。直接嵌入 Go 程序，不需要独立数据库服务。

## 本地引用

要求 Go 1.25 或更高版本。本目录是 `util` 项目中的独立 Go 子模块，模块名为 `github.com/injoyai/util/sqlitets`，保留已有的 `github.com/glebarez/go-sqlite` 驱动依赖。核心 `github.com/injoyai/util` 模块仍只依赖标准库。

本项目尚未发布。在其他 Go 项目中执行，将示例绝对路径替换为实际位置：

```bash
go mod edit -require=github.com/injoyai/util/sqlitets@v0.0.0
go mod edit -replace=github.com/injoyai/util/sqlitets=/absolute/path/to/util/sqlitets
go mod tidy
```

对子模块的本地 `replace` 需要单独配置，仅替换核心模块路径不会替换本子模块。原先使用 `sqlitets`、`sqlitets/sqlite` 或 `sqlitets/monthly` 的调用方需改用新的完整 import 路径；API 和磁盘 schema 保持不变。

## 使用

```go
import (
    "context"
    "time"

    "github.com/injoyai/util/sqlitets"
    "github.com/injoyai/util/sqlitets/sqlite"
)

store, err := sqlite.Open("./data", map[string]string{
    "readings": "sensors",
})
if err != nil {
    return err
}
// 在所属服务退出时调用 store.Close()，并处理返回的错误。

err = store.Write(sqlitets.Point{
    Measurement: "readings",
    Time: time.Now(),
    Fields: map[string]any{
        "temperature": 23.5,
        "humidity": 60.0,
    },
})
if err != nil {
    return err
}

ctx := context.Background()
rows, err := store.Query(ctx, sqlitets.Query{
    Selector: sqlitets.Selector{
        Measurement: "readings",
        Start: time.Now().Add(-time.Hour),
    },
    Fields: []string{"temperature"},
    Aggregate: sqlitets.AggregateMean,
    Interval: 5 * time.Minute,
    Limit: 100,
})
// 处理 rows 和 err。
```

可执行、带输出断言的完整示例见 `sqlite/example_test.go`，通过 `go test ./...` 运行。

写入统一接收 `Point`，与查询、计数和删除的参数对象保持一致。`store.Write(point)` 无需传 Context，内部使用 `context.Background()`；需要取消或超时控制时使用 `store.WriteContext(ctx, point)`。查询、计数和删除继续显式接收 Context。

`Point` 包含 `Measurement`、`Time` 和 `Fields` 三个字段。存储格式和同时间字段合并规则不变。

注册映射的键是逻辑 measurement 名，值是物理分片目录。目录名必须匹配 `[a-z][a-z0-9-]*`，不同 measurement 不得指向同一目录。注册表在创建时复制，之后不随调用方修改而变化；没有隐式默认 measurement。

上面的数据存放在 `data/sensors/YYYY-MM.db`。每个分片内使用：

```sql
CREATE TABLE points (
    time_ns INTEGER PRIMARY KEY,
    fields_json TEXT NOT NULL
) WITHOUT ROWID;
```

## 查询和数据语义

- `Measurement + Time` 唯一标识一个数据点。相同纳秒再次写入时合并字段；同名字段由后写值覆盖。不存在独立 tags 或设备 ID 维度，字段名由调用方组织。
- 写入时间不得为零，字段不得为空，值必须能通过标准 JSON 编码。原始查询的所有数字（含嵌套值）统一返回 `json.Number`，保留整数与十进制数字文本的精度；合并同时间的旧字段时也保留数字文本。调用方已经转为浮点数并丢失的精度无法恢复。
- 写入时间及非零的 Start/End/At 必须能由 int64 Unix 纳秒精确表示。越界会在访问分库前返回错误，避免 UnixNano 溢出后存成另一日期；零 Start/End 仍表示无界，零 At 仍无效。此校验不会还原此前已经溢出错写的日期，既有数据需结合原始记录核查。
- 对象、数组读回为 `map[string]any`、`[]any`；结构体按 JSON 标签编码，`time.Time` 编码为时间字符串，`[]byte` 编码为 Base64 字符串。不会隐式恢复原始 Go 类型，不支持函数、channel、循环引用、NaN 和 Infinity。
- `Start`、`End` 都是排他边界，即 `Start < time < End`；零值表示不限制该端。`At` 按纳秒精确匹配，和时间范围、字段过滤共同生效。
- 字段过滤支持相等、字符串包含、集合成员匹配，多个条件为 AND。过滤值在扫描前转换为与存储相同的 JSON 表示，不修改调用方的值；数组按顺序、对象按键递归比较。数值比较不先转为浮点数，`7` 与 `7.0` 相等，但与字符串 `"7"` 不相等。缺失字段不同于显式 null。
- `Contains` 要求 JSON 字符串；`In` 要求非 null 的 JSON 数组（空数组不匹配任何值，nil 切片编码为 null 会被拒绝）。所有过滤条件、时间范围与精确时间参数均在扫描前校验，空库与有数据时行为一致。
- 原始查询使用 `Fields` 选择字段，或用 `AllFields` 读取全部字段；二者不能同时指定。聚合要求显式字段，支持 mean、max、min、sum、count。
- 聚合仅处理数值，不将字符串和布尔值转为数字。`Count` 统计指定字段的非 null 点数，`AggregateCount` 统计数值点数，二者含义不同。聚合计数返回 `int64`；其他聚合返回 `float64`，不提供精确十进制或大整数运算。参与数值聚合的值超出 float64 范围，或 sum/mean 的中间和溢出时返回错误；计数不受浮点范围限制。
- 正时间戳的固定时长桶从 Unix epoch 对齐；不提供本地自然日、自然月或时区日历聚合。无 `Interval` 的聚合合并整个范围，结果时间为 Unix epoch；不自动填充空桶。
- 分片按 UTC 月份划分，跨月排序、分页和聚合统一计算。`Limit = 0` 返回全部结果；`Offset > 0` 必须同时指定正数 Limit。
- 查询、计数和按条件删除先根据时间范围及 At 筛选月份，再打开相关分库。无关月份的损坏不会阻断有明确时间选择的操作；访问到的相关分库若损坏，仍正常返回错误。
- 查询内部按序扫描并在页满后停止，但最终返回切片。长时间范围、字段过滤、大 Offset 和无界结果仍有扫描或内存成本。
- `Delete` 必须带时间或过滤条件；仅指定 `End` 的过期清理优先删除整月文件，再删除边界月中过期的行。跨月操作不提供跨文件事务。

## 按指定类型解码

原始查询可显式读取数字文本或转换数值，并检查转换错误：

```go
number := rows[0].Fields["sequence"].(json.Number)
text := number.String() // 精确的原始 JSON 数字文本
```

也可用 `Row.DecodeFields` 将整个字段对象解码到指定结构：

```go
var reading struct {
    Sequence uint64    `json:"sequence"`
    At       time.Time `json:"at"`
    Payload  []byte    `json:"payload"`
}
if err := row.DecodeFields(&reading); err != nil {
    return err
}
```

类型转换遵循标准 JSON 规则：结构体中的整数字段可精确恢复，溢出或类型不兼容会返回错误；接口字段中的数字仍保留为 `json.Number`。未声明的 JSON 字段被忽略，缺失字段遵循标准库对目标已有值的处理。`Row.Time` 不参与字段解码。

与最初提取版本相比，原始查询数字的返回类型从 `float64` 改为 `json.Number`，需要同步更新类型断言；JSON 序列化结果仍是数字，磁盘 schema 不变。此前已发生的精度损失不会由升级自动修复。

## 资源与共享分库

`sqlite.Open` 创建并拥有分库管理器，`Close` 关闭全部句柄。业务调用方应先停止生产者、等待在途任务，再关闭存储。

需要与固定结构记录共用分库管理器时，通过 `monthly.New(root)` 创建，再调用 `sqlite.Use(shards, measurements)`。`Use` 借用管理器，其 `Close` 不关闭共享资源；由管理器所有者统一执行 `shards.Close()`。各使用方必须协调目录注册，避免相同目录承载不兼容的 schema。

`monthly` 提供 schema 注册、分片回调、遍历、过期清理和固定记录分页。`Walk` 保留全量遍历行为；`WalkFiltered` 接受基于 UTC 月首时间的筛选函数，在打开数据库之前决定是否访问该月，nil 表示全量遍历。筛选函数应及时返回，不得在回调中关闭同一个管理器。

`Page` 面向包含 `id` 及时间列的记录表，Columns、Where 和 Schema 应来自应用代码，值使用参数绑定。PageNum 从零开始，PageSize 为零时返回全部并忽略 PageNum；负值或页偏移乘积溢出返回错误。DeleteBefore 的非零截止时间也必须能由 int64 Unix 纳秒精确表示。

每个分片最多一个数据库连接；最多缓存 8 个闲置分片，活动回调中的分片不会被淘汰。连接使用 WAL、5 秒 busy_timeout、synchronous=NORMAL，空闲一分钟可回收。多进程同时访问与高写入负载尚未验证。

## edge 存储格式兼容

使用以下映射可读取原 edge 时序月库，无需修改表结构或移动数据：

```go
store, err := sqlite.Open(root, map[string]string{
    "device_nature_log": "device",
    "system_nature_log": "system",
    "user_nature_log":   "user",
})
```

`root` 为原来的 `data/database`。库不包含 edge 的保存开关、点位模型、`_count_` 约定、定时清理调度、HTTP、文件导出或旧目录迁移。

## 验证

```bash
# 在本子模块目录执行；根模块的 go test ./... 不包含本目录。
go vet ./...
go test -race ./...
go build ./...
```

测试使用临时目录，覆盖跨月查询、聚合、分页、清理、连接淘汰、注册校验、资源所有权、原 edge schema 的双向读写兼容，以及大整数合并/重开、递归过滤、类型解码、非法参数与聚合越界。边界回归覆盖跳过无关损坏分库、纳秒时间的有效端点与越界拒绝，以及页偏移溢出。兼容性测试使用合成数据库，不访问生产数据。
