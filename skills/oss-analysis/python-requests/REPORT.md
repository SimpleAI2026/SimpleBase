# psf/requests：simplify-code 分析报告

- **仓库**：https://github.com/psf/requests
- **Commit**：`611c6162cbc4ac2020a2f91c7cfa4f3abf9bbb60`（浅克隆）
- **语言/版本约束**：Python ≥ 3.10（`pyproject.toml`），无 AGENTS.md/CLAUDE.md；风格由 ruff/pre-commit 管控
- **范围**：`src/requests/utils.py`（1155 行，全文通读）。sessions.py / models.py 未纳入（超出时间预算）
- **技能参考**：SKILL.md 工作流 + `rules.md` §5/§7/§9/§10/§20 + `languages-tier1.md#Python` + `behavior-traps.md`

## Summary
`utils.py` 已有数十年的打磨，大部分函数是"每个分支都是独立契约"的最小形态。只找到 1 个值得改的 P2（嵌套 + 重复的 IPv4 校验）、2 个 P3 和 1 个"顺手修复"性质的资源泄漏。已在本地分支应用其中 2 处并跑测试，行为不变。

## 代码在做什么
`utils.py` 是 requests 的公共工具箱：代理旁路判断（`should_bypass_proxies`、`is_valid_cidr`、`address_in_network`）、netrc 认证、header 解析、编码猜测、URL 重新引用等。它通过 `requests.utils` 对外暴露，**所有函数名、返回类型都是公共 API**。

## 发现（按优先级）

### P2-1 `is_valid_cidr` 嵌套 + 重复知识 — `utils.py:763-784`
- **规则**：rules §5 卫语句/去嵌套；§6 DRY（"IPv4 是否合法"已由 `is_ipv4_address` 表达）
- **原因**：主逻辑包在 `if count == 1:` 里，`else: return False` 在最后；第二个 `try/except OSError` 与同文件 `is_ipv4_address`（752-760）完全相同。
- **Before**
  ```python
  if string_network.count("/") == 1:
      try:
          mask = int(string_network.split("/")[1])
      except ValueError:
          return False
      if mask < 1 or mask > 32:
          return False
      try:
          socket.inet_aton(string_network.split("/")[0])
      except OSError:
          return False
  else:
      return False
  return True
  ```
- **After**
  ```python
  if string_network.count("/") != 1:
      return False
  try:
      mask = int(string_network.split("/")[1])
  except ValueError:
      return False
  if mask < 1 or mask > 32:
      return False
  return is_ipv4_address(string_network.split("/")[0])
  ```
- **风险**：低。`is_ipv4_address` 捕获的也是 `OSError`、返回同样的 bool；分支顺序（先 mask 后地址）保持不变。
- **行为保持**：是（已应用 + 测试）。

### P3-1 `if bypass: return True / return False` — `utils.py:867-870`
- **规则**：rules §9 / §20.5 "直接返回布尔"；behavior-traps "只有在原值已经是 bool 时才能 `return cond`"
- **陷阱**：`bypass` 来自 `proxy_bypass(...)`（urllib / Windows 注册表 / macOS sysconf 实现），历史上部分实现返回 `1/0` 而非 bool。直接 `return bypass` 会把公共函数 `should_bypass_proxies` 的返回值从 `True` 变成 `1`。
- **After**：`return bool(bypass)`（保留强制转换）。
- **风险**：低；**行为保持**：是（已应用 + 测试）。

### P3-2 `address_in_network` 重复掩码 — `utils.py:737-738`
- **规则**：§20.3 冗余运算
- `network = unpack(...)[0] & netmask` 之后又 `(network & netmask)`，`&` 幂等，第二次无意义：
  ```python
  # After
  return (ipaddr & netmask) == network
  ```
- **风险**：极低；行为保持；未应用（收益很小，留作建议）。

### 顺手修复（需评审）`extract_zipped_paths` 未关闭 ZipFile — `utils.py:313`
- **规则**：§10 资源用 `with`；examples A33 "bonus fix"
- `zip_file = zipfile.ZipFile(archive)` 从未 close，依赖 GC。改为 `with zipfile.ZipFile(archive) as zip_file:` 包住 `namelist()`/`read()`。
- **风险**：低，但它改变了文件句柄生命周期（在 Windows 上属于可观察差异），因此按技能要求作为"bonus fix 单独报告"，不混进纯简化。

## 刻意保留（及原因）
- `super_len`（160-228）：分支多但每条对应一类 file-like 对象（`__len__`/`.len`/`fileno`/`tell+seek`），合并会改变优先级。
- `get_netrc_auth` 的宽泛 `except (ImportError, AttributeError): pass`（278-280）：注释写明是 App Engine 兼容，属于有意吞错，不能"修"。
- `from_key_val_list`/`to_key_val_list` 重复的类型守卫（364、398）：两份，且未来可能分开演化（rule of three 未到）。
- `get_environ_proxies` 中 `else` 跟在 `return` 后（879-882）：纯风格，ruff 未报，改了只制造 churn。
- `select_proxy` 的 loop+break（902-908）：可改成循环内 `return`，但收益极低。
- `_null/_null2/_null3` 模块级常量（1003-1005）：有注释说明避免重复创建，保留。

## 验证
- 分支 `simplify-demo`，diff 见 `applied.diff`（+10/−17）。
- `pytest tests/test_utils.py`：修改前后均 **216 passed, 13 skipped**。
- 全量 `pytest tests`：修改前后均 **607 passed, 12 failed, 15 skipped, 1 xfailed**。12 个失败是沙箱代理/网络环境导致（如 `test_use_proxy_from_environment`），修改前即失败，数量一致（未逐条比对失败集合）。

## 置信度
高（P2-1、P3-1 有测试覆盖 + 逐行比对）；P3-2 与 zip 句柄靠推理。
