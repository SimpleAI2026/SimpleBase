# sinatra/sinatra：simplify-code 分析报告

- **仓库**：https://github.com/sinatra/sinatra
- **Commit**：`cb22afd7902b566b6eaba6c4ea89739494a65d12`
- **范围**：`lib/sinatra/base.rb` 中 `Sinatra::Request` 及 `AcceptEntry`（31-135 行）、`Helpers#uri`（326-344 行）、状态谓词（656-690 行）
- **约束**：RuboCop（`.rubocop.yml`）；`base.rb` 共 2173 行（god file，但拆分不在本次范围）
- **技能参考**：`languages-tier2.md#Ruby`、`rules.md` §2/§9/§20、`behavior-traps.md`

## Summary
找到 2 个可安全应用的 P3，1 个需要审批的 Ruby 元编程惯用法问题，以及 1 个公共 API 层面的 flag 参数（只记录）。box 无 Ruby，未运行测试。

## 代码在做什么
`Request#accept` 解析并按优先级排序 Accept 头（结果缓存在 `env['sinatra.accept']`）；`preferred_type` 做内容协商；`AcceptEntry` 通过 `method_missing` 伪装成字符串；`uri/url/to` 生成绝对或相对 URL。

## 发现（按优先级）

### P3-1 冗余的 `include?` 检查 — `base.rb:37`
```ruby
# Before
@env['sinatra.accept'] ||= if @env.include?('HTTP_ACCEPT') && (@env['HTTP_ACCEPT'].to_s != '')
# After
@env['sinatra.accept'] ||= if @env['HTTP_ACCEPT'].to_s != ''
```
键不存在时 `@env['HTTP_ACCEPT']` 为 `nil`，`nil.to_s == ''`，结果相同。前提：Rack env 是普通 `Hash`（无 default proc）——Rack 规范保证。**行为保持**：是（推理）。

### P3-2 用 `detect` 做提前返回 — `base.rb:58-61`
```ruby
accept.detect do |accept_header|
  type = types.detect { |t| MimeTypeEntry.new(t).accepts?(accept_header) }
  return type if type
end
```
外层 `detect` 的块永远返回 `nil`，它实际只是 `each` + 非局部 `return`。改写：
```ruby
accept.each do |accept_header|
  type = types.detect { |t| MimeTypeEntry.new(t).accepts?(accept_header) }
  return type if type
end
nil
```
**陷阱**：`each` 返回接收者本身（`accept` 数组），所以**必须**补上末尾 `nil`，否则无匹配时 `preferred_type` 会返回数组而不是 `nil`（`accept?` 会因此变真）。这正是 behavior-traps "哪个具体输入会让新旧不同"要抓的点。

### 需审批 `AcceptEntry#respond_to?` + `method_missing` — `base.rb:128-134`
Ruby 风格指南建议实现 `respond_to_missing?` 而非覆写 `respond_to?`，否则 `method(:upcase)` 等反射行为不一致。改写会改变 `method`/`respond_to?` 的可观察结果 → 提议而非直接改。

### 记录（公共 API，不改）`uri(addr = nil, absolute = true, add_script_name = true)` — `base.rb:326`
两个位置布尔参数（rules §2 flag arguments / examples B1），调用处 `uri('/x', false, false)` 难读。但它是 Sinatra 公共 helper（还有 `url`、`to` 别名），只能在大版本中引入关键字参数并保留兼容。

### P3-3（可选）`uri = [host = String.new]` — `base.rb:329`
在数组字面量中顺带赋值，读者容易忽略 `host` 与 `uri[0]` 是同一对象（后续 `host <<` 依赖这一点）。拆成两行 `host = String.new; uri = [host]` 更清楚，语义相同。

## 刻意保留
- `informational?` … `server_error?` 五个谓词（656-680）：形式相同但各自是公开命名的 HTTP 语义，元编程生成会降低可搜索性（rules §1 可搜索名称）。
- `AcceptEntry#initialize` 的参数解析：一次 `scan` + `map!`，已紧凑。
- `params` 的两个 `rescue`：不同异常映射为不同用户可见文案。

## 验证
- 仅阅读与推理；box 无 Ruby，未运行 `rake test`。

## 置信度
中（无运行验证）；P3-2 的陷阱分析高置信。
