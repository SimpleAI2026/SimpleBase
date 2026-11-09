# Alamofire/Alamofire（`Core/ParameterEncoding.swift`）：simplify-code 分析报告

- **仓库**：https://github.com/Alamofire/Alamofire
- **Commit**：`bda9ed57d72988a3a2ada33d824583541f86eac6`
- **范围**：`Source/Core/ParameterEncoding.swift`（349 行，全文）
- **约束**：Swift 6 工具链（多个 `Package@swift-6.x.swift`），`.swiftformat`；已普遍使用 `guard let x`、隐式 `switch` 表达式返回
- **技能参考**：`languages-tier1.md#Swift`、`rules.md` §6/§20、examples A17

## Summary
发现 1 处整函数级复制粘贴（P1，可安全消除），以及 2 个 P3。box 无 Swift 工具链，未运行测试。

## 代码在做什么
`URLEncoding` 把参数字典编码到查询串或表单体（数组/布尔编码策略可配，键排序保证确定性）；`JSONEncoding` 把参数或任意 JSON 对象序列化进 body 并在缺省时设置 `Content-Type: application/json`。均为公共 API。

## 发现（按优先级）

### P1-1 `JSONEncoding` 两个 `encode` 方法逐行重复 — `ParameterEncoding.swift:275-297` 与 `307-329`
- **规则**：§6 DRY（同一错误映射、同一 header 规则、同一 body 写入）
- 两个方法除参数名（`parameters` vs `jsonObject`）外完全相同。
- **After**
  ```swift
  public func encode(_ urlRequest: any URLRequestConvertible, with parameters: Parameters?) throws -> URLRequest {
      try encode(urlRequest, withJSONObject: parameters)
  }
  ```
- **等价性检查**：`Parameters?` 传给 `Any?` 时，`nil` 仍为 `nil`（不会变成"装着 nil 的 Any"），因此 `guard let` 早退路径一致；`asURLRequest()` 仍只调用一次；`isValidJSONObject` 接收的仍是同一个桥接字典；抛出的 `AFError` 完全相同。
- **风险**：低；公共签名不变；**行为保持**：是（推理，未编译）。

### P3-1 `query()` 中的强制解包 — `ParameterEncoding.swift:233-235`
```swift
// Before
for key in parameters.keys.sorted(by: <) {
    let value = parameters[key]!
    components += queryComponents(fromKey: key, value: value)
}
// After
for (key, value) in parameters.sorted(by: { $0.key < $1.key }) {
    components += queryComponents(fromKey: key, value: value)
}
```
字典键唯一，排序稳定性无影响，顺序相同；去掉了 `!`。

### P3-2 `NSNumber` 分支重复 append — `ParameterEncoding.swift:207-212`
```swift
case let number as NSNumber:
    let string = number.isBool ? boolEncoding.encode(value: number.boolValue) : "\(number)"
    components.append((escape(key), escape(string)))
```
单层三元、无副作用，可读性略好；可选。

## 刻意保留
- `case let bool as Bool` 分支（213-214）：在 Linux（swift-corelibs-foundation）上 `Bool` 不一定桥接为 `NSNumber`，与上一分支并非重复——删除会改变非 Darwin 平台行为。
- `Destination`/`ArrayEncoding`/`BoolEncoding` 的 `switch` 表达式：已是最小形式。
- `escape` 的 `?? string` 回退：保留编码失败时的原样输出契约。

## 验证
- 仅阅读与推理；box 无 Swift 工具链（Linux 上虽可装，但 Alamofire 测试依赖网络 httpbin），未编译、未运行。

## 置信度
中高（P1-1 的 Optional→Any 语义是唯一需要编译器确认的点）。
