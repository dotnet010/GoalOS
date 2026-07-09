# Google Go Style Guide - 完整整合版

> **来源**: https://google.github.io/styleguide/go/best-practices  
> **整合日期**: 2026-07-08  
> **说明**: 本文档将 Google Go Style Guide 系列文档（Overview、Guide、Decisions、Best Practices）整合为单一文件。所有被引用的关联文档内容已内联到相应位置，确保单文件即可完整理解所有规范。

---

## 目录

1. [概述](#1-概述)
2. [风格原则](#2-风格原则)
3. [核心规范](#3-核心规范)
4. [命名规范](#4-命名规范)
5. [包大小与组织](#5-包大小与组织)
6. [导入规范](#6-导入规范)
7. [错误处理](#7-错误处理)
8. [文档规范](#8-文档规范)
9. [变量声明](#9-变量声明)
10. [函数参数列表](#10-函数参数列表)
11. [命令行接口](#11-命令行接口)
12. [测试规范](#12-测试规范)
13. [字符串连接](#13-字符串连接)
14. [全局状态](#14-全局状态)
15. [接口设计](#15-接口设计)
16. [语言规范](#16-语言规范)
17. [常见库](#17-常见库)

---

## 1. 概述

Google Go Style Guide 系列文档包含以下四个部分：

| 文档 | 主要受众 | 规范性 | 权威性 |
|------|---------|--------|--------|
| **Style Guide（指南）** | 所有人 | 是 | 是 |
| **Style Decisions（决策）** | 可读性导师 | 是 | 否 |
| **Best Practices（最佳实践）** | 任何感兴趣的人 | 否 | 否 |
| **本文档（整合版）** | Go 开发者 | 是 | 是 |

本文档是 **Best Practices** 的完整整合版，已将 **Guide** 和 **Decisions** 中被引用的相关内容内联到对应位置，形成单一、自包含的编码规范文档。

---

## 2. 风格原则

可读代码应具备以下属性，按重要性排序：

### 2.1 清晰性 (Clarity)

代码的目的和原理对读者来说应该是清晰的。

- **代码在做什么？** 通过有效的命名、有益的注释和高效的代码组织来实现。
- **代码为什么这样做？** 当代码包含读者可能不熟悉的细微差别时（如闭包捕获循环变量、业务逻辑中的访问控制检查），需要添加注释解释。

> 重要：代码应该易于阅读，而不是易于编写。注释应该解释"为什么"，而不是"做什么"。

### 2.2 简洁性 (Simplicity)

Go 代码应该以对使用者、阅读者和维护者来说最简单的方式编写。

- 从上到下易于阅读
- 不假设读者已经知道代码在做什么
- 没有不必要的抽象层
- 名称不引人注目
- 值和决策的传播对读者来说是清晰的
- 注释解释"为什么"而不是"做什么"
- 文档独立完整
- 有用的错误和测试失败信息

**最小机制原则**：当有几种方式表达相同的想法时，优先使用最标准的工具。
1. 优先使用核心语言结构（channel、slice、map、loop、struct）
2. 其次使用标准库工具（HTTP client、template engine）
3. 最后考虑 Google 代码库中的核心库

### 2.3 简洁性 (Concision)

简洁的 Go 代码具有高信噪比。

- 避免重复代码
- 避免多余的语法
- 避免不透明的名称
- 避免不必要的抽象
- 合理使用空白

### 2.4 可维护性 (Maintainability)

代码被编辑的次数远多于被编写的次数。

- 易于未来的程序员正确修改
- API 结构能够优雅地扩展
- 明确假设，抽象映射到问题结构而非代码结构
- 避免不必要的耦合，不包含未使用的功能
- 全面的测试套件确保承诺的行为被维护

### 2.5 一致性 (Consistency)

一致的代码在更广泛的代码库中看起来、感觉上和表现上都相似。

一致性关注不应覆盖上述原则，但如果必须打破平局，通常有利于一致性。

---

## 3. 核心规范

### 3.1 格式化

所有 Go 源文件必须符合 `gofmt` 工具输出的格式。生成的代码通常也应该格式化。

### 3.2 MixedCaps

Go 源代码使用 `MixedCaps` 或 `mixedCaps`（驼峰式），而不是下划线（蛇形命名）。

- 常量：`MaxLength`（导出）或 `maxLength`（未导出）
- 不适用 `MAX_LENGTH` 或 `max_length`

### 3.3 行长度

Go 源代码没有固定的行长度限制。如果一行感觉太长，优先重构而不是拆分。如果已经尽可能短了，允许保持长行。

不要拆分行：
- 在缩进变化之前（如函数声明、条件语句）
- 为了让长字符串（如 URL）适合多行

### 3.4 命名

Go 中的命名倾向于比其他语言更短，但遵循相同的一般准则：
- 使用时不应感觉重复
- 考虑上下文
- 不重复已经清楚的概念

更具体的命名指导见 [命名规范](#4-命名规范) 部分。

### 3.5 局部一致性

当风格指南对某个特定风格点没有说明时，作者可以自由选择他们喜欢的风格，除非附近代码（通常在同一个文件或包内）已经对此采取了统一的立场。

**有效的局部风格考虑**：
- 使用 `%s` 或 `%v` 格式化打印错误
- 使用缓冲通道代替互斥锁

**无效的局部风格考虑**：
- 代码行长度限制
- 使用基于断言的测试库

---

## 4. 命名规范

### 4.1 函数和方法命名

#### 4.1.1 避免重复

选择函数或方法名时，考虑名称将被阅读的上下文：

- 以下信息通常可以从函数和方法名中省略：
  - 输入和输出的类型（当没有冲突时）
  - 方法接收器的类型
  - 输入或输出是否为指针
- 函数不要重复包名
- 方法不要重复接收器名
- 不要重复作为参数传递的变量名
- 不要重复返回值的名和类型

```go
// Bad:
package yamlconfig
func ParseYAMLConfig(input string) (*Config, error)

// Good:
package yamlconfig
func Parse(input string) (*Config, error)
```

```go
// Bad:
func (c *Config) WriteConfigTo(w io.Writer) (int64, error)

// Good:
func (c *Config) WriteTo(w io.Writer) (int64, error)
```

```go
// Bad:
func OverrideFirstWithSecond(dest, source *Config) error

// Good:
func Override(dest, source *Config) error
```

```go
// Bad:
func TransformToJSON(input *Config) *jsonconfig.Config

// Good:
func Transform(input *Config) *jsonconfig.Config
```

当需要区分相似名称的函数时，可以包含额外信息：

```go
// Good:
func (c *Config) WriteTextTo(w io.Writer) (int64, error)
func (c *Config) WriteBinaryTo(w io.Writer) (int64, error)
```

#### 4.1.2 命名约定

- 返回某物的函数使用名词式名称：
```go
// Good:
func (c *Config) JobName(key string) (value string, ok bool)
```
  推论：函数和方法名应避免 `Get` 前缀。
```go
// Bad:
func (c *Config) GetJobName(key string) (value string, ok bool)
```

- 执行某事的函数使用动词式名称：
```go
// Good:
func (c *Config) WriteDetail(w io.Writer) (int64, error)
```

- 仅在类型不同的相同函数，在名称末尾包含类型名：
```go
// Good:
func ParseInt(input string) (int, error)
func ParseInt64(input string) (int64, error)
func AppendInt(buf []byte, value int) []byte
func AppendInt64(buf []byte, value int64) []byte
```
  如果有明确的"主要"版本，该版本可以省略类型：
```go
// Good:
func (c *Config) Marshal() ([]byte, error)
func (c *Config) MarshalText() (string, error)
```

### 4.2 测试辅助包和测试替身命名

测试替身可以是 stub、fake、mock 或 spy。

#### 4.2.1 创建测试辅助包

为另一个包创建测试替身包时，安全的选择是在原包名后追加 `test`：

```go
// Good:
package creditcardtest
```

#### 4.2.2 简单情况

如果只有一个类型需要测试替身，可以使用简洁的命名：

```go
// Good:
import (
    "path/to/creditcard"
    "path/to/money"
)

// Stub stubs creditcard.Service and provides no behavior of its own.
type Stub struct{}

func (Stub) Charge(*creditcard.Card, money.Money) error { return nil }
```

这比 `StubService` 或 `StubCreditCardService` 更可取，因为包名已经暗示了 `creditcardtest.Stub` 是什么。

#### 4.2.3 多种测试替身行为

当一种 stub 不够时，根据行为命名：

```go
// Good:
// AlwaysCharges stubs creditcard.Service and simulates success.
type AlwaysCharges struct{}
func (AlwaysCharges) Charge(*creditcard.Card, money.Money) error { return nil }

// AlwaysDeclines stubs creditcard.Service and simulates declined charges.
type AlwaysDeclines struct{}
func (AlwaysDeclines) Charge(*creditcard.Card, money.Money) error {
    return creditcard.ErrDeclined
}
```

#### 4.2.4 多种类型的测试替身

当包中有多个值得创建替身的类型时，使用更明确的命名：

```go
// Good:
type StubService struct{}
func (StubService) Charge(*creditcard.Card, money.Money) error { return nil }

type StubStoredValue struct{}
func (StubStoredValue) Credit(*creditcard.Card, money.Money) error { return nil }
```

#### 4.2.5 测试中的局部变量

在测试中引用替身时，选择最能区分替身与其他生产类型的名称：

```go
// Good:
var spyCC creditcardtest.Spy
proc := &Processor{CC: spyCC}
```

### 4.3 下划线

Go 名称通常不应包含下划线。有三个例外：
1. 仅由生成代码导入的包名可以包含下划线
2. `*_test.go` 文件中的 Test、Benchmark 和 Example 函数名可以包含下划线
3. 与操作系统或 cgo 交互的低级库可以重用标识符（如 `syscall`）

### 4.4 包名

Go 包名应短且只包含小写字母。多词包名应保持完整且全部小写。

- `tabwriter` 而非 `tabWriter`、`TabWriter` 或 `tab_writer`
- 避免选择可能被常用局部变量名遮蔽的包名（如 `usercount` 比 `count` 好）
- 避免无信息量的包名如 `util`、`helper`、`common`、`models`

### 4.5 接收器名

接收器变量名必须：
- 短（通常一到两个字母）
- 是类型本身的缩写
- 对该类型的每个接收器一致应用

| 长名称 | 更好的名称 |
|--------|-----------|
| `func (tray Tray)` | `func (t Tray)` |
| `func (info *ResearchInfo)` | `func (ri *ResearchInfo)` |
| `func (this *ReportWriter)` | `func (w *ReportWriter)` |
| `func (self *Scanner)` | `func (s *Scanner)` |

### 4.6 常量名

常量名必须使用 MixedCaps。不应是值的派生物，而应解释值表示什么。

```go
// Good:
const MaxPacketSize = 512

const (
    ExecuteBit = 1 << iota
    WriteBit
    ReadBit
)
```

```go
// Bad:
const MAX_PACKET_SIZE = 512
const kMaxBufferSize = 1024
```

### 4.7 首字母缩略词

名称中的首字母缩略词（如 `URL`、`NATO`）应大小写一致。

| 英文用法 | 作用域 | 正确 | 错误 |
|----------|--------|------|------|
| XML API | 导出 | `XMLAPI` | `XmlApi`、`XMLApi`、`XmlAPI` |
| XML API | 未导出 | `xmlAPI` | `xmlapi`、`xmlApi` |
| iOS | 导出 | `IOS` | `Ios`、`IoS` |
| iOS | 未导出 | `iOS` | `ios` |
| gRPC | 导出 | `GRPC` | `Grpc` |
| gRPC | 未导出 | `gRPC` | `grpc` |
| DDoS | 导出 | `DDoS` | `DDOS`、`Ddos` |
| DDoS | 未导出 | `ddos` | `dDoS`、`dDOS` |
| ID | 导出 | `ID` | `Id` |
| ID | 未导出 | `id` | `iD` |
| DB | 导出 | `DB` | `Db` |
| DB | 未导出 | `db` | `dB` |

### 4.8 Getter

函数和方法名不应使用 `Get` 或 `get` 前缀，除非底层概念使用"get"这个词（如 HTTP GET）。

```go
// Good: Counts
// Bad: GetCounts
```

如果函数涉及复杂计算或远程调用，可以使用 `Compute` 或 `Fetch`。

### 4.9 变量名

变量名的长度应与其作用域大小成正比，与使用次数成反比。

- 小作用域（1-7行）：短名
- 中作用域（8-15行）：中等长度
- 大作用域（15-25行）：较长名称
- 非常大作用域（>25行）：非常描述性的名称

- 单字名称如 `count` 或 `options` 是好的起点
- 不要简单省略字母来节省打字（`Sandbox` 优于 `Sbx`）
- 从变量名中省略类型和类型类词
  - `userCount` 优于 `numUsers` 或 `usersInt`
  - `users` 优于 `userSlice`

#### 4.9.1 单字母变量名

单字母变量名可用于：
- 方法接收器（一到两个字母）
- 常见类型的熟悉变量名：`r`（`io.Reader` 或 `*http.Request`）、`w`（`io.Writer` 或 `http.ResponseWriter`）
- 整数循环变量，特别是索引（`i`）和坐标（`x`、`y`）

### 4.10 重复

#### 4.10.1 包与导出符号名

导出符号时，包名始终可见，因此两者之间应减少或消除冗余信息。

```go
// Examples:
// widget.NewWidget -> widget.New
// widget.NewWidgetWithName -> widget.NewWithName
// db.LoadFromDatabase -> db.Load
// goatteleportutil.CountGoatsTeleported -> gtutil.CountGoatsTeleported
// myteampb.MyTeamMethodRequest -> mtpb.MyTeamMethodRequest
```

#### 4.10.2 变量名与类型

```go
// Bad:  var numUsers int
// Good: var users int

// Bad:  var nameString string
// Good: var name string

// Bad:  var primaryProject *Project
// Good: var primary *Project
```

#### 4.10.3 外部上下文与局部名

```go
// Bad:  // In package "ads/targeting/revenue/reporting"
//       type AdsTargetingRevenueReport struct{}
// Good: type Report struct{}

// Bad:  func (p *Project) ProjectName() string
// Good: func (p *Project) Name() string
```

### 4.11 变量遮蔽 (Shadowing)

使用 `:=` 短变量声明时，有时不会创建新变量（称为"stomping"），有时会在新作用域中引入新变量（称为"shadowing"）。

**Stomping**（原始值不再需要时）：
```go
// Good:
func (s *Server) innerHandler(ctx context.Context, req *pb.MyRequest) *pb.MyResponse {
    ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
    defer cancel()
    // 这里的代码不再能访问原始 context
}
```

**Shadowing**（错误示例）：
```go
// Bad:
func (s *Server) innerHandler(ctx context.Context, req *pb.MyRequest) *pb.MyResponse {
    if *shortenDeadlines {
        ctx, cancel := context.WithTimeout(ctx, 3*time.Second)  // BUG: 新作用域中的 ctx
        defer cancel()
    }
    // BUG: "ctx" 这里又指向调用者提供的原始 context
}
```

正确版本：
```go
// Good:
func (s *Server) innerHandler(ctx context.Context, req *pb.MyRequest) *pb.MyResponse {
    if *shortenDeadlines {
        var cancel func()
        ctx, cancel = context.WithTimeout(ctx, 3*time.Second)  // 使用 = 而非 :=
        defer cancel()
    }
}
```

不要使用与标准包同名的变量名，除非在非常小的作用域中：
```go
// Bad:
func LongFunction() {
    url := "https://example.com/"
    // Oops, 现在无法使用 net/url 包
}
```

### 4.12 Util 包

Go 包名应该与包提供的内容相关。将包命名为 `util`、`helper`、`common` 等通常是不好的选择。

```go
// Good:
db := spannertest.NewDatabaseFromFile(...)
_, err := f.Seek(0, io.SeekStart)
b := elliptic.Marshal(curve, x, y)

// Bad:
db := test.NewDatabaseFromFile(...)
_, err := f.Seek(0, common.SeekStart)
b := helper.Marshal(curve, x, y)
```

---

## 5. 包大小与组织

### 5.1 包大小

如果你在问自己 Go 包应该有多大，一个好的起点是 Go 官方关于包名的博客文章。

其他考虑因素：
- 用户在一个页面上看到包的 godoc
- 如果客户端代码可能需要两个不同类型的值来相互交互，将它们放在同一个包中可能更方便
- 包内代码可以访问未导出的标识符
- 如果用户必须导入两个包才能有意义地使用其中任何一个，通常应该将它们合并

将整个项目放在一个包中可能太大。当某物在概念上独立时，给它自己的小包可以使它更容易使用。

Go 风格对文件大小很灵活，因为维护者可以在包内将代码从一个文件移动到另一个文件而不影响调用者。但作为一般准则：
- 单个文件有数千行通常不好
- 许多小文件也不好
- 没有"一个类型一个文件"的约定
- 文件应该足够聚焦，维护者能知道哪个文件包含什么
- 标准库经常将大包拆分为几个源文件，按文件分组相关代码

### 5.2 文件组织

包文档较长的包可以选择 dedicating 一个名为 `doc.go` 的文件，只包含包文档和包声明，但这不是必需的。

### 5.3 参考示例

- **小包**（一个内聚概念）：
  - `csv`：CSV 数据编码和解码
  - `expvar`：白盒程序遥测
- **中等包**（一个大域及其多个职责）：
  - `flag`：命令行标志管理
- **大包**（几个密切相关的域）：
  - `http`：client.go（HTTP 客户端）、server.go（HTTP 服务器）、cookie.go（cookie 管理）
  - `os`：exec.go（子进程管理）、file.go（文件管理）、tempfile.go（临时文件）

---

## 6. 导入规范

### 6.1 Protocol Buffer 消息和 Stub

Proto 库导入的处理方式与标准 Go 导入不同：
- `pb` 后缀通常用于 `go_proto_library` 规则
- `grpc` 后缀通常用于 `go_grpc_library` 规则

```go
// Good:
import (
    foopb "path/to/package/foo_service_go_proto"
    foogrpc "path/to/package/foo_service_go_grpc"
)
```

优先使用完整单词。短名是好的，但避免歧义。不确定时，使用 proto 包名直到 `_go` 并加 `pb` 后缀：

```go
// Good:
import (
    pushqueueservicepb "path/to/package/push_queue_service_go_proto"
)
```

### 6.2 导入分组

导入应组织为两组：
1. 标准库包
2. 其他（项目和 vendored 包）

```go
// Good:
import (
    "fmt"
    "hash/adler32"
    "os"

    "github.com/dsnet/compress/flate"
    "golang.org/x/text/encoding"
    "google.golang.org/protobuf/proto"
    foopb "myproj/foo/proto/proto"
    _ "myproj/rpc/protocols/dial"
    _ "myproj/security/auth/authhooks"
)
```

如果需要，可以将项目包分成多个组（如重命名导入、副作用导入）：

```go
// Good:
import (
    "fmt"
    "hash/adler32"
    "os"

    "github.com/dsnet/compress/flate"
    "golang.org/x/text/encoding"
    "google.golang.org/protobuf/proto"

    foopb "myproj/foo/proto/proto"

    _ "myproj/rpc/protocols/dial"
    _ "myproj/security/auth/authhooks"
)
```

### 6.3 导入重命名

包导入通常不应重命名，但在以下情况必须或可以改善可读性：
- 避免与其他导入的名称冲突
- 生成的 protobuf 包必须重命名以去除下划线，本地名必须有 `pb` 后缀
- 非自动生成包如果名称无信息量（如 `util` 或 `v1`）可以重命名

如果包名与常用局部变量名冲突（如 `url`、`ssh`），首选使用 `pkg` 后缀（如 `urlpkg`）。

### 6.4 空白导入 (`import _`)

仅因副作用而导入的包（`import _ "package"`）只能在 main 包或需要它们的测试中使用。

不要在库包中使用空白导入，即使库间接依赖它们。

例外：
- 绕过 nogo 静态检查器的禁止导入检查
- 在使用 `//go:embed` 编译器指令的源文件中空白导入 `embed` 包

### 6.5 点导入 (`import .`)

不要在 Google 代码库中使用此功能；它使功能来源更难判断。

---

## 7. 错误处理

### 7.1 返回错误

使用 `error` 来信号函数可能失败。按照惯例，`error` 是最后一个结果参数。

```go
// Good:
func Good() error { /* ... */ }
```

返回 `nil` 错误是信号操作成功的惯用方式。如果函数返回错误，调用者必须将所有非错误返回值视为未指定，除非另有明确文档说明。

```go
// Good:
func GoodLookup() (*Result, error) {
    if err != nil {
        return nil, err
    }
    return res, nil
}
```

导出的返回错误的函数应使用 `error` 类型返回错误。具体错误类型容易产生微妙的 bug：具体的 `nil` 指针可能被包装到接口中，从而变成非 nil 值。

```go
// Bad:
func Bad() *os.PathError { /*...*/ }
```

### 7.2 错误字符串

错误字符串不应大写（除非以导出名称、专有名词或首字母缩略词开头），也不应以标点符号结尾。

```go
// Bad:
err := fmt.Errorf("Something bad happened.")

// Good:
err := fmt.Errorf("something bad happened")
```

完整显示消息（日志、测试失败、API 响应或其他 UI）的风格应通常大写：

```go
// Good:
log.Infof("Operation aborted: %v", err)
t.Errorf("Op(%q) failed unexpectedly; err=%v", args, err)
```

### 7.3 处理错误

遇到错误的代码应该对如何处理它做出深思熟虑的选择。通常不应使用 `_` 变量丢弃错误。

```go
// Good:
var b *bytes.Buffer
n, _ := b.Write(p) // never returns a non-nil error
```

### 7.4 带内错误

不要使用带内错误处理（如返回 -1、null 或空字符串来信号错误或缺失结果）。

```go
// Bad:
// Lookup returns the value for key or -1 if there is no mapping for key.
func Lookup(key string) int
```

Go 对多返回值的支持提供了更好的解决方案：

```go
// Good:
// Lookup returns the value for key or ok=false if there is no mapping for key.
func Lookup(key string) (value string, ok bool)
```

### 7.5 缩进错误流

在继续其余代码之前处理错误。这通过使读者快速找到正常路径来提高代码可读性。

```go
// Good:
if err != nil {
    // error handling
    return // or continue, etc.
}
// normal code
```

```go
// Bad:
if err != nil {
    // error handling
} else {
    // normal code that looks abnormal due to indentation
}
```

### 7.6 错误结构

如果调用者需要询问错误（如区分不同的错误条件），给错误值结构以便程序化完成，而不是让调用者进行字符串匹配。

最简单的结构化错误是未参数化的全局值：

```go
type Animal string

var (
    ErrDuplicate = errors.New("duplicate")
    ErrMarsupial = errors.New("marsupials are not supported")
)

func process(animal Animal) error {
    switch {
    case seen[animal]:
        return ErrDuplicate
    case marsupial(animal):
        return ErrMarsupial
    }
    seen[animal] = true
    return nil
}
```

调用者可以简单地比较：

```go
// Good:
func handlePet(...) {
    switch err := process(an); err {
    case ErrDuplicate:
        return fmt.Errorf("feed %q: %v", an, err)
    case ErrMarsupial:
        alternate = an.BackupAnimal()
        return handlePet(..., alternate, ...)
    }
}
```

如果 `process` 返回包装的错误，使用 `errors.Is`：

```go
// Good:
func handlePet(...) {
    switch err := process(an); {
    case errors.Is(err, ErrDuplicate):
        return fmt.Errorf("feed %q: %v", an, err)
    case errors.Is(err, ErrMarsupial):
        // ...
    }
}
```

不要基于字符串形式区分错误：

```go
// Bad:
func handlePet(...) {
    err := process(an)
    if regexp.MatchString(`duplicate`, err.Error()) {...}
    if regexp.MatchString(`marsupial`, err.Error()) {...}
}
```

### 7.7 向错误添加信息

向错误添加信息时，避免底层错误已经提供的冗余信息。

```go
// Good:
if err := os.Open("settings.txt"); err != nil {
    return fmt.Errorf("launch codes unavailable: %v", err)
}
// Output: launch codes unavailable: open settings.txt: no such file or directory

// Bad:
if err := os.Open("settings.txt"); err != nil {
    return fmt.Errorf("could not open settings.txt: %v", err)
}
// Output: could not open settings.txt: open settings.txt: no such file or directory
```

不要添加仅用于指示失败的注释：

```go
// Bad:
return fmt.Errorf("failed: %v", err) // just return err instead
```

### 7.8 `%v` 与 `%w` 的选择

**`%v` 用于简单注释或新错误**：
- 添加有趣、非冗余的上下文
- 日志或显示错误
- 创建全新的独立错误（在系统边界处转换领域特定错误）

```go
// Good:
func (*FortuneTeller) SuggestFortune(...) (*pb.SuggestionResponse, error) {
    if err != nil {
        return nil, fmt.Errorf("couldn't find fortune database: %v", err)
    }
}
```

**`%w` 用于程序化检查和错误链**：
- 添加上下文同时保留原始错误以供程序化检查
- 当明确文档化和测试暴露的底层错误时

```go
// Good:
func (s *Server) internalFunction(ctx context.Context) error {
    if err != nil {
        return fmt.Errorf("couldn't find remote file: %w", err)
    }
}
```

### 7.9 `%w` 的放置位置

优先将 `%w` 放在错误字符串的末尾，形式为 `[...]: %w`。

```go
// Good:
err1 := fmt.Errorf("err1")
err2 := fmt.Errorf("err2: %w", err1)
err3 := fmt.Errorf("err3: %w", err2)
fmt.Println(err3) // err3: err2: err1
```

```go
// Bad:
err1 := fmt.Errorf("err1")
err2 := fmt.Errorf("%w: err2", err1)
err3 := fmt.Errorf("%w: err3", err2)
fmt.Println(err3) // err1: err2: err3
```

#### 哨兵错误放置的例外

当包装哨兵错误时，将 `%w` 放在开头可以提高可读性：

```go
// Good:
var ErrParse = fmt.Errorf("parse error")
var ErrParseInvalidHeader = fmt.Errorf("%w: invalid header", ErrParse)

func parseHeader() error {
    err := checkHeader()
    return fmt.Errorf("%w: invalid character in header: %v", ErrParseInvalidHeader, err)
}
```

### 7.10 错误日志

- 日志消息应清楚表达出了什么问题
- 如果返回错误，通常最好不要自己记录，而是让调用者处理
- 注意 PII（个人身份信息）
- 谨慎使用 `log.Error`。ERROR 级别日志会导致刷新，性能开销更大

#### 自定义详细级别

使用详细日志（`log.V`）：
- `V(1)`：少量额外信息
- `V(2)`：跟踪更多信息
- `V(3)`：转储大型内部状态

```go
// Good:
for _, sql := range queries {
    log.V(1).Infof("Handling %v", sql)
    if log.V(2) {
        log.Infof("Handling %v", sql.Explain())
    }
    sql.Run(...)
}

// Bad:
// sql.Explain 即使日志不打印也会被调用
log.V(2).Infof("Handling %v", sql.Explain())
```

### 7.11 程序初始化

程序初始化错误（如错误的标志和配置）应向上传播到 `main`，`main` 应调用 `log.Exit` 并附带解释如何修复错误的错误消息。通常不应使用 `log.Fatal`。

### 7.12 程序检查和 panic

标准错误处理应围绕错误返回值构建。库应优先向调用者返回错误，而不是中止程序。

如果内部状态变得不可恢复，调用 `log.Fatal` 是最可靠的方式。在这些情况下使用 `panic` 不可靠，因为延迟函数可能死锁或进一步损坏状态。

不要试图恢复 panic 来避免崩溃，因为这可能导致传播损坏状态。

### 7.13 何时使用 panic

标准库在 API 误用时 panic。例如，`reflect` 在许多情况下 panic。

panic 的另一个有用（虽然不常见）情况是作为包的内部实现细节，在调用链中始终有匹配的 recover。解析器和类似的深度嵌套、紧密耦合的内部函数组可以受益于这种设计。

关键属性：**panic 绝不能跨越包边界逃逸**，也不构成包的 API 的一部分。

```go
// Good:
type syntaxError struct {
    msg string
}

func parseInt(in string) int {
    n, err := strconv.Atoi(in)
    if err != nil {
        panic(&syntaxError{"not a valid integer"})
    }
    return n
}

func Parse(in string) (_ *Node, err error) {
    defer func() {
        if p := recover(); p != nil {
            sErr, ok := p.(*syntaxError)
            if !ok {
                panic(p) // 传播 panic，因为它在我们的代码域之外
            }
            err = fmt.Errorf("syntax error: %v", sErr.msg)
        }
    }()
    // ... Parse input calling parseInt internally
}
```

当编译器无法识别不可达代码时，也使用 panic：

```go
// Good:
func answer(i int) string {
    switch i {
    case 42:
        return "yup"
    case 54:
        return "base 13, huh"
    default:
        log.Fatalf("Sorry, %d is not the answer.", i)
        panic("unreachable")
    }
}
```

---

## 8. 文档规范

### 8.1 约定

熟悉风格的 Go 文档代码更容易阅读，不太可能被误用。可运行示例出现在 Godoc 和代码搜索中，是解释如何使用代码的绝佳方式。

#### 参数和配置

并非每个参数都必须在文档中枚举。适用于：
- 函数和方法参数
- 结构体字段
- 选项 API

记录容易出错或不明显的字段和参数，说明它们为什么有趣。

```go
// Bad:
// Sprintf formats according to a format specifier and returns the resulting string.
//
// format is the format, and data is the interpolation data.
func Sprintf(format string, data ...any) string

// Good:
// Sprintf formats according to a format specifier and returns the resulting string.
//
// The provided data is used to interpolate the format string. If the data does
// not match the expected format verbs or the amount of data does not satisfy
// the format specification, the function will inline warnings about formatting
// errors into the output string.
func Sprintf(format string, data ...any) string
```

#### Context

context 参数的取消会中断提供给它的函数，这是隐含的。如果函数可以返回错误，通常是 `ctx.Err()`。

这个事实不需要重述：

```go
// Bad:
// Run executes the worker's run loop.
//
// The method will process work until the context is cancelled and accordingly
// returns an error.
func (Worker) Run(ctx context.Context) error

// Good:
// Run executes the worker's run loop.
func (Worker) Run(ctx context.Context) error
```

当 context 行为不同或不明显时，应明确记录：

```go
// Good:
// Run executes the worker's run loop.
//
// If the context is cancelled, Run returns a nil error.
func (Worker) Run(ctx context.Context) error

// Good:
// Run executes the worker's run loop.
//
// Run processes work until the context is cancelled or Stop is called.
// Context cancellation is handled asynchronously internally: run may return
// before all work has stopped. The Stop method is synchronous and waits
// until all operations from the run loop finish. Use Stop for graceful
// shutdown.
func (Worker) Run(ctx context.Context) error
func (Worker) Stop()

// Good:
// NewReceiver starts receiving messages sent to the specified queue.
// The context should not have a deadline.
func NewReceiver(ctx context.Context) *Receiver
```

#### 并发

Go 用户假设概念上只读的操作对并发使用是安全的，不需要额外的同步。

以下情况强烈建议文档说明：
- 不清楚操作是只读还是变异：
```go
// Good:
package lrucache
// Lookup returns the data associated with the key from the cache.
//
// This operation is not safe for concurrent use.
func (*Cache) Lookup(key string) (data []byte, ok bool)
```
- API 提供同步：
```go
// Good:
// NewFortuneTellerClient returns an *rpc.Client for the FortuneTeller service.
// It is safe for simultaneous use by multiple goroutines.
func NewFortuneTellerClient(cc *rpc.ClientConn) *FortuneTellerClient
```
- API 消费用户实现的接口类型，消费者对并发有特定要求：
```go
// Good:
package health
// A Watcher reports the health of some entity (usually a backend service).
//
// Watcher methods are safe for simultaneous use by multiple goroutines.
type Watcher interface {
    Watch(changed chan<- bool) (unwatch func())
    Health() error
}
```

#### 清理

记录 API 的任何显式清理要求：

```go
// Good:
// NewTicker returns a new Ticker containing a channel that will send the
// current time on the channel after each tick.
//
// Call Stop to release the Ticker's associated resources when done.
func NewTicker(d Duration) *Ticker
func (*Ticker) Stop()
```

如果不清楚如何清理资源，解释如何清理：

```go
// Good:
// Get issues a GET to the specified URL.
//
// When err is nil, resp always contains a non-nil resp.Body.
// Caller should close resp.Body when done reading from it.
//
//    resp, err := http.Get("http://example.com/")
//    if err != nil {
//        // handle error
//    }
//    defer resp.Body.Close()
//    body, err := io.ReadAll(resp.Body)
func (c *Client) Get(url string) (resp *Response, err error)
```

#### 错误

记录函数返回给调用者的重要错误哨兵值或错误类型：

```go
// Good:
package os
// Read reads up to len(b) bytes from the File and stores them in b. It returns
// the number of bytes read and any error encountered.
//
// At end of file, Read returns 0, io.EOF.
func (*File) Read(b []byte) (n int, err error)
```

当函数返回特定错误类型时，正确记录错误是否是指针接收器：

```go
// Good:
package os

type PathError struct {
    Op   string
    Path string
    Err  error
}

// Chdir changes the current working directory to the named directory.
//
// If there is an error, it will be of type *PathError.
func Chdir(dir string) error
```

在包的文档中记录整体错误约定：

```go
// Good:
// Package os provides a platform-independent interface to operating system
// functionality.
//
// Often, more information is available within the error. For example, if a
// call that takes a file name fails, such as Open or Stat, the error will
// include the failing file name when printed and will be of type *PathError,
// which may be unpacked for more information.
package os
```

### 8.2 Godoc 格式化

- 空行用于分隔段落
- 测试文件可以包含可运行示例，出现在 godoc 中
- 额外缩进两个空格的行被格式化为逐字文本
- 以大写字母开头、除括号和逗号外无标点符号、后跟另一段的单行被格式化为标题

```go
// Good:
// LoadConfig reads a configuration out of the named file.
//
// See some/shortlink for config file format details.

// Good:
// Update runs the function in an atomic transaction.
//
// This is typically used with an anonymous TransactionFunc:
//
//   if err := db.Update(func(state *State) { state.Foo = bar }); err != nil {
//     //...
//   }
```

### 8.3 信号增强

有时一行代码看起来像某种常见的东西，但实际上不是。最好的例子之一是 `err == nil` 检查（因为 `err != nil` 更常见）。

```go
// Good:
if err := doSomething(); err != nil {
    // ...
}

// Bad:
if err := doSomething(); err == nil {
    // ...
}

// Good: 通过添加注释增强信号
if err := doSomething(); err == nil { // if NO error
    // ...
}
```

### 8.4 注释

所有顶级导出名称必须有文档注释，未导出类型或函数声明如果有不明显的行为或含义也应有注释。

注释应以被描述对象的名称开头的完整句子。冠词（"a"、"an"、"the"）可以放在名称前面使其读起来更自然。

```go
// Good:
// A Request represents a request to run a command.
type Request struct { ... }

// Encode writes the JSON encoding of req to w.
func Encode(w io.Writer, req *Request) { ... }
```

文档注释适用于以下符号，或如果出现在结构体中则适用于字段组：

```go
// Good:
// Options configure the group management service.
type Options struct {
    // General setup:
    Name  string
    Group *FooGroup

    // Dependencies:
    DB *sql.DB

    // Customization:
    LargeGroupThreshold int // optional; default: 10
    MinimumMembers      int // optional; default: 2
}
```

注释如果是完整句子，应像标准英语句子一样大写和标点。简单句末注释（特别是结构体字段）可以是简单短语。

包注释必须紧接在包子句上方，包注释和包名之间没有空行。

```go
// Good:
// Package math provides basic constants and mathematical functions.
//
// This package does not guarantee bit-identical results across architectures.
package math
```

每个包必须只有一个包注释。如果包由多个文件组成，恰好一个文件应有包注释。

`main` 包的注释有稍微不同的形式，BUILD 文件中 `go_binary` 规则的名称代替包名：

```go
// Good:
// The seed_generator command is a utility that generates a Finch seed file
// from a set of JSON study configs.
package main
```

### 8.5 命名结果参数

命名参数时，考虑函数签名在 Godoc 中的显示方式。

```go
// Good:
func (n *Node) Parent1() *Node
func (n *Node) Parent2() (*Node, error)
```

如果函数返回两个或更多相同类型的参数，添加名称可能有帮助：

```go
// Good:
func (n *Node) Children() (left, right *Node, err error)
```

如果调用者必须对特定结果参数采取行动，命名它们可以帮助建议行动是什么：

```go
// Good:
// WithTimeout returns a context that will be canceled no later than d duration
// from now.
//
// The caller must arrange for the returned cancel function to be called when
// the context is no longer needed to prevent a resource leak.
func WithTimeout(parent Context, d time.Duration) (ctx Context, cancel func())
```

不要使用命名结果参数来避免在函数内部声明变量：

```go
// Bad:
func (n *Node) Parent1() (node *Node)
func (n *Node) Parent2() (node *Node, err error)
```

裸返回只在小函数中可接受。一旦函数达到中等大小，就明确写出返回值。

如果值必须在延迟闭包中更改，命名结果参数总是可以接受的。

### 8.6 示例

包应清楚记录其预期用法。尽量提供可运行示例；示例出现在 Godoc 中。可运行示例属于测试文件，而非生产源文件。


---

## 9. 变量声明

### 9.1 初始化

为了一致性，用非零值初始化新变量时优先使用 `:=` 而非 `var`。

```go
// Good:
i := 42

// Bad:
var i = 42
```

### 9.2 零值声明

以下声明使用零值：

```go
// Good:
var (
    coords Point
    magic  [4]byte
    primes []int
)
```

当你想传达一个**准备好供后续使用**的空值时，应使用零值声明。

```go
// Bad:
var (
    coords = Point{X: 0, Y: 0}
    magic  = [4]byte{0, 0, 0, 0}
    primes = []int(nil)
)
```

零值声明的常见应用是在 unmarshalling 时用作输出：

```go
// Good:
var coords Point
if err := json.Unmarshal(data, &coords); err != nil {
```

需要指针类型的零值时：

```go
// Good:
msg := new(pb.Bar) // or "&pb.Bar{}"
if err := proto.Unmarshal(data, msg); err != nil {
```

如果结构体包含不可复制的字段（如锁），可以将其设为值类型以利用零值初始化。这意味着包含类型现在必须通过指针传递：

```go
// Good:
type Counter struct {
    mu   sync.Mutex
    data map[string]int64
}

// Note this must be a pointer receiver to prevent copying.
func (c *Counter) IncrementBy(name string, n int64)
```

```go
// Good:
func NewCounter(name string) *Counter {
    c := new(Counter)
    registerCounter(name, c)
    return c
}

var msg = new(pb.Bar)
```

```go
// Bad:
func NewCounter(name string) *Counter {
    var c Counter
    registerCounter(name, &c)
    return &c
}

var msg = pb.Bar{}
```

> **重要**: Map 类型必须在修改之前显式初始化。但从零值 map 读取是完全可以的。

### 9.3 复合字面量

以下使用复合字面量声明：

```go
// Good:
var (
    coords   = Point{X: x, Y: y}
    magic    = [4]byte{'I', 'W', 'A', 'D'}
    primes   = []int{2, 3, 5, 7, 11}
    captains = map[string]string{"Kirk": "James Tiberius", "Picard": "Jean-Luc"}
)
```

当你知道初始元素或成员时，应使用复合字面量。

需要零值指针时，两个选项都可以：`new` 关键字可以提醒读者如果需要非零值，复合字面量不适用：

```go
// Good:
var (
    buf = new(bytes.Buffer) // 非空 Buffer 用构造函数初始化
    msg = new(pb.Message) // 非空 proto 消息用 builder 或逐个设置字段初始化
)
```

### 9.4 大小提示

以下声明利用大小提示预先分配容量：

```go
// Good:
var (
    buf = make([]byte, 131072)
    q = make([]Node, 0, 16)
    seen = make(map[string]bool, shardSize)
)
```

大小提示和预分配是重要步骤，**当结合对代码及其集成的实证分析时**，以创建性能敏感和资源高效的代码。

大多数代码不需要大小提示或预分配，可以让运行时根据需要增长 slice 或 map。

> **警告**: 预分配比需要更多的内存可能浪费内存甚至损害性能。不确定时，默认使用零初始化或复合字面量声明。

### 9.5 Channel 方向

尽可能指定 channel 方向。

```go
// Good:
func sum(values <-chan int) int {
    // ...
}
```

这防止了不指定时可能出现的简单编程错误：

```go
// Bad:
func sum(values chan int) (out int) {
    for v := range values {
        out += v
    }
    close(values) // panic: 关闭已关闭的 channel
}
```

---

## 10. 函数参数列表

不要让函数签名变得太长。参数越多，单个参数的角色越不清晰。

### 10.1 选项结构

选项结构是收集函数或方法部分或全部参数的结构体类型，作为最后一个参数传递给函数。

```go
// Bad:
func EnableReplication(ctx context.Context, config *replicator.Config, primaryRegions, readonlyRegions []string, replicateExisting, overwritePolicies bool, replicationInterval time.Duration, copyWorkers int, healthWatcher health.Watcher)

// Good:
type ReplicationOptions struct {
    Config              *replicator.Config
    PrimaryRegions      []string
    ReadonlyRegions     []string
    ReplicateExisting   bool
    OverwritePolicies   bool
    ReplicationInterval time.Duration
    CopyWorkers         int
    HealthWatcher       health.Watcher
}

func EnableReplication(ctx context.Context, opts ReplicationOptions)
```

调用：

```go
// Good:
storage.EnableReplication(ctx, storage.ReplicationOptions{
    Config:              config,
    PrimaryRegions:      []string{"us-east1", "us-central2", "us-west3"},
    ReadonlyRegions:     []string{"us-east5", "us-central6"},
    OverwritePolicies:   true,
    ReplicationInterval: 1 * time.Hour,
    CopyWorkers:         100,
    HealthWatcher:       watcher,
})
```

> **注意**: Context 永远不应包含在选项结构中。

### 10.2 可变选项

使用可变选项，导出返回闭包的函数，这些闭包可以传递给函数的 variadic (`...`) 参数。

```go
// Good:
type replicationOptions struct {
    readonlyCells       []string
    replicateExisting   bool
    overwritePolicies   bool
    replicationInterval time.Duration
    copyWorkers         int
    healthWatcher       health.Watcher
}

type ReplicationOption func(*replicationOptions)

func ReadonlyCells(cells ...string) ReplicationOption {
    return func(opts *replicationOptions) {
        opts.readonlyCells = append(opts.readonlyCells, cells...)
    }
}

func ReplicateExisting(enabled bool) ReplicationOption {
    return func(opts *replicationOptions) {
        opts.replicateExisting = enabled
    }
}

var DefaultReplicationOptions = []ReplicationOption{
    OverwritePolicies(true),
    ReplicationInterval(12 * time.Hour),
    CopyWorkers(10),
}

func EnableReplication(ctx context.Context, config *placer.Config, primaryCells []string, opts ...ReplicationOption)
```

调用：

```go
// Good:
storage.EnableReplication(ctx, config, []string{"po", "is", "ea"},
    storage.ReadonlyCells("ix", "gg"),
    storage.OverwritePolicies(true),
    storage.ReplicationInterval(1*time.Hour),
    storage.CopyWorkers(100),
    storage.HealthWatcher(watcher),
)
```

选项应按顺序处理。如果有冲突或非累积选项被多次传递，最后一个参数应获胜。

---

## 11. 命令行接口

### 11.1 子命令库

如果不需要额外功能，推荐 `subcommands`（最简单且易于正确使用）。如果需要 `cobra` 提供的额外功能，可以选择它。

- **cobra**: getopt 标志约定，功能丰富，但使用中有陷阱
- **subcommands**: Go 标志约定，简单且易于正确使用，推荐

**警告**: cobra 命令函数应使用 `cmd.Context()` 获取 context，而不是用 `context.Background()` 创建自己的根 context。

### 11.2 库与 CLI 分离

如果代码可以同时作为库和二进制文件使用，通常将 CLI 代码和库分开是有益的，使 CLI 只是其客户端之一。

---

## 12. 测试规范

### 12.1 测试辅助函数 vs 断言辅助函数

- **测试辅助函数**：执行设置或清理任务。所有失败都是环境的失败（非被测代码），如测试数据库无法启动。调用 `t.Helper` 标记为测试辅助函数。
- **断言辅助函数**：检查系统正确性并在期望未满足时使测试失败。**在 Go 中不被认为是惯用的**。

测试的目的是报告被测代码的通过/失败条件。使测试失败的理想位置是在 `Test` 函数本身内。

如果许多测试用例需要相同的验证逻辑，按以下方式安排：
- 在 `Test` 函数中内联逻辑（即使重复）
- 如果输入相似，考虑统一为表驱动测试，同时保持逻辑内联
- 如果多个调用者需要相同的验证函数但表测试不合适，让验证函数返回值（通常是 `error`）而非接受 `testing.T` 参数

```go
// Good:
func polygonCmp() cmp.Option {
    return cmp.Options{
        cmp.Transformer("polygon", func(p *s2.Polygon) []*s2.Loop { return p.Loops() }),
        cmp.Transformer("loop", func(l *s2.Loop) []s2.Point { return l.Vertices() }),
        cmpopts.EquateApprox(0.00000001, 0),
        cmpopts.EquateEmpty(),
    }
}

func TestFenceposts(t *testing.T) {
    got := Fencepost(tomsDiner, 1*meter)
    if diff := cmp.Diff(want, got, polygonCmp()); diff != "" {
        t.Errorf("Fencepost(tomsDiner, 1m) returned unexpected diff (-want+got):\n%v", diff)
    }
}
```

### 12.2 可扩展验证 API 设计

#### 验收测试

验收测试的前提是使用者不知道测试中发生的每个细节；他们只是将输入交给测试设施来完成工作。

```go
// Good:
// ExercisePlayer tests a Player implementation in a single turn on a board.
//
// It returns a nil error if the player makes a correct move in the context
// of the provided board. Otherwise ExercisePlayer returns one of this
// package's errors to indicate how and why the player failed the
// validation.
func ExercisePlayer(b *chess.Board, p chess.Player) error
```

失败报告有两种方式：
- **快速失败**: 实现违反不变量时立即返回错误
- **聚合所有失败**: 收集所有失败，然后全部报告

### 12.3 使用真实传输

测试组件集成时，特别是使用 HTTP 或 RPC 作为底层传输时，优先使用真实的底层传输连接到后端测试版本。

### 12.4 `t.Error` vs `t.Fatal`

测试通常不应在遇到的第一个问题上中止。

`t.Fatal` 适用于：
- 测试设置失败，特别是测试设置辅助函数中
- 表驱动测试中影响整个测试函数的设置失败

对于影响单个表条目的失败：
- 不使用 `t.Run` 子测试：使用 `t.Error` 后跟 `continue`
- 使用子测试（在 `t.Run` 内部）：使用 `t.Fatal`（结束当前子测试，允许测试用例进入下一个子测试）

**警告**: 从单独的 goroutine 调用 `t.Fatal` 不安全。

### 12.5 测试辅助函数中的错误处理

测试辅助函数执行的操作有时会失败。当测试辅助函数失败时，优先在辅助函数中调用 `Fatal` 函数：

```go
// Good:
func mustAddGameAssets(t *testing.T, dir string) {
    t.Helper()
    if err := os.WriteFile(path.Join(dir, "pak0.pak"), pak0, 0644); err != nil {
        t.Fatalf("Setup failed: could not write pak0 asset: %v", err)
    }
}
```

这比让辅助函数将错误返回给测试本身更干净：

```go
// Bad:
func addGameAssets(t *testing.T, dir string) error {
    t.Helper()
    if err := os.WriteFile(path.Join(d, "pak0.pak"), pak0, 0644); err != nil {
        return err
    }
    return nil
}
```

### 12.6 不要在单独的 goroutine 中调用 `t.Fatal`

从除运行 Test 函数的 goroutine 之外的任何 goroutine 调用 `t.FailNow`、`t.Fatal` 等是不正确的。

```go
// Good:
func TestRevEngine(t *testing.T) {
    engine, err := Start()
    if err != nil {
        t.Fatalf("Engine failed to start: %v", err)
    }

    num := 11
    var wg sync.WaitGroup
    wg.Add(num)
    for i := 0; i < num; i++ {
        go func() {
            defer wg.Done()
            if err := engine.Vroom(); err != nil {
                t.Errorf("No vroom left on engine: %v", err)
                return
            }
        }()
    }
    wg.Wait()
}
```

### 12.7 在结构体字面量中使用字段名

在表驱动测试中，初始化测试用例结构体字面量时优先指定字段名。

```go
// Good:
func TestStrJoin(t *testing.T) {
    tests := []struct {
        slice     []string
        separator string
        skipEmpty bool
        want      string
    }{
        {
            slice:     []string{"a", "b", ""},
            separator: ",",
            want:      "a,b,",
        },
        {
            slice:     []string{"a", "b", ""},
            separator: ",",
            skipEmpty: true,
            want:      "a,b",
        },
    }
}
```

### 12.8 将设置代码限定到特定测试

尽可能将资源和依赖的设置限定到特定测试用例。

```go
// Good:
func TestParseData(t *testing.T) {
    data := mustLoadDataset(t)
    parsed, err := ParseData(data)
    if err != nil {
        t.Fatalf("Unexpected error parsing data: %v", err)
    }
    want := &DataTable{ /* ... */ }
    if got := parsed; !cmp.Equal(got, want) {
        t.Errorf("ParseData(data) = %v, want %v", got, want)
    }
}

func TestRegression682831(t *testing.T) {
    // 不使用数据集
    if got, want := guessOS("zpc79.example.com"), "grhat"; got != want {
        t.Errorf(`guessOS("zpc79.example.com") = %q, want %q`, got, want)
    }
}
```

```go
// Bad:
var dataset []byte

func init() {
    dataset = mustLoadDataset()
}
```

#### 何时使用自定义 `TestMain`

如果**包中的所有测试**都需要公共设置，且**设置需要清理**，可以使用自定义 `TestMain` 入口点。通常仅用于功能测试。

```go
// Good:
var db *sql.DB

func TestInsert(t *testing.T) { /* omitted */ }
func TestSelect(t *testing.T) { /* omitted */ }

func runMain(ctx context.Context, m *testing.M) (code int, err error) {
    ctx, cancel := context.WithCancel(ctx)
    defer cancel()

    d, err := setupDatabase(ctx)
    if err != nil {
        return 0, err
    }
    defer d.Close()
    db = d

    return m.Run(), nil
}

func TestMain(m *testing.M) {
    code, err := runMain(context.Background(), m)
    if err != nil {
        log.Fatal(err)
    }
    os.Exit(code)
}
```

#### 摊销公共测试设置

如果公共设置：昂贵、仅适用于部分测试、不需要清理，可以使用 `sync.Once`：

```go
// Good:
var dataset struct {
    once sync.Once
    data []byte
    err  error
}

func mustLoadDataset(t *testing.T) []byte {
    t.Helper()
    dataset.once.Do(func() {
        data, err := os.ReadFile("path/to/your/project/testdata/dataset")
        dataset.data = data
        dataset.err = err
    })
    if err := dataset.err; err != nil {
        t.Fatalf("Could not load dataset: %v", err)
    }
    return dataset.data
}
```

### 12.9 有用的测试失败

应该可以在不阅读测试源代码的情况下诊断测试失败。测试失败时应提供有帮助的消息，详细说明：
- 什么导致了失败
- 什么输入导致了错误
- 实际结果
- 期望的结果

#### 断言库

不要创建"断言库"作为测试辅助函数。

```go
// Bad:
var obj BlogPost
assert.IsNotNil(t, "obj", obj)
assert.StringEq(t, "obj.Type", obj.Type, "blogPost")
assert.IntEq(t, "obj.Comments", obj.Comments, 2)
```

优先使用标准库如 `cmp` 和 `fmt`：

```go
// Good:
var got BlogPost

want := BlogPost{
    Comments: 2,
    Body:     "Hello, world!",
}

if !cmp.Equal(got, want) {
    t.Errorf("Blog post = %v, want = %v", got, want)
}
```

#### 标识函数

在大多数测试中，失败消息应包含失败的函数名，格式为 `YourFunc(%v) = %v, want %v`。

#### 标识输入

在大多数测试中，失败消息应包含函数输入（如果简短）。

#### Got 在 want 之前

测试输出应先包含函数返回的实际值，再打印期望值。标准格式：`YourFunc(%v) = %v, want %v`。

#### 完整结构比较

如果函数返回结构体，避免手写逐字段比较。使用深度比较：

```go
// Good:
want := &Doc{
    Type:     "blogPost",
    Comments: 2,
    Body:     "This is the post body.",
    Authors:  []string{"isaac", "albert", "emmy"},
}
if !cmp.Equal(got, want) {
    t.Errorf("AddPost() = %+v, want %+v", got, want)
}
```

#### 比较稳定结果

避免比较可能依赖于你不拥有的包的输出稳定性的结果。

#### 继续运行

测试应尽可能继续运行，即使在失败后，以便在一次运行中打印出所有失败的检查。

优先调用 `t.Error` 而非 `t.Fatal` 来报告不匹配。

#### 打印差异

如果函数返回大型输出，使用差异比较：

```go
// Good:
if diff := cmp.Diff(want, got); diff != "" {
    t.Errorf("AddPost() returned unexpected diff (-want +got):\n%s", diff)
}
```

#### 测试错误语义

不要使用字符串比较来检查函数返回的错误类型。测试应仅测试可以可靠观察的语义信息。

```go
// Good:
err := f(test.input)
gotErr := err != nil
if gotErr != test.wantErr {
    t.Errorf("f(%q) = %v, want error presence = %v", test.input, err, test.wantErr)
}
```

### 12.10 子测试

标准 Go 测试库提供定义子测试的设施。允许设置和清理的灵活性、控制并行性和测试过滤。

子测试不应依赖其他用例的执行来成功或获取初始状态。

#### 子测试名称

命名子测试使其在测试输出中可读，在命令行上对测试过滤用户有用。

```go
// Good:
func TestTranslate(t *testing.T) {
    data := []struct {
        name, desc, srcLang, dstLang, srcText, wantDstText string
    }{
        {
            name:        "hu=en_bug-1234",
            desc:        "regression test following bug 1234",
            srcLang:     "hu",
            srcText:     "cigarettat es egy ongyujtot kerek",
            dstLang:     "en",
            wantDstText: "cigarettes and a lighter please",
        },
    }
    for _, d := range data {
        t.Run(d.name, func(t *testing.T) {
            got := Translate(d.srcLang, d.dstLang, d.srcText)
            if got != d.wantDstText {
                t.Errorf("%s\nTranslate(%q, %q, %q) = %q, want %q",
                    d.desc, d.srcLang, d.dstLang, d.srcText, got, d.wantDstText)
            }
        })
    }
}
```

避免：
```go
// Bad:
t.Run("check that there is no mention of scratched records or hovercrafts", ...)
t.Run("AM/PM confusion", ...)  // 斜杠在命令行上造成问题
```

### 12.11 表驱动测试

当许多不同的测试用例可以使用相似的测试逻辑进行测试时，使用表驱动测试。

```go
// Good:
func TestCompare(t *testing.T) {
    compareTests := []struct {
        a, b string
        want int
    }{
        {"", "", 0},
        {"a", "", 1},
        {"", "a", -1},
        {"abc", "abc", 0},
        {"ab", "abc", -1},
        {"abc", "ab", 1},
    }

    for _, test := range compareTests {
        got := Compare(test.a, test.b)
        if got != test.want {
            t.Errorf("Compare(%q, %q) = %v, want %v", test.a, test.b, got, test.want)
        }
    }
}
```

当一些测试用例需要使用与其他测试用例不同的逻辑检查时，编写多个测试函数。

#### 数据驱动测试用例

当表测试行变得复杂时，额外的清晰度来自测试用例之间的重复是必要的。

```go
// Good:
type decodeCase struct {
    name   string
    input  string
    output string
    err    error
}

func TestDecode(t *testing.T) {
    codex := setupCodex(t)
    var tests []decodeCase
    for _, test := range tests {
        t.Run(test.name, func(t *testing.T) {
            output, err := Decode(test.input, codex)
            if got, want := output, test.output; got != want {
                t.Errorf("Decode(%q) = %v, want %v", test.input, got, want)
            }
            if got, want := err, test.err; !cmp.Equal(got, want) {
                t.Errorf("Decode(%q) err %q, want %q", test.input, got, want)
            }
        })
    }
}
```

不要使用测试表中的索引作为测试名称或打印输入的替代：

```go
// Bad:
for i, d := range tests {
    if strings.ToUpper(d.input) != d.want {
        t.Errorf("Failed on case #%d", i)
    }
}
```

### 12.12 测试辅助函数

测试辅助函数是执行设置或清理任务的函数。如果传递 `*testing.T`，调用 `t.Helper` 将辅助函数中的失败归因于调用辅助函数的行。

```go
// Good:
func readFile(t *testing.T, filename string) string {
    t.Helper()
    contents, err := runfiles.ReadFile(filename)
    if err != nil {
        t.Fatal(err)
    }
    return string(contents)
}
```

### 12.13 测试包

#### 同包测试

测试可以定义在与被测代码相同的包中。

- 将测试放在 `foo_test.go` 文件中
- 使用 `package foo`
- 不要显式导入要测试的包

#### 不同包测试

有时不适合或不可能在与被测代码相同的包中定义测试。在这些情况下，使用带 `_test` 后缀的包名。

```go
// Good:
package gmailintegration_test

import "testing"
```

### 12.14 使用 `testing` 包

Go 标准库提供 `testing` 包。这是 Google 代码库中唯一允许的 Go 测试框架。特别是，不允许使用断言库和第三方测试框架。

---

## 13. 字符串连接

### 13.1 优先使用 "+" 处理简单情况

连接少量字符串时优先使用 "+"。这种方法语法最简单，不需要导入。

```go
// Good:
key := "projectid: " + p
```

### 13.2 格式化时优先使用 `fmt.Sprintf`

构建带格式化的复杂字符串时优先使用 `fmt.Sprintf`。

```go
// Good:
str := fmt.Sprintf("%s [%s:%d]-> %s", src, qos, mtu, dst)

// Bad:
bad := src.String() + " [" + qos.String() + ":" + strconv.Itoa(mtu) + "]-> " + dst.String()
```

**最佳实践**: 当字符串构建操作的输出是 `io.Writer` 时，不要用 `fmt.Sprintf` 构造临时字符串再发送到 Writer。而是直接使用 `fmt.Fprintf`。

### 13.3 逐步构建字符串时优先使用 `strings.Builder`

逐步构建字符串时优先使用 `strings.Builder`。`strings.Builder` 摊销线性时间，而 "+" 和 `fmt.Sprintf` 在顺序调用形成更大字符串时呈二次时间。

```go
// Good:
b := new(strings.Builder)
for i, d := range digitsOfPi {
    fmt.Fprintf(b, "the %d digit of pi is: %d\n", i, d)
}
str := b.String()
```

### 13.4 常量字符串

构建常量多行字符串字面量时优先使用反引号。

```go
// Good:
usage := `Usage:

custom_tool [args]`

// Bad:
usage := "" +
  "Usage:\n" +
  "\n" +
  "custom_tool [args]"
```

---

## 14. 全局状态

库不应强迫其客户端使用依赖全局状态的 API。建议不要公开控制所有客户端行为的 API 或导出包级变量作为其 API 的一部分。

相反，如果你的功能维护状态，允许客户端创建和使用实例值。

```go
// Good:
package sidecar

type Registry struct { plugins map[string]*Plugin }

func New() *Registry { return &Registry{plugins: make(map[string]*Plugin)} }

func (r *Registry) Register(name string, p *Plugin) error { ... }
```

```go
// Good:
package main

func main() {
    sidecars := sidecar.New()
    if err := sidecars.Register("Cloud Logger", cloudlogger.New()); err != nil {
        log.Exitf("Could not setup cloud logger: %v", err)
    }
    cfg := &myapp.Config{Sidecars: sidecars}
    myapp.Run(context.Background(), cfg)
}
```

```go
// Bad:
package sidecar

var registry = make(map[string]*Plugin)

func Register(name string, p *Plugin) error { /* registers plugin in registry */ }
```

使用全局状态会产生以下问题：
- 多个函数通过全局状态交互，尽管在其他方面独立
- 独立测试用例通过全局状态相互交互
- 用户被诱惑用测试替身替换全局状态
- 用户必须考虑与全局状态交互的特殊顺序要求

### 14.1 包状态 API 的主要形式

几种最常见的有问题 API 形式：

- 顶级变量（无论是否导出）
- 服务定位器模式（全局定义）
- 回调注册表
- 后端、存储、数据访问层等厚客户端单例

### 14.2 安全测试

如果满足以下任何条件，上述 API 是安全的：
- 全局状态在逻辑上是常量
- 包的可观察行为是无状态的
- 全局状态不会渗入程序外部的事物
- 没有可预测行为的期望

### 14.3 提供默认实例

虽然不推荐，但如果需要最大化用户便利性，可以提供使用包级状态的简化 API。

遵循以下准则：
1. 包必须提供客户端创建包类型的隔离实例的能力
2. 使用全局状态的公共 API 必须是前述 API 的薄代理
3. 包级 API 只能由二进制构建目标使用，不能由库使用
4. 包级 API 必须文档化和强制执行其不变量，并提供重置全局状态的 API

---

## 15. 接口设计

### 15.1 避免不必要的接口

最常见的错误是在真正需要之前创建接口。

1. **不要混淆概念与关键字**：仅仅因为你在设计"服务"或"仓库"模式，并不意味着你需要命名接口类型。首先关注行为和具体实现。
2. **重用现有接口**：如果接口已经存在，特别是在生成代码中，使用它。
3. **不要仅为测试定义后门**：不要从消费接口的 API 中导出测试替身实现。

当确实有意义创建接口时：
1. **多个实现**：当有两个或更多具体类型必须由相同逻辑处理时
2. **解耦包**：为了打破两个包之间的循环依赖
3. **隐藏复杂性**：当具体类型有巨大的 API 表面，但特定函数只需要一两个方法时

### 15.2 接口所有权和可见性

1. **不要不必要地导出接口类型**：如果接口仅在包内部使用，保持未导出。
2. **消费者定义接口**：在 Go 中，接口通常属于使用它们的包，而不是实现它们的包。消费者应只定义他们实际使用的方法。

生产者定义接口的常见场景：
- **接口就是产品**：当包的主要目的是提供许多不同实现必须遵循的通用协议时（如 `io.Writer`、`hash.Hash`）
- **防止接口膨胀**：在大型代码库中，维护多个包中完全相同的接口可能是不必要的负担
- **解决循环依赖**

### 15.3 设计有效的接口

1. **保持接口小**：接口越大，实现和编写利用它的代码就越难。
2. **文档**：将每个接口视为抽象的"用户手册"。
3. **接受接口，返回具体类型**：返回具体类型允许调用者使用值的完整功能，而不被锁定在特定接口抽象中。

返回接口是惯用选择的几种情况：
1. **封装**：限制默认 API 表面并引导调用者行为
2. **某些模式**：如果函数设计为基于运行时决策返回几种不同具体类型之一
3. **避免循环依赖**

---

## 16. 语言规范

### 16.1 字面量格式化

#### 字段名

结构体字面量必须为当前包外定义的类型指定**字段名**。

```go
// Good:
r := csv.Reader{
    Comma: ',',
    Comment: '#',
    FieldsPerRecord: 4,
}

// Bad:
r := csv.Reader{',', '#', 4, false, false, false, false}
```

对于包本地类型，字段名是可选的。

#### 匹配括号

括号对的关闭半部分应始终出现在与开启半部分相同缩进量的行上。

```go
// Good:
good := []*Type{{Key: "value"}}

good := []*Type{
    {Key: "multi"},
    {Key: "line"},
}

// Bad:
bad := []*Type{
    {Key: "multi"},
    {Key: "line"}}
```

#### 紧凑括号

仅在以下两个条件都为真时，才允许对 slice 和 array 字面量使用紧凑括号：
- 缩进匹配
- 内部值也是字面量或 proto builder（即不是变量或其他表达式）

```go
// Good:
good := []*Type{{ // Cuddled correctly
    Field: "value",
}, {
    Field: "value",
}}

// Bad:
bad := []*Type{
    first,
    {
        Field: "second",
    }}
```

#### 重复类型名

可以从 slice 和 map 字面量中省略重复的类型名。

```go
// Good:
good := []*Type{
    {A: 42},
    {A: 43},
}

// Bad:
repetitive := []*Type{
    &Type{A: 42},
    &Type{A: 43},
}
```

#### 零值字段

当不损失清晰度时，可以从结构体字面量中省略零值字段。

```go
// Bad:
ldb := leveldb.Open("/my/table", &db.Options{
    BlockSize: 1<<16,
    ErrorIfDBExists: true,
    BlockRestartInterval: 0,
    Comparer: nil,
    Compression: nil,
    // ... all zero values
})

// Good:
ldb := leveldb.Open("/my/table", &db.Options{
    BlockSize: 1<<16,
    ErrorIfDBExists: true,
})
```

### 16.2 Nil Slices

对于大多数目的，`nil` 和空 slice 之间没有功能差异。内置函数如 `len` 和 `cap` 在 `nil` slice 上按预期工作。

```go
// Good:
var s []int         // nil
fmt.Println(s)      // []
fmt.Println(len(s)) // 0
fmt.Println(cap(s)) // 0
for range s {...}   // no-op
s = append(s, 42)
```

如果你将空 slice 声明为局部变量（特别是如果它可以是返回值），优先使用 nil 初始化以减少调用者的 bug 风险。

```go
// Good:
var t []string

// Bad:
t := []string{}
```

不要创建迫使客户端区分 nil 和空 slice 的 API。

```go
// Good:
func Ping(hosts []string) ([]string, error) { ... }

// Bad:
func Ping(hosts []string) []string { ... } // nil signifies system error
```

### 16.3 缩进混淆

避免引入会使行的其余部分与缩进代码块对齐的换行。

```go
// Bad:
if longCondition1 && longCondition2 &&
    longCondition3 && longCondition4 {  // 与 if 块内的代码缩进相同
    log.Info("all conditions met")
}
```

### 16.4 函数格式化

函数或方法声明的签名应保持在一行上以避免缩进混淆。

```go
// Bad:
func (r *SomeType) SomeLongFunctionName(foo1, foo2, foo3 string,
    foo4, foo5, foo6 int) {
    foo7 := bar(foo1)
}
```

函数和方法调用不应仅基于行长度拆分。

```go
// Good:
good := foo.Call(long, list, of, parameters, all, on, one, line)

// Bad:
bad := foo.Call(long, list, of, parameters,
    with, arbitrary, line, breaks)
```

避免在特定函数参数上添加内联注释。而是使用选项结构或向函数文档添加更多细节。

```go
// Good:
good := server.New(ctx, server.Options{Port: 42})

// Bad:
bad := server.New(
    ctx,
    42, // Port
)
```

长字符串字面量不应为了行长度而拆分。

```go
// Good:
log.Warningf("Database key (%q, %d, %q) incompatible in transaction started by (%q, %d, %q)",
    currentCustomer, currentOffset, currentKey,
    txCustomer, txOffset, txKey)

// Bad:
log.Warningf("Database key (%q, %d, %q) incompatible in"+
    " transaction started by (%q, %d, %q)",
    currentCustomer, currentOffset, currentKey, txCustomer,
    txOffset, txKey)
```

### 16.5 条件和循环

`if` 语句不应换行；多行 `if` 子句可能导致缩进混淆。

```go
// Bad:
if db.CurrentStatusIs(db.InTransaction) &&
    db.ValuesEqual(db.TransactionKey(), row.Key()) {
    return db.Errorf(...)
}
```

如果不需要短路行为，可以直接提取布尔操作数：

```go
// Good:
inTransaction := db.CurrentStatusIs(db.InTransaction)
keysMatch := db.ValuesEqual(db.TransactionKey(), row.Key())
if inTransaction && keysMatch {
    return db.Error(...)
}
```

`if` 语句包含闭包或多行结构体字面量时，应确保括号匹配以避免缩进混淆。

```go
// Good:
if err := db.RunInTransaction(func(tx *db.TX) error {
    return tx.Execute(userUpdate, x, y, z)
}); err != nil {
    return fmt.Errorf("user update failed: %s", err)
}
```

同样，不要尝试在 `for` 语句中插入人工换行。

```go
// Good:
for i, max := 0, collection.Size(); i < max && !collection.HasPendingWriters(); i++ {
    // ...
}
```

`switch` 和 `case` 语句也应保持在一行上。

```go
// Good:
switch good := db.TransactionStatus(); good {
case db.TransactionStarting, db.TransactionActive, db.TransactionWaiting:
    // ...
case db.TransactionCommitted, db.NoTransaction:
    // ...
default:
    // ...
}
```

在将变量与常量比较的条件中，将变量值放在等式运算符的左侧：

```go
// Good:
if result == "foo" {
    // ...
}

// Bad:
if "foo" == result {  // Yoda style
    // ...
}
```

### 16.6 复制

为避免意外的别名和类似 bug，从其他包复制结构体时要小心。例如，同步对象如 `sync.Mutex` 不得复制。

不要复制方法集与指针类型 `*T` 关联的 `T` 类型的值。

```go
// Bad:
b1 := bytes.Buffer{}
b2 := b1
```

如果结构体包含不应复制的字段，API 作者通常应接受和返回指针类型。

### 16.7 不要 panic

不要使用 `panic` 进行正常错误处理。而是使用 `error` 和多返回值。

在 `package main` 和初始化代码中，考虑对应该终止程序的错误使用 `log.Exit`（如无效配置），因为在许多情况下堆栈跟踪对读者没有帮助。

### 16.8 Must 函数

在失败时停止程序的设置辅助函数遵循 `MustXYZ`（或 `mustXYZ`）的命名约定。通常，它们只应在程序启动早期调用，而不是在用户输入等情况下。

```go
// Good:
func MustParse(version string) *Version {
    v, err := Parse(version)
    if err != nil {
        panic(fmt.Sprintf("MustParse(%q) = _, %v", version, err))
    }
    return v
}

var DefaultVersion = MustParse("1.2.3")
```

### 16.9 Goroutine 生命周期

当你生成 goroutine 时，要清楚它们何时或是否退出。

Goroutine 可能通过在 channel 发送或接收时阻塞而泄漏。垃圾收集器不会终止在 channel 上阻塞的 goroutine，即使没有其他 goroutine 引用该 channel。

```go
// Bad:
ch := make(chan int)
ch <- 42
close(ch)
ch <- 13 // panic
```

并发代码应使 goroutine 生命周期显而易见。通常这意味着将与同步相关的代码限制在函数范围内，并将逻辑分解为同步函数。

```go
// Good:
func (w *Worker) Run(ctx context.Context) error {
    var wg sync.WaitGroup
    for item := range w.q {
        wg.Add(1)
        go func() {
            defer wg.Done()
            process(ctx, item)
        }()
    }
    wg.Wait()
}
```

### 16.10 接口（Decisions 补充）

Go 接口通常属于**消费**接口类型值的包，而不是**实现**接口类型的包。实现包应返回具体（通常是指针或结构体）类型。

不要从消费接口的 API 中导出测试替身实现。

在需要现实示例之前不要定义接口。

如果包的用户不需要为它们传递不同类型，不要使用接口类型参数。

### 16.11 泛型

允许在泛型满足业务需求的地方使用泛型。在许多应用中，使用现有语言功能（slice、map、接口等）的传统方法同样有效，因此要小心过早使用。

引入使用泛型的导出 API 时，确保它有适当的文档。强烈鼓励包含激励性的可运行示例。

不要仅仅因为你正在实现不关心成员元素类型的算法或数据结构就使用泛型。如果实践中只有一种类型被实例化，首先让你的代码在不使用泛型的情况下工作。

不要使用泛型来发明领域特定语言（DSL）。特别是，不要引入可能对读者造成重大负担的错误处理框架。

### 16.12 传递值

不要为了节省几个字节而将指针作为函数参数传递。如果函数只将其参数 `x` 读取为 `*x`，那么参数不应该是指针。常见实例包括传递字符串指针（`*string`）或接口值指针（`*io.Reader`）。

这不适用于大结构体，或即使小但可能增长的结构体。特别是，protobuf 消息通常应通过指针而非值处理。

### 16.13 接收器类型

方法接收器可以作为值或指针传递，就像它是常规函数参数一样。选择基于方法应该属于哪个方法集。

**正确性胜过速度或简单性。**

- 如果接收器是 slice 且方法不 reslice 或重新分配 slice，使用值而非指针
- 如果方法需要改变接收器，接收器必须是指针
- 如果接收器是包含不能安全复制的字段的结构体，使用指针接收器（如 `sync.Mutex`）
- 如果接收器是"大"结构体或数组，指针接收器可能更高效
- 如果接收器是内置类型（如整数或字符串）且不需要修改，使用值
- 如果接收器是 map、函数或 channel，使用值而非指针
- 如果接收器是"小"数组或结构体，没有可变字段和指针，值接收器通常是正确选择
- **不确定时，使用指针接收器**

作为一般准则，优先使类型的方法全为指针方法或全为值方法。

### 16.14 `switch` 和 `break`

不要在 `switch` 子句末尾使用无目标标签的 `break` 语句；它们是冗余的。与 C 和 Java 不同，Go 中的 `switch` 子句自动 break，需要 `fallthrough` 语句来实现 C 风格的行为。

```go
// Good:
switch x {
case "A", "B":
    buf.WriteString(x)
case "C":
    // handled outside of the switch statement
default:
    return fmt.Errorf("unknown value: %q", x)
}

// Bad:
switch x {
case "A", "B":
    buf.WriteString(x)
    break // redundant
case "C":
    break // redundant
default:
    return fmt.Errorf("unknown value: %q", x)
}
```

> **注意**: 如果 `switch` 子句在 `for` 循环内，`switch` 内的 `break` 不会退出包围的 `for` 循环。要退出包围循环，在 `for` 语句上使用标签。

### 16.15 同步函数

同步函数直接返回其结果，并在返回之前完成任何回调或 channel 操作。优先使用同步函数而非异步函数。

同步函数将 goroutine 保持在调用范围内。这有助于推理它们的生命周期，避免泄漏和数据竞争。同步函数也更容易测试。

### 16.16 类型别名

使用**类型定义**，`type T1 T2`，来定义新类型。使用**类型别名**，`type T1 = T2`，来引用现有类型而不定义新类型。类型别名很少见；其主要用途是帮助将包迁移到新的源代码位置。不需要时不要使用类型别名。

### 16.17 使用 `%q`

Go 的格式函数有 `%q` 动词，它在双引号内打印字符串。

```go
// Good:
fmt.Printf("value %q looks like English text", someText)

// Bad:
fmt.Printf("value \"%s\" looks like English text", someText)
```

### 16.18 使用 `any`

Go 1.18 引入 `any` 类型作为 `interface{}` 的别名。因为它是别名，`any` 在许多情况下等同于 `interface{}`。优先在新代码中使用 `any`。

---

## 17. 常见库

### 17.1 标志 (Flags)

Go 程序使用标准 `flag` 包的内部变体。标志名应优先使用下划线分隔单词，但持有标志值的变量应遵循标准 Go 名称风格（mixed caps）。

```go
// Good:
var (
    pollInterval = flag.Duration("poll_interval", time.Minute, "Interval to use for polling.")
)

// Bad:
var (
    poll_interval = flag.Int("pollIntervalSeconds", 60, "Interval to use for polling in seconds.")
)
```

标志只能在 `package main` 或等效包中定义。

通用包应使用 Go API 配置，而不是通过命令行接口；不要让导入库导出新的标志作为副作用。

### 17.2 日志 (Logging)

Go 程序使用标准 `log` 包的变体。对于异常程序退出，使用 `log.Fatal` 中止并带堆栈跟踪，`log.Exit` 停止而不带堆栈跟踪。没有 `log.Panic` 函数。

`log.Info(v)` 等同于 `log.Infof("%v", v)`，其他日志级别也是如此。当没有格式化要做时，优先使用非格式化版本。

### 17.3 Context

`context.Context` 类型的值跨 API 和进程边界携带安全凭证、跟踪信息、deadline 和取消信号。

传递给函数或方法时，`context.Context` 始终是第一个参数。

```go
func F(ctx context.Context /* other arguments */) {}
```

例外：
- HTTP handler 中，context 来自 `req.Context()`
- 流式 RPC 方法中，context 来自 stream
- 入口点函数中，使用 `context.Background()` 或 `tb.Context()`

不要在结构体类型中添加 context 成员。而是向需要传递它的类型的每个方法添加 context 参数。

#### 自定义 context

不要创建自定义 context 类型或在函数签名中使用 `context.Context` 之外的接口。没有例外。

### 17.4 crypto/rand

不要使用 `math/rand` 包生成密钥，即使是临时的。如果未播种，生成器完全可预测。使用 `crypto/rand` 的 Reader。

```go
// Good:
import "crypto/rand"

func Key() string {
    buf := make([]byte, 16)
    if _, err := rand.Read(buf); err != nil {
        log.Fatalf("Out of randomness, should never happen: %v", err)
    }
    return fmt.Sprintf("%x", buf)
}
```

---

## 附录：外部参考资源

以下资源在原始文档中被引用，但属于 Google 内部或外部资源，未包含在本文档中：

### Go Tips（Google 内部系列）
- Go Tip #1: Line of Sight
- Go Tip #3: Benchmarking Go Code
- Go Tip #4: Cleaning Up Your Tests
- Go Tip #5: Slimming Your Client Libraries
- Go Tip #10: Configuration Structs and Flags
- Go Tip #13: Designing Errors for Checking
- Go Tip #24: Use Case-Specific Constructions
- Go Tip #25: Subtests: Making Your Tests Lean
- Go Tip #29: Building Strings Efficiently
- Go Tip #36: Enclosing Package-Level State
- Go Tip #38: Functions as Named Types
- Go Tip #40: Improving Time Testability with Function Parameters
- Go Tip #41: Identify Function Call Parameters
- Go Tip #42: Authoring a Stub for Testing
- Go Tip #44: Improving Time Testability with Struct Fields
- Go Tip #45: Avoid Flags, Especially in Library Code
- Go Tip #48: Error Sentinel Values
- Go Tip #49: Accept Interfaces, Return Concrete Types
- Go Tip #50: Disjoint Table Tests
- Go Tip #51: Patterns for Configuration
- Go Tip #71: Reducing Parallel Test Flakiness
- Go Tip #78: Minimal Viable Interfaces
- Go Tip #80: Dependency Injection Principles
- Go Tip #81: Avoiding Resource Leaks in API Design
- Go Tip #89: When to Use Canonical Status Codes as Errors
- Go Tip #97: What's in a Name
- Go Tip #106: Error Naming Conventions
- Go Tip #108: The Power of a Good Package Name
- Go Tip #110: Don't Mix Exit With Defer
- Go Tip #117: Subtest Names

### Testing-on-the-Toilet 文章
- TotT: Identifier Naming
- TotT: Testing State vs. Testing Interactions
- TotT: Effective Testing
- TotT: Risk-driven Testing
- TotT: Change-Detector Tests Considered Harmful
- TotT: Reduce Code Complexity by Reducing Nesting
- TotT: Data Driven Traps!

### 外部文章和演讲
- Effective Go
- Go FAQ
- Go Language Specification
- Go Memory Model
- Go Data Structures
- Go Interfaces
- Go Proverbs
- Go and Dogma
- Less is exponentially more
- Esmerelda's Imagination
- Regular expressions for parsing
- Gofmt's style is no one's favorite, yet Gofmt is everyone's favorite
- Using Generics in Go (Ian Lance Taylor)
- Rethinking Classical Concurrency Patterns (Bryan Mills)
- Organizing Go Code (Blog Post & Presentation)

---

> **文档结束**  
> 本文档整合了 Google Go Style Guide 系列所有核心内容。如有疑问，请参考原始文档：https://google.github.io/styleguide/go/
