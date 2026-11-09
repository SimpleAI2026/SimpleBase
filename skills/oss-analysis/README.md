# simplify-code 技能 × 开源项目分析索引

- **技能**：`/workspace/user-skills/simplify-code/`（SKILL.md + references 全部读完后执行）
- **日期**：2026-10-14（Asia/Shanghai）
- **方式**：每个仓库浅克隆并固定 commit；**只做分析**，没有 PR，也没有向上游推送任何内容。可以落地的建议在本地分支 `simplify-demo` 上实际修改并跑测试，diff 放在各目录的 `applied.diff`。
- **优先级口径**：技能本身没有定义优先级，这里自定如下。P1 = 整段重复或复杂度明显超标，收益高；P2 = 真实的重复知识、嵌套或死代码，改动低风险；P3 = 可选的局部可读性改进。"缺陷/需审批"指分析中顺带发现的 bug 或会改变行为的点，不算简化。

| 语言 | 仓库 | Commit | 分析文件 | P1 | P2 | P3 | 缺陷/需审批 | 跑了测试？ | 备注 |
|---|---|---|---|---|---|---|---|---|---|
| Python | [psf/requests](https://github.com/psf/requests) | `611c6162` | `src/requests/utils.py` | 0 | 1 | 2 | 1（ZipFile 未关闭，属于顺手修复） | ✅ 改了 2 处；test_utils 216 通过，全量前后都是 607 通过 / 12 失败（沙箱网络问题） | `return bool(bypass)` 必须保留强制转换 |
| JavaScript | [expressjs/express](https://github.com/expressjs/express) | `a419aa73` | `lib/utils.js`、`lib/view.js` | 0 | 2 | 2 | 0 | ✅ 改了 2 处；1257 通过（前后一致），eslint 干净 | rest 参数与文件的 ES5 风格是否一致，需要维护者定 |
| TypeScript | [sindresorhus/ky](https://github.com/sindresorhus/ky) | `35418888` | `source/utils/merge.ts`、`normalize.ts` | 1（仅提议） | 0 | 3 | 0 | ⚠️ 改了 1 处；6 个测试文件前后都是 418 通过 / 10 失败，失败集合一致（Node 20 低于项目要求的 22） | 去掉嵌套三元后触发 xo `max-depth`，已回退 |
| Go | [spf13/cobra](https://github.com/spf13/cobra) | `adbc8813` | `args.go`、`cobra.go`、`command.go`（usage） | 0 | 1 | 2 | 1（`findSuggestions(args[0])`） | ✅ 改了 2 处；`go test ./...` 通过 | `go 1.15` 不允许用 `slices` 和 `min`；`appendIfNotPresent` 在模板里动态注册，不能删 |
| Rust | [BurntSushi/ripgrep](https://github.com/BurntSushi/ripgrep) | `3fce3b5b` | `crates/cli/src/*`（6 个文件） | 0 | 0 | 3 | 0 | ✅ 改了 14+2 处；28 个测试通过（用了 `--ignore-rust-version`） | box 上没有 rustfmt/clippy，格式靠手工核对 |
| Java | [google/guava](https://github.com/google/guava) | `2b86a821` | `base/Strings.java`、`base/Joiner.java` | 0 | 0 | 0 | 0 | ➖ 没有改动，所以没测 | 代码已经是最简形态；Java 8、GWT、Android 的兼容要求否决了所有"现代化"写法 |
| C | [antirez/kilo](https://github.com/antirez/kilo) | `323d93b2` | `kilo.c` 366–865 行 | 0 | 1 | 5 | 1（`editorDelRow` 里 idx 方向错误，可越界读） | ✅ 改了 4 处；自写特征测试，输出 md5 逐字节一致，`-Wall -W -pedantic` 0 告警 | 缺陷有复现程序 `bug.c` |
| C++ | [nlohmann/json](https://github.com/nlohmann/json) | `7461a199` | `string_escape.hpp`、`value_t.hpp`、`string_utils.hpp` | 0 | 0 | 2 | 0 | ⚠️ 只编译了单个测试单元：候选改动让 alt-string 测试编译失败，已回退 | 自定义 `StringType` 的最小接口本身就是 API |
| C# | [JamesNK/Newtonsoft.Json](https://github.com/JamesNK/Newtonsoft.Json) | `52fa3aef` | `Utilities/StringUtils.cs` | 0 | 1 | 4 | 0 | ❌ box 上没有 dotnet | LINQ 延迟执行会多次调用 selector，不能随手 `ToList()` |
| PHP | [guzzle/guzzle](https://github.com/guzzle/guzzle) | `93939470` | `src/Utils.php` | 0 | 1 | 2 | 0 | ❌ box 上没有 php | 同一个校验函数被调用了 3 次 |
| Ruby | [sinatra/sinatra](https://github.com/sinatra/sinatra) | `cb22afd7` | `lib/sinatra/base.rb`（Request、`uri`、状态谓词） | 0 | 0 | 3 | 1（`respond_to_missing?`）+ 1 条公共 API 记录 | ❌ box 上没有 ruby | `detect` 改成 `each` 时必须补 `nil` |
| Kotlin | [square/okio](https://github.com/square/okio) | `a6ae13ee` | `internal/Path.kt` | 0 | 1 | 2 | 0 | ❌ 多平台 Gradle 构建太重 | 不能用 `isLetter()`，它会接受非 ASCII 字母 |
| Swift | [Alamofire/Alamofire](https://github.com/Alamofire/Alamofire) | `bda9ed57` | `Core/ParameterEncoding.swift` | 1 | 0 | 2 | 0 | ❌ box 上没有 swift | `JSONEncoding` 里两个 `encode` 整段重复 |
| Bash | [nvm-sh/nvm](https://github.com/nvm-sh/nvm) | `b1b3a003` | `nvm.sh` 1–140 行 | 0 | 1（需审批） | 2 | 0 | ✅ 改了 1 处；官方单测在 bash、dash 下通过，另做了对比脚本 | stdout 不变，但命令不存在时 stderr 从 5 行变成 1 行 |
| SQL | （可选，未做） | – | – | – | – | – | – | – | – |

**合计**：14 个仓库。P1 2 条（其中 1 条仅提议），P2 9 条，P3 34 条；缺陷/需审批 4 条，另有 1 条公共 API 记录（sinatra `uri`）。在 8 个仓库里实际改了代码并验证：requests、express、ky、cobra、ripgrep、kilo、nvm 保留了改动，json 的候选改动在验证中被否决并回退。

## 总体观察
1. 成熟项目里真正值得改的东西很少：guava 是 0 条，ripgrep 和 json 只有 P3。这和技能的"Already simple → 什么都不改"模式是一致的。
2. 这次价值最高的产出，是验证时被推翻的"看似安全的简化"：
   - json：`find_first_of("~/")` 让自定义字符串类型的测试编译失败。
   - ky：去掉嵌套三元后违反了项目的 lint 规则。
   - nvm：`type` 只调一次后，stderr 输出次数变了。
3. 分析中顺带发现了 2 个真实缺陷：kilo 的 `idx` 越界，cobra 的错误提示针对了错的参数。技能说明自己不用于修 bug，所以这里只记录、没有修改。

## 这个技能好用的地方
- **语言版本门槛**（"项目的语言版本必须支持该语法"）在 cobra（go 1.15）、guava（Java 8/GWT）、guzzle（PHP 7.4）、ripgrep（MSRV）上都直接挡掉了错误建议。
- **behavior-traps 清单**很有用。命中的例子：Python 里 `return cond` 要求 cond 本来就是 bool；C# LINQ 的延迟执行；Kotlin/Ruby 标准库函数语义不完全相同。
- **"动态注册不能删"**这条规则直接救了 cobra 的 `appendIfNotPresent`。
- **"先补特征测试"**这一步让没有测试的 kilo 也能做到字节级验证。

## 技能的缺口（建议补充）
1. **缺少"审计 / 只分析"模式。** 
   - golden rule 6 规定只动用户指定的代码或本次会话的 diff，工作流也默认"改完再报告"。
   - 但对第三方仓库做审计时，需要的是"选范围 → 排优先级 → 只提议"。
   - 输出格式里也没有优先级、风险、文件:行号、是否需审批这些字段，这次只能自己定义。
2. **技能规则和项目 lint 冲突时怎么办，没有说。** ky 的例子：golden rule 4（不要嵌套三元）和项目的 xo `max-depth` 冲突。建议明确写上"项目 lint 优先；需要的话先拆函数"。
3. **behavior-traps 缺少"泛型/模板参数的最小接口"这一类。** C++ 的 `StringType`、鸭子类型参数都属于这种情况：标准库有某个重载，不代表用户类型也有。
4. **缺少"去重会改变副作用次数"这一类陷阱。** nvm 的例子：合并重复的命令调用后，stderr 输出次数变了。现有的 rules §20.3 只说了"不要通过内联重复执行有副作用的表达式"，反方向（合并）也同样会改变行为。
5. **缺少"顺带发现的 bug"的报告位置。** 技能说明不用于修 bug，但实际一定会遇到 bug（kilo、cobra）。建议在输出格式里加一个 `Bugs found (not fixed)` 小节。
6. **Ruby（tier 2）可以补一条：`each` 返回的是接收者本身**，所以用 `detect`/`map` 做控制流时，改写要注意最终返回值。
7. **工具链不可用或版本过旧时的验证降级没有指引**，比如没有 rustfmt、Node 版本低于项目要求、没有 dotnet/php。建议写明：只靠推理的结论要标出来；能编译单个测试单元时，优先这样验证。
8. **条件编译 / 多平台代码没有提到。** 比如 C# 的 `#if HAVE_*`、Rust 的 `cfg`、KMP 的 `expect/actual`：本机编译通过，不代表所有分支都被编译过。
9. **大仓库怎么选范围没有指引。** 这次是按"公共工具文件 + 近期改动较多的模块"挑的，技能里可以加一句启发式规则。
