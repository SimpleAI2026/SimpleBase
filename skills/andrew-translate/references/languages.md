# 语言与地区参数速查

本文件帮助你为 `source_lang` / `target_lang` / `country` 选择合适的取值。

## source_lang / target_lang 命名约定

使用**英文全称**（与原始 translation-agent 项目一致），首字母大写：

| 中文叫法 | 推荐取值 |
| --- | --- |
| 中文 | `Chinese` |
| 英文 | `English` |
| 日文 | `Japanese` |
| 韩文 | `Korean` |
| 法语 | `French` |
| 德语 | `German` |
| 西班牙语 | `Spanish` |
| 葡萄牙语 | `Portuguese` |
| 俄语 | `Russian` |
| 阿拉伯语 | `Arabic` |
| 越南语 | `Vietnamese` |
| 泰语 | `Thai` |
| 印尼语 | `Indonesian` |
| 意大利语 | `Italian` |

> 注：使用英文全称是为了让 LLM 在 system message 里准确理解。BCP-47 代码（如 `zh-CN`）虽然规范，但模型对自然语言名称响应更稳定。

## country 取值与典型差异

`country` 字段决定译文的**地区风格**——词汇选择、拼写、口语习惯。仅在用户明确指定或上下文强烈暗示时设置。

### 中文

| country | 风格特点 |
| --- | --- |
| `China` 或留空 | 简体中文、大陆通用译法（"软件"、"网络"、"打印机"） |
| `Taiwan` | 繁体中文、台湾用语（"軟體"、"網路"、"印表機"、"專案"） |
| `Hong Kong` | 繁体中文、港式用语（"軟件"、"網絡"、"打印機"、夹杂粤语口语习惯） |
| `Singapore` | 简体中文、新加坡华语（保留部分英文专有名词、口语化） |

### 英文

| country | 风格特点 |
| --- | --- |
| `USA` 或留空 | 美式拼写（color、organize、center），美式表达 |
| `UK` | 英式拼写（colour、organise、centre），英式表达（lift / autumn / queue） |
| `Australia` | 接近英式拼写，澳洲俚语和口语 |
| `Canada` | 拼写偏英式（colour），用词偏美式 |
| `India` | 印度英语习惯表达 |

### 西班牙语

| country | 风格特点 |
| --- | --- |
| `Spain` | 半岛西班牙语，使用 *vosotros*，词汇如 *coche* / *ordenador* |
| `Mexico` | 拉美西班牙语，使用 *ustedes*，词汇如 *carro* / *computadora* |
| `Argentina` | 阿根廷西班牙语，*vos* 代替 *tú*，独特口音词汇 |
| `Colombia` / `Chile` / `Peru` | 各自地区用词差异 |

### 葡萄牙语

| country | 风格特点 |
| --- | --- |
| `Brazil` | 巴西葡语（*você*、*trem*=火车、动名词进行时） |
| `Portugal` | 欧洲葡语（*tu*、*comboio*=火车、不定式进行时） |

### 法语

| country | 风格特点 |
| --- | --- |
| `France` | 法国本土法语 |
| `Canada` (Quebec) | 魁北克法语，词汇与口语差异较大 |

## 选择 country 的判断逻辑

1. **用户明确指定** → 直接用。例：用户说"翻成繁体"或"台湾用语" → `country = "Taiwan"`。
2. **用户给出上下文线索** → 推断。例：用户说"我在伦敦工作要写邮件" → `country = "UK"`。
3. **不确定时** → 留空。空字符串会触发 `references/prompts.md` 中的 2B 版反思提示词，不强制特定地区风格。

## 模型不擅长的小语种

对于训练数据较少的语种（部分非洲语言、少数民族语言、古典语言），三步翻译仍能提升质量，但**反思步骤可能给出泛泛建议**。这种情况下可考虑：

- 在 system message 中追加领域知识或风格示例。
- 让用户提供 1-2 个高质量参考译文样例放进 prompt。
- 接受质量上限低于主流语种的现实，并在交付时提示用户复核。
