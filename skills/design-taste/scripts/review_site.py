#!/usr/bin/env python3
"""Capture a web page and compute design-taste metrics.

Usage:
  python3 review_site.py <url> [--out DIR] [--industry NAME] [--mobile] [--full-max 8000]

Outputs (in --out, default ./review-<host>):
  fold.png          1440x900 first viewport
  full.png          full page, clipped to --full-max px
  mobile.png        390x844 first viewport (only with --mobile)
  hero.png          largest image/video/background-image in the first viewport (if >= 12% of it)
  hero-image/       review_image.py output for hero.png (needs Pillow + numpy; skipped otherwise)
  metrics.json      raw metrics + metric sub-scores + metric_score
  report.md         report skeleton (--lang en|zh); the agent fills the visual score

Dependency: playwright only.
  pip install playwright && python3 -m playwright install chromium
The script never downloads anything itself.
Score = 45% metric (computed here) + 55% visual (agent judges screenshots).
"""
import argparse, asyncio, csv, json, os, statistics, sys
from urllib.parse import urlparse

LANG = "en"  # default report language for this copy; override with --lang en|zh


def T(zh, en):
    return zh if LANG == "zh" else en

try:
    from playwright.async_api import async_playwright
except ImportError:
    sys.exit("playwright missing: pip install playwright && python3 -m playwright install chromium")

W, H = 1440, 900
UA = ("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 "
      "(KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36")
HERE = os.path.dirname(os.path.abspath(__file__))

# Metrics over visible elements that carry their own text. VW/VH = fold box.
JS = r"""
([VW, VH]) => {
  const parse = c => { const m = c && c.match(/rgba?\(([^)]+)\)/); return m ? m[1].split(',').map(parseFloat) : null; };
  const lum = p => { const f = v => { v /= 255; return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4); };
                     return 0.2126 * f(p[0]) + 0.7152 * f(p[1]) + 0.0722 * f(p[2]); };
  const ratio = (a, b) => { const x = lum(a), y = lum(b); return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05); };
  const bgOf = e => { for (; e; e = e.parentElement) { const p = parse(getComputedStyle(e).backgroundColor);
                      if (p && (p.length < 4 || p[3] > 0.5)) return p; } return [255, 255, 255]; };
  const visible = e => { const r = e.getBoundingClientRect(); return r.width > 0 && r.height > 0; };
  const ownText = e => [...e.childNodes].filter(n => n.nodeType === 3).map(n => n.textContent.trim()).join(' ').trim();
  const inFold = r => Math.max(0, Math.min(r.right, VW) - Math.max(r.left, 0)) * Math.max(0, Math.min(r.bottom, VH) - Math.max(r.top, 0));

  const T = new Set(), B = new Set(), BO = new Set(), F = new Set(), FW = new Set(), R = new Set(), S = new Set();
  const FS = {}; const body = []; let grad = 0, maxW = 0, hero = 0, heroText = '', media = 0;
  const els = [...document.querySelectorAll('body *')].slice(0, 6000);
  for (const e of els) {
    if (!visible(e)) continue;
    const s = getComputedStyle(e), r = e.getBoundingClientRect(), t = ownText(e);
    if (t.length > 1) {
      T.add(s.color); FW.add(s.fontWeight);
      F.add(s.fontFamily.split(',')[0].replace(/["']/g, '').trim().toLowerCase());
      FS[s.fontSize] = (FS[s.fontSize] || 0) + 1;
      const fs = parseFloat(s.fontSize), c = parse(s.color);
      if (t.length > 40 && fs <= 20 && c) body.push([ratio(c, bgOf(e)), fs, t.length]);
      if (t.length > 2 && r.top < VH && r.bottom > 0 && fs > hero) { hero = fs; heroText = e.textContent.trim().slice(0, 80); }
    }
    const bp = parse(s.backgroundColor); if (!(bp && bp.length === 4 && bp[3] === 0)) B.add(s.backgroundColor);
    if (parseFloat(s.borderTopWidth) > 0 && s.borderTopStyle !== 'none') BO.add(s.borderTopColor);
    if (s.borderTopLeftRadius !== '0px') R.add(s.borderTopLeftRadius);
    if (s.boxShadow !== 'none') S.add(s.boxShadow);
    if (s.backgroundImage.includes('gradient')) grad++;
    if (s.backgroundImage.includes('url(') && inFold(r) > VW * VH * 0.2) media += inFold(r);
    if (['MAIN', 'DIV', 'SECTION'].includes(e.tagName) && s.maxWidth.endsWith('px')) {
      const mw = parseFloat(s.maxWidth); if (mw >= 600 && mw <= 2600 && r.width > 500) maxW = Math.max(maxW, mw); }
  }
  for (const e of document.querySelectorAll('img,video,picture,canvas,svg')) {
    const a = inFold(e.getBoundingClientRect()); if (a > VW * VH * 0.05) media += a; }
  const chars = {}; body.forEach(([, fs, n]) => chars[fs] = (chars[fs] || 0) + n);
  const top = Object.entries(chars).sort((a, b) => b[1] - a[1])[0];
  const total = body.reduce((a, x) => a + x[2], 0);
  const sizes = Object.entries(FS).sort((a, b) => b[1] - a[1]);
  return {
    title: document.title, finalUrl: location.href,
    textColors: T.size, bgColors: B.size, borderColors: BO.size,
    fonts: [...F].slice(0, 12), fontCount: F.size,
    fontSizes: sizes.length, fontSizesSig: sizes.filter(([, n]) => n >= 2).length,
    topFontSizes: sizes.slice(0, 10).map(([k, n]) => k + '×' + n),
    fontWeights: FW.size, radiiList: [...R].slice(0, 12), radii: R.size, shadows: S.size, gradients: grad,
    maxWidth: maxW, heroSize: hero, heroText,
    bodySize: top ? parseFloat(top[0]) : null,
    contrast: total ? body.reduce((a, x) => a + x[0] * x[2], 0) / total : null,
    lowContrastShare: total ? body.filter(x => x[0] < 4.5).reduce((a, x) => a + x[2], 0) / total : null,
    heroMediaRatio: Math.min(1, media / (VW * VH)), heroMedia: media / (VW * VH) > 0.35,
    pageHeight: document.documentElement.scrollHeight, nEls: els.length,
    textColorList: [...T].slice(0, 20), bgColorList: [...B].slice(0, 20),
  };
}
"""

CONSENT_TEXT = ["Accept all", "Accept All", "Accept All Cookies", "Accept all cookies", "Accept Cookies",
                "Accept", "I agree", "Agree", "Allow all", "Allow All", "Got it", "OK", "I Accept",
                "Accept and continue", "Agree and continue", "接受", "同意", "全部接受", "我知道了"]
CONSENT_SEL = ["#onetrust-accept-btn-handler", "#truste-consent-button", ".cc-allow", "#didomi-notice-agree-button"]


async def dismiss_consent(page):
    for t in CONSENT_TEXT:
        for fr in page.frames:
            try:
                b = fr.get_by_role("button", name=t, exact=True)
                if await b.count() and await b.first.is_visible():
                    await b.first.click(timeout=1500); await page.wait_for_timeout(800); return t
            except Exception:
                pass
    for sel in CONSENT_SEL:
        try:
            b = page.locator(sel)
            if await b.count() and await b.first.is_visible():
                await b.first.click(timeout=1500); await page.wait_for_timeout(800); return sel
        except Exception:
            pass
    return ""


async def settle(page, url):
    resp = await page.goto(url, wait_until="load", timeout=60000)
    try:
        await page.wait_for_load_state("networkidle", timeout=12000)
    except Exception:
        pass
    await page.wait_for_timeout(2000)
    return resp


def clamp(x):
    return max(0.0, min(100.0, x))


def score_metrics(m):
    """Same formula as the top-100 benchmark runs (comparable with benchmarks.csv)."""
    ncol = m["textColors"] + m["bgColors"] + m["borderColors"]
    color = clamp(100 - max(0, ncol - 12) * 5 - max(0, m["gradients"] - 3) * 1.5)
    typ = clamp(100 - max(0, m["fontSizesSig"] - 7) * 10 - max(0, m["fontCount"] - 2) * 12
                - max(0, m["fontWeights"] - 3) * 8)
    cons = clamp(100 - max(0, m["radii"] - 3) * 8 - max(0, m["shadows"] - 3) * 10)
    hb = m["heroSize"] / (m["bodySize"] or 16)
    hier = clamp(40 + min(hb, 4) * 15)
    if m["bodySize"] is None:
        acc = 50.0
    else:
        acc = clamp(min(m["contrast"] or 0, 12) / 12 * 70 + 30 - (m["lowContrastShare"] or 0) * 60
                    - (15 if m["bodySize"] < 14 else 0))
    metric = 0.25 * color + 0.25 * typ + 0.15 * cons + 0.15 * hier + 0.20 * acc
    return {"colorsTotal": ncol, "heroBodyRatio": round(hb, 2),
            "m_color": round(color), "m_type": round(typ), "m_consist": round(cons),
            "m_hier": round(hier), "m_access": round(acc), "metric_score": round(metric, 1)}


# Targets = median of top-20 sites across both benchmark runs (see references/benchmarks.md).
TARGETS = [("colorsTotal", ("颜色总数", "Total colors"), "≤15", lambda v: v <= 15),
           ("fontCount", ("字体族", "Font families"), "≤2", lambda v: v <= 2),
           ("fontSizesSig", ("字号档", "Font sizes"), "≤7", lambda v: v <= 7),
           ("fontWeights", ("字重", "Font weights"), "≤3", lambda v: v <= 3),
           ("radii", ("圆角值", "Radii"), "≤3", lambda v: v <= 3),
           ("shadows", ("阴影值", "Shadows"), "≤1", lambda v: v <= 1),
           ("gradients", ("渐变", "Gradients"), "≤1", lambda v: v <= 1),
           ("heroSize", ("Hero 标题 px", "Hero title px"), "≥56", lambda v: v >= 56),
           ("bodySize", ("正文 px", "Body px"), "≥16", lambda v: v is not None and v >= 16),
           ("heroBodyRatio", ("Hero/正文比", "Hero/body ratio"), "≥3.5", lambda v: v >= 3.5),
           ("contrast", ("正文对比度", "Body contrast"), "≥12", lambda v: v is not None and v >= 12),
           ("lowContrastShare", ("低于 AA 文本占比", "Text below AA"), "0%", lambda v: v is not None and v <= 0.02)]


def load_bench(industry):
    path = os.path.join(HERE, "benchmarks.csv")
    if not os.path.exists(path):
        return None
    rows = list(csv.DictReader(open(path, encoding="utf-8")))
    peers = [r for r in rows if industry and r["industry"].lower() == industry.lower()]
    rank_in = lambda score, pool: 1 + sum(float(r["score"]) > score for r in pool)
    return rows, peers, rank_in


def fmt(v):
    if v is None:
        return "-"
    if isinstance(v, float):
        return f"{v:.2f}".rstrip("0").rstrip(".")
    return str(v)


HERO_JS = """([W, H]) => {
  let best = null, bestA = 0;
  for (const el of document.querySelectorAll('img,video,picture,canvas,svg,[style*="background"],section,header,div')) {
    const cs = getComputedStyle(el);
    const media = /^(IMG|VIDEO|PICTURE|CANVAS|svg)$/i.test(el.tagName) || (cs.backgroundImage && cs.backgroundImage.includes('url('));
    if (!media || cs.visibility === 'hidden' || +cs.opacity === 0) continue;
    const r = el.getBoundingClientRect();
    const x = Math.max(0, r.left), y = Math.max(0, r.top), x2 = Math.min(W, r.right), y2 = Math.min(H, r.bottom);
    const a = Math.max(0, x2 - x) * Math.max(0, y2 - y);
    if (a > bestA) { bestA = a; best = {x, y, width: x2 - x, height: y2 - y}; }
  }
  return best && bestA >= 0.12 * W * H ? best : null;
}"""


def review_hero(out):
    path = os.path.join(out, "hero.png")
    if not os.path.exists(path):
        return None
    try:
        sys.path.insert(0, HERE)
        import review_image
        m, _ = review_image.run(path, os.path.join(out, "hero-image"), "photo", lang=LANG)
        return {k: m[k] for k in ("metric_score", "sub_scores", "palette", "harmony_guess", "flags", "width", "height")}
    except Exception as e:  # Pillow/numpy missing or odd image
        return {"error": str(e)[:200]}


def build_report(url, m, s, industry, mobile, hero=None):
    return (_report_zh if LANG == "zh" else _report_en)(url, m, s, industry, mobile, hero)


def _metric_rows(m, s, zh):
    out = []
    for key, names, target, ok in TARGETS:
        v = s.get(key, m.get(key))
        shown = f"{v:.0%}" if key == "lowContrastShare" and v is not None else fmt(v)
        out.append(f"| {names[0] if zh else names[1]} | {shown} | {target} | {'✅' if ok(v) else '⚠️'} |")
    return out


def _report_zh(url, m, s, industry, mobile, hero=None):
    lines = [f"# 网站设计评审：{m.get('title') or url}", "",
             f"- URL：{url}（最终：{m.get('finalUrl')}）",
             f"- 行业：{industry or '未指定'}｜视口 1440×900{' + 390×844' if mobile else ''}",
             f"- 截图：`fold.png`、`full.png`{'、`mobile.png`' if mobile else ''}", "",
             "## 1. 评分", "",
             "| 项 | 分 |", "|---|---|",
             f"| 指标分（45%） | **{s['metric_score']}** |",
             "| 视觉分（55%） | **TODO**（1–10 ×10，看截图后填写） |",
             f"| 总分 | **TODO** = 0.45×{s['metric_score']} + 0.55×视觉分 |", "",
             f"子分：色彩 {s['m_color']}｜字号体系 {s['m_type']}｜一致性 {s['m_consist']}｜层级 {s['m_hier']}｜可访问性 {s['m_access']}", "",
             "## 2. 指标 vs 基准（Top 20 中位数）", "",
             "| 指标 | 本站 | 目标 | 结论 |", "|---|---|---|---|"]
    lines += _metric_rows(m, s, True)
    lines += ["", f"字体：{', '.join(m['fonts'])}", f"常用字号：{', '.join(m['topFontSizes'])}",
              f"Hero 文本：{m['heroText']!r}｜首屏媒体占比 {m['heroMediaRatio']:.0%}｜max-width {m['maxWidth'] or '-'}px", ""]
    if hero and "error" not in hero:
        pal = "、".join(f"`{c['hex']}` {c['pct']}%" for c in hero["palette"] if c["pct"] >= 3)
        lines += ["### 首屏主图（hero.png → hero-image/）", "",
                  f"- 图片指标分 {hero['metric_score']}/100（仅参考，不计入网站指标分）｜{hero['width']}×{hero['height']}｜配色 {hero['harmony_guess']}",
                  f"- 主色：{pal}", "- 标记：" + ("；".join(hero["flags"]) or "无"),
                  "- TODO：按 references/image-aesthetics.md 评主图（主体清晰、与品牌色是否同源、文字压图可读性、AI 痕迹）", ""]
    elif hero:
        lines += [f"首屏主图分析跳过：{hero['error']}", ""]
    else:
        lines += ["首屏未检测到占比 ≥12% 的主图/背景图。", ""]
    b = load_bench(industry)
    if b:
        rows, peers, rank_in = b
        est = s["metric_score"]
        lines += ["## 3. 对标", "",
                  f"- 基准库 {len(rows)} 家；按指标分估算，在全库约第 {1 + sum(float(r['metric_score']) > est for r in rows)} 名（总分待视觉分确定后重算）。"]
        if peers:
            top = sorted(peers, key=lambda r: -float(r["score"]))[:3]
            lines.append(f"- 同行业 {len(peers)} 家，均分 {statistics.mean(float(r['score']) for r in peers):.1f}；前三："
                         + "；".join(f"{r['company']} {r['score']}（{r['comment']}）" for r in top))
        lines.append("- TODO：从 references/benchmarks.md 选 1–2 个最接近的对标站点，说明差距。")
        lines.append("")
    lines += ["## 4. 问题清单", "", "### P0（影响可读性/可用性）", "- TODO", "",
              "### P1（明显降低质感）", "- TODO", "", "### P2（打磨）", "- TODO", "",
              "## 5. 美化建议", "", "### Token 方案", "```css", ":root {", "  /* TODO：颜色 ≤12、字号 ≤7 档、圆角一族、阴影 ≤2 */", "}", "```", "",
              "### Before → After", "", "| 区域 | 现状 | 建议 |", "|---|---|---|", "| Hero | TODO | TODO |", "", "## 6. 一句话结论", "", "TODO", ""]
    return "\n".join(lines)


def _report_en(url, m, s, industry, mobile, hero=None):
    lines = [f"# Website design review: {m.get('title') or url}", "",
             f"- URL: {url} (final: {m.get('finalUrl')})",
             f"- Industry: {industry or 'not specified'} | viewport 1440×900{' + 390×844' if mobile else ''}",
             f"- Screenshots: `fold.png`, `full.png`{', `mobile.png`' if mobile else ''}", "",
             "## 1. Score", "",
             "| Item | Score |", "|---|---|",
             f"| Metric score (45%) | **{s['metric_score']}** |",
             "| Visual score (55%) | **TODO** (1–10 ×10, fill in after viewing screenshots) |",
             f"| Total | **TODO** = 0.45×{s['metric_score']} + 0.55×visual |", "",
             f"Sub-scores: color {s['m_color']} | type scale {s['m_type']} | consistency {s['m_consist']} | hierarchy {s['m_hier']} | accessibility {s['m_access']}", "",
             "## 2. Metrics vs benchmark (top-20 median)", "",
             "| Metric | This site | Target | Verdict |", "|---|---|---|---|"]
    lines += _metric_rows(m, s, False)
    lines += ["", f"Fonts: {', '.join(m['fonts'])}", f"Common font sizes: {', '.join(m['topFontSizes'])}",
              f"Hero text: {m['heroText']!r} | first-screen media share {m['heroMediaRatio']:.0%} | max-width {m['maxWidth'] or '-'}px", ""]
    if hero and "error" not in hero:
        pal = ", ".join(f"`{c['hex']}` {c['pct']}%" for c in hero["palette"] if c["pct"] >= 3)
        lines += ["### Hero image (hero.png → hero-image/)", "",
                  f"- Image metric score {hero['metric_score']}/100 (reference only, not part of the site metric score) | {hero['width']}×{hero['height']} | harmony {hero['harmony_guess']}",
                  f"- Main colors: {pal}", "- Flags: " + ("; ".join(hero["flags"]) or "none"),
                  "- TODO: rate the hero per references/image-aesthetics.md (clear subject, shares brand colors?, text-on-image legibility, AI artifacts)", ""]
    elif hero:
        lines += [f"Hero image analysis skipped: {hero['error']}", ""]
    else:
        lines += ["No hero/background image covering ≥12% of the first viewport detected.", ""]
    b = load_bench(industry)
    if b:
        rows, peers, rank_in = b
        est = s["metric_score"]
        lines += ["## 3. Benchmark", "",
                  f"- Benchmark set: {len(rows)} companies; by metric score this site ranks about #{1 + sum(float(r['metric_score']) > est for r in rows)} (recompute once the visual score is in)."]
        if peers:
            top = sorted(peers, key=lambda r: -float(r["score"]))[:3]
            lines.append(f"- {len(peers)} peers in the same industry, mean score {statistics.mean(float(r['score']) for r in peers):.1f}; top 3: "
                         + "; ".join(f"{r['company']} {r['score']} ({r.get('comment_en') or r['comment']})" for r in top))
        lines.append("- TODO: pick 1–2 closest benchmark sites from references/benchmarks.md and describe the gap.")
        lines.append("")
    lines += ["## 4. Issues", "", "### P0 (hurts readability/usability)", "- TODO", "",
              "### P1 (clearly lowers polish)", "- TODO", "", "### P2 (polish)", "- TODO", "",
              "## 5. Improvements", "", "### Token plan", "```css", ":root {", "  /* TODO: colors ≤12, font sizes ≤7, one radius family, shadows ≤2 */", "}", "```", "",
              "### Before → After", "", "| Area | Now | Proposal |", "|---|---|---|", "| Hero | TODO | TODO |", "", "## 6. One-line verdict", "", "TODO", ""]
    return "\n".join(lines)


async def capture(url, out, mobile, full_max):
    async with async_playwright() as p:
        browser = await p.chromium.launch(args=["--disable-blink-features=AutomationControlled"])
        try:
            ctx = await browser.new_context(viewport={"width": W, "height": H}, locale="en-US", user_agent=UA)
            page = await ctx.new_page()
            resp = await settle(page, url)
            consent = await dismiss_consent(page)
            for y in range(0, 6000, 900):  # trigger lazy loading
                await page.evaluate(f"window.scrollTo(0,{y})"); await page.wait_for_timeout(250)
            await page.evaluate("window.scrollTo(0,0)"); await page.wait_for_timeout(1200)
            for attempt in range(3):
                try:
                    m = await page.evaluate(JS, [W, H]); break
                except Exception:
                    if attempt == 2:
                        raise
                    await page.wait_for_timeout(3000)
            m["status"] = resp.status if resp else None
            m["consentClicked"] = consent
            await page.screenshot(path=os.path.join(out, "fold.png"))
            try:
                box = await page.evaluate(HERO_JS, [W, H])
                if box:
                    await page.screenshot(path=os.path.join(out, "hero.png"), clip=box)
                    m["heroImageBox"] = box
            except Exception as e:
                m["heroImageError"] = str(e)[:200]
            h = min(m["pageHeight"], full_max)
            await page.set_viewport_size({"width": W, "height": h}); await page.wait_for_timeout(800)
            await page.screenshot(path=os.path.join(out, "full.png"), clip={"x": 0, "y": 0, "width": W, "height": h})
            await ctx.close()
            if mobile:
                mctx = await browser.new_context(viewport={"width": 390, "height": 844}, device_scale_factor=2,
                                                 is_mobile=True, has_touch=True, locale="en-US",
                                                 user_agent=UA.replace("Macintosh; Intel Mac OS X 10_15_7", "iPhone; CPU iPhone OS 17_0 like Mac OS X") + " Mobile")
                mp = await mctx.new_page()
                await settle(mp, url); await dismiss_consent(mp)
                await mp.screenshot(path=os.path.join(out, "mobile.png"))
                await mctx.close()
            return m
        finally:
            await browser.close()


def main():
    global LANG
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("url")
    ap.add_argument("--out")
    ap.add_argument("--industry", help="e.g. Pharma, Finance, Internet, Software/Cloud, Semiconductors, Hardware/Devices")
    ap.add_argument("--mobile", action="store_true", help="also capture 390x844")
    ap.add_argument("--full-max", type=int, default=8000)
    ap.add_argument("--lang", choices=["en", "zh"], default=None, help=f"report language (default {LANG})")
    a = ap.parse_args()
    if a.lang:
        LANG = a.lang
    url = a.url if "://" in a.url else "https://" + a.url
    out = a.out or "review-" + urlparse(url).netloc.replace(":", "_")
    os.makedirs(out, exist_ok=True)
    m = asyncio.run(capture(url, out, a.mobile, a.full_max))
    s = score_metrics(m)
    if m.get("status") and m["status"] >= 400 or any(k in (m.get("title") or "") for k in ("Access Denied", "Just a moment")):
        print(f"WARNING: looks blocked (status={m.get('status')}, title={m.get('title')!r}); metrics may be meaningless", file=sys.stderr)
    hero = review_hero(out)
    data = {"url": url, "industry": a.industry, **m, **s, "hero_image": hero, "visual_score": None, "score": None}
    json.dump(data, open(os.path.join(out, "metrics.json"), "w"), ensure_ascii=False, indent=1)
    open(os.path.join(out, "report.md"), "w", encoding="utf-8").write(build_report(url, m, s, a.industry, a.mobile, hero))
    print(json.dumps({k: data[k] for k in ("title", "status", "colorsTotal", "fontCount", "fontSizesSig", "radii", "shadows",
                                            "gradients", "heroSize", "bodySize", "contrast", "m_color", "m_type", "m_consist",
                                            "m_hier", "m_access", "metric_score")}, ensure_ascii=False))
    print(f"wrote {out}/metrics.json, report.md, fold.png, full.png" + (", mobile.png" if a.mobile else "")
          + (", hero.png, hero-image/" if hero and "error" not in hero else ""))


if __name__ == "__main__":
    main()
