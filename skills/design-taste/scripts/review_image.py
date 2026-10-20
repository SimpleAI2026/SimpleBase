#!/usr/bin/env python3
"""Local aesthetic metrics for any image (photo, poster, product shot, UI screenshot, figure).

Usage:
  python3 review_image.py <image> [--out DIR] [--type photo|product|poster|social|ui|illustration|figure|ai]
  python3 review_image.py <image> --type game-screenshot|key-art|capsule|icon|game-ui|concept|sprite [--genre G]
  python3 review_image.py --list-genres

Outputs (in --out, default ./image-review-<name>):
  metrics.json       all numbers below + metric sub-scores
  palette.png        swatch strip of the dominant colors with %
  thirds-overlay.png image + rule-of-thirds (white) / golden-ratio (amber) lines,
                     edge-energy heat and visual-weight centroid (red dot)
  report.md          report skeleton (--lang en|zh): metric score filled, visual score left for the agent
  game types also write:
  grayscale.png      grayscale | 3-value posterize (dark/mid/light) | Otsu figure-ground mask
  thumbnail.png      store-size thumbnails (capsule 184x69 & 120x45 / icon 128 & 64 / 320x180),
                     colour + grayscale, nearest-upscaled on Steam navy #1b2838

Pure Pillow + numpy (uses OpenCV only if already installed). No network access.
Metrics are proxies: they catch blur, clipping, muddy palettes, crowded or off-balance
layouts. They do NOT judge meaning, emotion, or AI artifacts — the agent must look at the image.
"""
import argparse, colorsys, json, math, os, sys

LANG = "en"  # default report language for this copy; override with --lang en|zh


def T(zh, en):
    return zh if LANG == "zh" else en


import numpy as np
from PIL import Image, ImageDraw, ImageFont, ImageOps

try:
    import cv2  # optional
except Exception:
    cv2 = None

GAME_TYPES = ["game-screenshot", "key-art", "capsule", "icon", "game-ui", "concept", "sprite"]
TYPES = ["photo", "product", "poster", "social", "ui", "illustration", "figure", "ai"] + GAME_TYPES
# per-type expectations: (ideal whitespace range, max palette colors, wants thirds placement)
PROFILE = {
    "photo":        ((0.05, 0.60), 8, True),
    "product":      ((0.30, 0.80), 5, False),
    "poster":       ((0.25, 0.75), 5, True),
    "social":       ((0.20, 0.70), 5, True),
    "ui":           ((0.25, 0.85), 6, False),
    "illustration": ((0.10, 0.70), 7, True),
    "figure":       ((0.40, 0.85), 6, False),
    "ai":           ((0.05, 0.70), 7, True),
    "game-screenshot": ((0.00, 0.60), 8, False),
    "key-art":      ((0.05, 0.60), 6, True),
    "capsule":      ((0.00, 0.60), 5, False),
    "icon":         ((0.00, 0.50), 4, False),
    "game-ui":      ((0.15, 0.80), 6, False),
    "concept":      ((0.10, 0.80), 6, True),
    "sprite":       ((0.20, 0.95), 16, False),
}
# thumbnail sizes checked per game type (Steam small capsule auto-sizes 184x69 / 120x45; icons 128 / 64)
THUMBS = {"capsule": [(184, 69), (120, 45)], "icon": [(128, 128), (64, 64)], "key-art": [(184, 69), (320, 180)],
          "game-screenshot": [(320, 180), (184, 104)], "game-ui": [(320, 180)], "concept": [(240, 240)],
          "sprite": [(128, 128)]}
# genre profiles: zh name, rms value-contrast range, max significant colours, max HUD/edge coverage, sat p95 range, hints
GENRES = {
    "rpg-western": ("欧美 RPG", (40, 75), 7, 0.25, (0.35, 0.80), ["低饱和写实材质+单一强调色（魔法/危险）", "HUD 角落分组、技能栏底部居中", "检查暗部是否吞掉可交互物"]),
    "jrpg": ("JRPG", (45, 85), 8, 0.30, (0.50, 0.95), ["高明度高饱和、角色描边/赛璐璐", "菜单可风格化（Persona 式）但层级必须清楚", "角色与背景用明度分离"]),
    "arpg": ("ARPG / 刷宝", (45, 85), 7, 0.25, (0.40, 0.90), ["暗背景 + 亮技能/掉落", "怪物剪影与地面明度差 ≥ 2 档", "掉落物稀有度颜色编码必须一致"]),
    "moba": ("MOBA", (45, 85), 8, 0.25, (0.50, 0.95), ["地图中低饱和、英雄与技能高对比", "友/敌颜色编码（蓝/红）不可被皮肤覆盖", "技能 VFX 重要性=视觉强度（大招>普攻）"]),
    "shooter": ("FPS/TPS 射击", (35, 80), 6, 0.15, (0.30, 0.85), ["敌人剪影/轮廓高于环境", "HUD 贴边、准星区保持干净", "环境低频细节，不要满屏粒子"]),
    "battle-royale": ("大逃杀", (40, 80), 7, 0.20, (0.40, 0.90), ["远距离可读：人物与植被明度区分", "安全区/圈颜色唯一", "小地图+队友标记高对比"]),
    "open-world": ("开放世界", (35, 75), 7, 0.15, (0.30, 0.85), ["大气透视分层（近暗远亮/冷）", "地标剪影可从远处识别", "HUD 极简，可隐藏"]),
    "platformer": ("2D 平台/银河城", (45, 90), 7, 0.15, (0.35, 0.95), ["可踩踏平面与背景明度/饱和分离", "前景-中景-背景三层，背景降对比", "危险物统一颜色/形状语言"]),
    "pixel": ("像素风", (45, 95), 16, 0.20, (0.30, 1.00), ["限制调色板（16–32 色）并统一像素尺度", "禁止混合像素尺度/亚像素旋转/模糊缩放", "整数倍放大，最近邻"]),
    "gacha": ("二次元 / 抽卡", (45, 85), 8, 0.35, (0.50, 0.95), ["角色立绘为主角，脸部是焦点", "UI 华丽但层级清晰，货币/活动入口不过载", "稀有度色（金/紫/蓝）统一"]),
    "slg": ("SLG / 4X 策略", (35, 70), 8, 0.40, (0.30, 0.80), ["信息密度高：地图低饱和、单位/阵营高饱和", "图标形状区分资源，别只靠颜色", "缩放层级（LOD）下仍可读"]),
    "card": ("卡牌 / CCG", (45, 85), 8, 0.40, (0.40, 0.95), ["卡框统一、费用/攻防数字最高优先", "插画在缩略尺寸仍可辨", "稀有度/阵营颜色编码一致"]),
    "casual": ("休闲 / 三消", (40, 85), 8, 0.30, (0.55, 1.00), ["高饱和但每种元素色相+形状双编码", "圆润形状语言、按钮大", "棋盘与背景强分离"]),
    "idle": ("放置 / 增量", (40, 80), 7, 0.45, (0.40, 0.90), ["数字层级（主货币最大）", "按钮状态（可买/不可买）一目了然", "长期观看：避免高频闪烁"]),
    "horror": ("恐怖", (25, 70), 5, 0.10, (0.10, 0.60), ["低调光、大面积暗部但保留可读的 key 物", "单一警示色（红/病绿）", "黑位不要死黑到失去轮廓"]),
    "racing": ("竞速", (40, 85), 7, 0.20, (0.40, 0.95), ["速度感：运动模糊只在周边", "赛道边界与路面明度对比", "HUD 底部角落，转速/名次清楚"]),
    "sports": ("体育", (40, 80), 7, 0.20, (0.40, 0.90), ["队伍颜色互斥（明度也不同）", "广播式记分牌、草地/场地中性", "球/焦点高对比"]),
    "roguelike": ("Roguelike / Roguelite", (45, 90), 7, 0.25, (0.40, 0.95), ["高密度战斗中敌弹/伤害区高亮", "房间背景降对比", "遗物/祝福图标形状可辨"]),
    "mmo": ("MMO", (40, 75), 8, 0.40, (0.40, 0.90), ["HUD 可定制但默认分区", "大量玩家时角色/名字牌不糊", "风格统一以防装备拼贴"]),
    "wuxia": ("武侠 / 仙侠", (35, 80), 6, 0.30, (0.25, 0.80), ["东方色谱（石青、朱砂、赭石、月白）", "留白与云雾分层", "书法字体只用于标题，正文保持可读"]),
    "ink": ("水墨", (40, 95), 4, 0.20, (0.00, 0.50), ["墨分五色：焦浓重淡清，明度分层即结构", "一个朱红点题", "留白≥30%，避免满屏数码笔刷纹理"]),
}
ANALYSIS_MAX = 640

GENRES_EN = {
    "rpg-western": ("Western RPG", ["Low-saturation realistic materials + one strong accent (magic/danger)", "HUD grouped in corners, skill bar bottom-center", "Check that shadows don't swallow interactables"]),
    "jrpg": ("JRPG", ["High value, high saturation, character outlines/cel shading", "Menus may be stylized (Persona-like) but hierarchy must stay clear", "Separate characters from background by value"]),
    "arpg": ("ARPG / looter", ["Dark background + bright skills/loot", "Monster silhouettes >= 2 value steps from ground", "Loot rarity color coding must be consistent"]),
    "moba": ("MOBA", ["Map mid/low saturation, heroes and skills high contrast", "Ally/enemy coding (blue/red) must not be overridden by skins", "Skill VFX intensity = importance (ultimate > basic attack)"]),
    "shooter": ("FPS/TPS shooter", ["Enemy silhouettes/outlines stronger than environment", "HUD hugs edges, crosshair area stays clean", "Low-frequency environment detail, no full-screen particles"]),
    "battle-royale": ("Battle royale", ["Long-range readability: players separated from foliage by value", "Safe-zone/circle color is unique", "Minimap + teammate markers high contrast"]),
    "open-world": ("Open world", ["Aerial perspective layering (near dark, far light/cool)", "Landmark silhouettes readable from afar", "Minimal, hideable HUD"]),
    "platformer": ("2D platformer / metroidvania", ["Walkable surfaces separated from background by value/saturation", "Foreground-midground-background layers, background lower contrast", "Hazards share one color/shape language"]),
    "pixel": ("Pixel art", ["Limited palette (16-32 colors) and one pixel scale", "No mixed pixel scales / sub-pixel rotation / blurry scaling", "Integer upscaling, nearest neighbor"]),
    "gacha": ("Anime / gacha", ["Character art is the star, the face is the focal point", "Ornate UI but clear hierarchy; don't overload currency/event entries", "Consistent rarity colors (gold/purple/blue)"]),
    "slg": ("SLG / 4X strategy", ["High info density: low-saturation map, high-saturation units/factions", "Distinguish resources by icon shape, not color alone", "Readable across zoom levels (LOD)"]),
    "card": ("Card / CCG", ["Unified card frames; cost/attack/defense numbers top priority", "Art still legible at thumbnail size", "Consistent rarity/faction color coding"]),
    "casual": ("Casual / match-3", ["High saturation but each element double-coded by hue + shape", "Rounded shape language, big buttons", "Strong board/background separation"]),
    "idle": ("Idle / incremental", ["Number hierarchy (main currency largest)", "Button states (affordable/not) obvious at a glance", "Long viewing: avoid high-frequency flashing"]),
    "horror": ("Horror", ["Low-key lighting, large dark areas but readable key objects", "Single warning color (red / sickly green)", "Blacks must not crush outlines"]),
    "racing": ("Racing", ["Speed: motion blur only at periphery", "Track edge vs road value contrast", "HUD in bottom corners, RPM/position clear"]),
    "sports": ("Sports", ["Team colors mutually exclusive (different value too)", "Broadcast-style scoreboard, neutral pitch/court", "Ball/focus high contrast"]),
    "roguelike": ("Roguelike / roguelite", ["Highlight enemy bullets/damage zones in dense combat", "Lower room background contrast", "Relic/blessing icons distinguishable by shape"]),
    "mmo": ("MMO", ["Customizable HUD with sensible default zones", "Characters/nameplates stay legible in crowds", "Unified style to avoid gear collage"]),
    "wuxia": ("Wuxia / xianxia", ["Eastern palette (azurite, cinnabar, ochre, moon white)", "Negative space and layered mist", "Calligraphic fonts for titles only; body stays readable"]),
    "ink": ("Ink wash", ["Five ink tones: value layering is the structure", "One vermilion accent", "Negative space >= 30%, avoid uniform digital brush texture"]),
}


def genre_name(g):
    return GENRES[g][0] if LANG == "zh" else GENRES_EN[g][0]


def genre_tips(g):
    return GENRES[g][5] if LANG == "zh" else GENRES_EN[g][1]



def to_gray(a):
    return a[..., 0] * 0.299 + a[..., 1] * 0.587 + a[..., 2] * 0.114


def conv2(img, k):
    if cv2 is not None:
        return cv2.filter2D(img.astype(np.float32), -1, k.astype(np.float32), borderType=cv2.BORDER_REFLECT)
    kh, kw = k.shape
    p = np.pad(img, ((kh // 2, kh // 2), (kw // 2, kw // 2)), mode="reflect")
    out = np.zeros_like(img, dtype=np.float64)
    for i in range(kh):
        for j in range(kw):
            if k[i, j]:
                out += k[i, j] * p[i:i + img.shape[0], j:j + img.shape[1]]
    return out


def aspect_name(w, h):
    r = w / h
    known = {"1:1": 1, "4:5": .8, "3:4": .75, "2:3": 2 / 3, "3:5": .6, "9:16": 9 / 16, "5:4": 1.25, "4:3": 4 / 3,
             "3:2": 1.5, "16:10": 1.6, "16:9": 16 / 9, "1.91:1 (OG/link)": 1.91, "21:9": 21 / 9, "3:1 (banner)": 3}
    name, val = min(known.items(), key=lambda kv: abs(math.log(r / kv[1])))
    return name if abs(math.log(r / val)) < 0.03 else f"{r:.2f}:1"


def kmeans(px, k=8, iters=12, seed=0):
    rng = np.random.default_rng(seed)
    # k-means++ init
    c = [px[rng.integers(len(px))]]
    for _ in range(1, k):
        d = np.min(((px[:, None, :] - np.array(c)[None]) ** 2).sum(-1), axis=1)
        if d.sum() == 0:
            break
        c.append(px[rng.choice(len(px), p=d / d.sum())])
    c = np.array(c, dtype=np.float64)
    for _ in range(iters):
        lab = np.argmin(((px[:, None, :] - c[None]) ** 2).sum(-1), axis=1)
        for i in range(len(c)):
            m = px[lab == i]
            if len(m):
                c[i] = m.mean(0)
    lab = np.argmin(((px[:, None, :] - c[None]) ** 2).sum(-1), axis=1)
    return c, np.bincount(lab, minlength=len(c)) / len(px)


def palette(a, k=8):
    px = a.reshape(-1, 3).astype(np.float64)
    if len(px) > 20000:
        px = px[np.random.default_rng(1).choice(len(px), 20000, replace=False)]
    cents, pct = kmeans(px, k)
    # merge near-duplicates (RGB distance < 24)
    cols = sorted(zip(cents.tolist(), pct.tolist()), key=lambda t: -t[1])
    merged = []
    for c, p in cols:
        for m in merged:
            if np.linalg.norm(np.array(m[0]) - np.array(c)) < 24:
                m[0] = ((np.array(m[0]) * m[1] + np.array(c) * p) / (m[1] + p)).tolist(); m[1] += p
                break
        else:
            merged.append([c, p])
    out = []
    for c, p in sorted(merged, key=lambda t: -t[1]):
        r, g, b = [int(round(v)) for v in c]
        h, s, v = colorsys.rgb_to_hsv(r / 255, g / 255, b / 255)
        out.append({"hex": f"#{r:02x}{g:02x}{b:02x}", "pct": round(p * 100, 1), "hue": round(h * 360),
                    "sat": round(s, 2), "val": round(v, 2), "chromatic": bool(s > 0.2 and 0.15 < v)})
    return out


def harmony(pal):
    hues = [c["hue"] for c in pal if c["chromatic"] and c["pct"] >= 3]
    if not hues:
        return "achromatic / neutral"
    def dist(a, b):
        d = abs(a - b) % 360
        return min(d, 360 - d)
    # cluster hues within 25°
    groups = []
    for h in sorted(hues):
        if groups and dist(groups[-1][-1], h) <= 25:
            groups[-1].append(h)
        else:
            groups.append([h])
    if len(groups) > 1 and dist(groups[0][0], groups[-1][-1]) <= 25:
        groups[0] += groups.pop()
    centers = [g[len(g) // 2] for g in groups]
    if len(centers) == 1:
        return "monochromatic (+neutrals)"
    span = max(dist(a, b) for a in centers for b in centers)
    if len(centers) == 2:
        d = dist(*centers)
        if d >= 150: return "complementary"
        if d <= 60: return "analogous"
        return "contrasting pair"
    if span <= 90: return "analogous"
    if len(centers) == 3:
        ds = sorted(dist(a, b) for i, a in enumerate(centers) for b in centers[i + 1:])
        if all(90 <= d <= 150 for d in ds): return "triadic"
        if ds[-1] >= 150: return "split-complementary"
    return f"multi-hue ({len(centers)} hue families)"


def stats(a):
    g = to_gray(a)
    hsv = np.array(Image.fromarray(a.astype(np.uint8)).convert("HSV"), dtype=np.float64) / 255
    hist = np.histogram(g, bins=16, range=(0, 256))[0] / g.size
    return {
        "brightness_mean": round(float(g.mean()), 1),
        "brightness_p5": round(float(np.percentile(g, 5)), 1),
        "brightness_p95": round(float(np.percentile(g, 95)), 1),
        "contrast_rms": round(float(g.std()), 1),
        "clip_dark_pct": round(float((g < 5).mean() * 100), 2),
        "clip_light_pct": round(float((g > 250).mean() * 100), 2),
        "saturation_mean": round(float(hsv[..., 1].mean()), 3),
        "saturation_p95": round(float(np.percentile(hsv[..., 1], 95)), 3),
        "luma_hist16": [round(float(x), 3) for x in hist],
    }


def colorfulness(a):  # Hasler & Süsstrunk 2003
    r, g, b = a[..., 0], a[..., 1], a[..., 2]
    rg, yb = r - g, 0.5 * (r + g) - b
    return float(math.hypot(rg.std(), yb.std()) + 0.3 * math.hypot(rg.mean(), yb.mean()))


def sharpness_noise(g):
    lap = conv2(g, np.array([[0, 1, 0], [1, -4, 1], [0, 1, 0]], float))
    # Immerkaer fast noise estimation
    m = np.array([[1, -2, 1], [-2, 4, -2], [1, -2, 1]], float)
    n = np.abs(conv2(g, m))[1:-1, 1:-1]
    sigma = math.sqrt(math.pi / 2) * n.sum() / (6 * n.size)
    return float(lap.var()), float(sigma)


def edges(g):
    gx = conv2(g, np.array([[-1, 0, 1], [-2, 0, 2], [-1, 0, 1]], float))
    gy = conv2(g, np.array([[-1, -2, -1], [0, 0, 0], [1, 2, 1]], float))
    return np.hypot(gx, gy), gx, gy


def composition(mag):
    h, w = mag.shape
    e = mag / (mag.sum() + 1e-9)
    ys, xs = np.mgrid[0:h, 0:w]
    cx, cy = float((e * xs).sum() / w), float((e * ys).sum() / h)
    pts = [(x, y) for x in (1 / 3, 2 / 3) for y in (1 / 3, 2 / 3)]
    d_thirds = min(math.hypot(cx - x, cy - y) for x, y in pts)
    d_center = math.hypot(cx - .5, cy - .5)
    grid = [[float(e[int(r * h / 3):int((r + 1) * h / 3), int(c * w / 3):int((c + 1) * w / 3)].sum()) for c in range(3)]
            for r in range(3)]
    band = np.zeros_like(e, bool)
    bw, bh = max(1, int(w * .04)), max(1, int(h * .04))
    for f in (1 / 3, 2 / 3):
        band[:, int(w * f) - bw:int(w * f) + bw] = True
        band[int(h * f) - bh:int(h * f) + bh, :] = True
    thirds_ratio = float(e[band].sum() / band.mean()) if band.mean() else 0
    lr = float(e[:, :w // 2].sum()); tb = float(e[:h // 2].sum())
    flat = np.array(grid).ravel()
    return {
        "centroid": [round(cx, 3), round(cy, 3)],
        "centroid_to_thirds_point": round(d_thirds, 3),
        "centroid_to_center": round(d_center, 3),
        "grid3x3_energy": [[round(v, 3) for v in r] for r in grid],
        "thirds_band_energy_ratio": round(thirds_ratio, 2),
        "balance_left_right": round(lr, 3),
        "balance_top_bottom": round(tb, 3),
        "energy_concentration": round(float(flat.max() / (flat.mean() + 1e-9)), 2),
        "mirror_symmetry": None,
    }


def symmetry(g):
    a = g - g.mean()
    b = a[:, ::-1]
    return float((a * b).sum() / (math.sqrt((a * a).sum() * (b * b).sum()) + 1e-9))


def block_maps(g, mag, gx, gy, bs=16):
    h, w = g.shape
    H, W = h // bs, w // bs
    if H == 0 or W == 0:
        return 0.0, 0.0
    gb = g[:H * bs, :W * bs].reshape(H, bs, W, bs)
    mb = mag[:H * bs, :W * bs].reshape(H, bs, W, bs)
    var = gb.std(axis=(1, 3)); en = mb.mean(axis=(1, 3))
    white = (var < 6) & (en < 12)
    # text proxy: many strong, thin, mostly vertical strokes + high local contrast
    vx = np.abs(gx[:H * bs, :W * bs]).reshape(H, bs, W, bs).mean(axis=(1, 3))
    vy = np.abs(gy[:H * bs, :W * bs]).reshape(H, bs, W, bs).mean(axis=(1, 3))
    strong = (mb > 80).mean(axis=(1, 3))
    text = (var > 35) & (strong > 0.08) & (strong < 0.45) & (vx > 0.8 * vy)
    # require a horizontal neighbor also text-like (text runs in lines)
    tn = text.copy()
    tn[:, 1:-1] = text[:, 1:-1] & (text[:, :-2] | text[:, 2:])
    return float(white.mean()), float(tn.mean())


def otsu(g):
    hist = np.histogram(g, bins=256, range=(0, 256))[0].astype(np.float64)
    p = hist / max(hist.sum(), 1); w = np.cumsum(p); mu = np.cumsum(p * np.arange(256)); mt = mu[-1]
    sb = (mt * w - mu) ** 2 / (w * (1 - w) + 1e-12)
    t = int(np.argmax(sb))
    return t, float(sb[t] / (g.var() + 1e-9))  # threshold, separability (0..1, 1 = perfectly two-tone)


def downscale(rgb, size, fit=True):
    im = rgb.copy()
    if fit:
        return im.resize(size, Image.LANCZOS)
    im.thumbnail(size, Image.LANCZOS); return im


def game_metrics(rgb, a, g, mag, gx, gy, kind):
    gm = {}
    v = g / 255.0
    dark, light = float((v < 1 / 3).mean()), float((v > 2 / 3).mean())
    gm["value_bands"] = {"dark": round(dark, 3), "mid": round(1 - dark - light, 3), "light": round(light, 3)}
    q = np.clip((v * 5).astype(int), 0, 4)
    gm["value_levels_used_5"] = int(sum((q == i).mean() > 0.05 for i in range(5)))
    t, sep = otsu(g)
    fg = g > t
    gm["otsu_threshold"] = t; gm["figure_ground_separability"] = round(sep, 3)
    # figure-ground: luminance gap between the two Otsu classes, and centre-vs-border gap
    m1, m0 = (g[fg].mean() if fg.any() else 0), (g[~fg].mean() if (~fg).any() else 0)
    gm["figure_ground_gap"] = round(float(m1 - m0), 1)
    h, w = g.shape
    cy0, cy1, cx0, cx1 = h // 4, 3 * h // 4, w // 4, 3 * w // 4
    cen = np.zeros_like(g, bool); cen[cy0:cy1, cx0:cx1] = True
    gm["center_border_luma_gap"] = round(float(abs(g[cen].mean() - g[~cen].mean())), 1)
    gm["center_border_edge_ratio"] = round(float(mag[cen].mean() / (mag[~cen].mean() + 1e-9)), 2)
    # silhouette / shape clarity: edges that survive a 4x blur-downscale = big-shape edges vs fine noise
    sm = Image.fromarray(g.astype(np.uint8)).resize((max(8, w // 4), max(8, h // 4)), Image.BOX)
    sm = sm.resize((w, h), Image.BILINEAR)
    mag4, _, _ = edges(np.asarray(sm, dtype=np.float64))
    gm["shape_edge_retention"] = round(min(1.0, float(np.percentile(mag4, 90) / (np.percentile(mag, 90) + 1e-9))), 3)
    # Otsu mask boundary length relative to perimeter = silhouette complexity (lower = cleaner masses)
    bnd = (fg[1:, :] != fg[:-1, :]).sum() + (fg[:, 1:] != fg[:, :-1]).sum()
    gm["mask_boundary_complexity"] = round(float(bnd / (2 * (h + w))), 2)
    # detail density: share of pixels with strong edges
    gm["edge_density"] = round(float((mag > 60).mean()), 3)
    # HUD / UI-like coverage: 16px blocks with axis-aligned strong edges or text-like strokes
    bs = 16; H, W = h // bs, w // bs
    if H and W:
        ax = np.abs(gx[:H * bs, :W * bs]); ay = np.abs(gy[:H * bs, :W * bs]); mm = mag[:H * bs, :W * bs]
        strong = mm > 60
        aligned = strong & ((np.minimum(ax, ay) / (np.maximum(ax, ay) + 1e-9)) < 0.15)
        al = aligned.reshape(H, bs, W, bs).mean(axis=(1, 3)); st_ = strong.reshape(H, bs, W, bs).mean(axis=(1, 3))
        ui = (al > 0.06) & (al > 0.6 * st_)
        border = np.ones((H, W), bool); bh, bw = max(1, int(H * 0.15)), max(1, int(W * 0.15))
        border[bh:H - bh, bw:W - bw] = False
        gm["hud_edge_coverage_est"] = round(float(ui[border].mean()), 3)
        gm["ui_like_coverage_est"] = round(float(ui.mean()), 3)
        gm["hud_center_clutter_est"] = round(float(ui[~border].mean()) if (~border).any() else 0.0, 3)
    else:
        gm["hud_edge_coverage_est"] = gm["ui_like_coverage_est"] = gm["hud_center_clutter_est"] = 0.0
    # colour count: 5-bit quantised bins covering 95% of pixels; exact unique colours (pixel art)
    qa = (a.astype(np.int32) >> 3); key = (qa[..., 0] << 10) | (qa[..., 1] << 5) | qa[..., 2]
    cnt = np.sort(np.bincount(key.ravel()))[::-1]
    gm["color_bins_95pct"] = int(np.searchsorted(np.cumsum(cnt) / cnt.sum(), 0.95) + 1)
    full = np.asarray(rgb, dtype=np.uint8)
    if full.shape[0] * full.shape[1] <= 4_000_000:
        u = np.unique(full.reshape(-1, 3), axis=0); gm["unique_colors_exact"] = int(len(u))
    else:
        gm["unique_colors_exact"] = None
    # pixel-art grid scale: modal run length of identical neighbouring pixels (rows)
    row = full[full.shape[0] // 2]; same = np.all(row[1:] == row[:-1], axis=1)
    runs, c = [], 1
    for s_ in same:
        if s_: c += 1
        else: runs.append(c); c = 1
    runs.append(c)
    rl = np.bincount(np.array(runs))
    gm["pixel_run_mode"] = int(np.argmax(rl[1:]) + 1) if len(rl) > 1 else 1
    # thumbnail readability
    th = []
    base_c = float(g.std()) or 1.0; base_e = float(np.percentile(mag, 90)) or 1.0
    for size in THUMBS.get(kind, [(320, 180)]):
        t_im = downscale(rgb, size, fit=kind in ("capsule", "icon"))
        tg = to_gray(np.asarray(t_im, dtype=np.float64)); tm, _, _ = edges(tg)
        th.append({"size": f"{t_im.size[0]}x{t_im.size[1]}", "contrast_rms": round(float(tg.std()), 1),
                   "contrast_retention": round(float(tg.std()) / base_c, 3),
                   "edge_retention": round(float(np.percentile(tm, 90)) / base_e, 3),
                   "luma_vs_steam_navy": round(abs(float(tg.mean()) - 38.5), 1)})
    gm["thumbnails"] = th
    return gm, fg


def game_sub_scores(gm, kind, genre):
    s = {}
    b = gm["value_bands"]; big = max(b.values())
    s["value_structure"] = round(max(2, min(10, 10 - max(0, big - 0.70) * 25 - max(0, 3 - gm["value_levels_used_5"]) * 1.5)), 1)
    s["figure_ground"] = round(max(1, min(10, gm["figure_ground_gap"] / 9)), 1)
    s["shape_clarity"] = round(max(1, min(10, (gm["shape_edge_retention"] - 0.15) * 20 + 1)), 1)
    hud_max = GENRES[genre][3] if genre else 0.25
    if kind in ("game-screenshot", "game-ui"):
        hc = gm["hud_edge_coverage_est"]
        s["hud_balance"] = round(10 if hc <= hud_max else max(2, 10 - (hc - hud_max) * 30), 1)
        s["center_clean"] = round(max(2, 10 - max(0, gm["hud_center_clutter_est"] - 0.08) * 40), 1)
    maxcol = GENRES[genre][2] if genre else PROFILE[kind][1]
    if kind == "sprite" or genre == "pixel":
        u = gm["unique_colors_exact"] or 999
        s["color_count"] = round(10 if u <= 64 else max(2, 10 - math.log2(u / 64) * 2), 1)
    else:
        s["color_count"] = round(10 if gm["color_bins_95pct"] <= 600 else max(3, 10 - math.log2(gm["color_bins_95pct"] / 600) * 2), 1)
    th = gm["thumbnails"]
    if th:
        worst = min(th, key=lambda x: x["contrast_retention"] + x["edge_retention"])
        s["thumbnail_readability"] = round(max(1, 0.5 * min(10, worst["contrast_rms"] / 5) + 0.5 * min(10, worst["contrast_retention"] * 10)), 1)
    return s


def genre_hints(m, genre):
    if not genre:
        return []
    _, (clo, chi), maxcol, hudmax, (slo, shi), _ = GENRES[genre]
    name = genre_name(genre)
    t, gm, out = m["tone"], m["game"], []
    c = t["contrast_rms"]
    if c < clo: out.append(T(f"P1 [{name}] 明度对比 {c} 低于该类型常见区间 {clo}–{chi}：主体/可交互物可能读不出来，先做灰度测试", f"P1 [{name}] value contrast {c} below the usual {clo}–{chi} for this genre: subject/interactables may not read; do a grayscale test first"))
    elif c > chi: out.append(T(f"P2 [{name}] 明度对比 {c} 高于常见区间 {clo}–{chi}：可能刺眼或噪，背景降对比", f"P2 [{name}] value contrast {c} above the usual {clo}–{chi}: may be harsh or noisy; lower background contrast"))
    if m["palette_significant_colors"] > maxcol: out.append(T(f"P1 [{name}] 显著色 {m['palette_significant_colors']} > {maxcol}：缺主色，建议主色+1 强调色+功能色", f"P1 [{name}] significant colors {m['palette_significant_colors']} > {maxcol}: no dominant color; use one main + one accent + functional colors"))
    sp = t["saturation_p95"]
    if sp < slo: out.append(T(f"P2 [{name}] 饱和 p95 {sp} 低于常见 {slo}–{shi}：可能显灰/不吸睛（除非刻意）", f"P2 [{name}] saturation p95 {sp} below the usual {slo}–{shi}: may look dull (unless intentional)"))
    elif sp > shi: out.append(T(f"P1 [{name}] 饱和 p95 {sp} 高于常见 {slo}–{shi}：饱和过载，焦点失效", f"P1 [{name}] saturation p95 {sp} above the usual {slo}–{shi}: oversaturated, focus is lost"))
    if m["type"] in ("game-screenshot", "game-ui") and gm["hud_edge_coverage_est"] > hudmax:
        out.append(T(f"P1 [{name}] 边缘 HUD 覆盖估计 {gm['hud_edge_coverage_est']:.0%} > {hudmax:.0%}：信息过载或面板过重", f"P1 [{name}] edge HUD coverage est. {gm['hud_edge_coverage_est']:.0%} > {hudmax:.0%}: information overload or heavy panels"))
    if genre in ("pixel",) and gm["unique_colors_exact"] and gm["unique_colors_exact"] > 256:
        out.append(T(f"P1 [{name}] 精确色数 {gm['unique_colors_exact']} > 256：疑似缩放插值/抗锯齿/AI 生成，非干净像素", f"P1 [{name}] exact colors {gm['unique_colors_exact']} > 256: likely interpolated scaling/anti-aliasing/AI, not clean pixels"))
    if genre == "ink" and m["palette_chromatic_colors"] > 2:
        out.append(T(f"P1 [{name}] 彩色族 {m['palette_chromatic_colors']} 个：水墨通常墨色+1 点朱红", f"P1 [{name}] {m['palette_chromatic_colors']} chromatic color families: ink wash is usually ink tones + one vermilion accent"))
    return out


def sub_scores(m, kind):
    (wlo, whi), maxcol, wants_thirds = PROFILE[kind]
    s = {}
    lv = m["sharpness_laplacian_var"]
    s["sharpness"] = 10 if lv >= 300 else max(1, round(lv / 30, 1)) if kind not in ("illustration", "poster", "ui", "figure") else (10 if lv >= 80 else max(3, round(lv / 8, 1)))
    st = m["tone"]
    clip = st["clip_dark_pct"] + st["clip_light_pct"]
    if kind in ("ui", "figure", "product", "poster", "illustration", "social") or kind in GAME_TYPES:
        # flat white/black fields are intentional in graphics: only punish a crushed mid-tone range
        s["exposure"] = round(min(10, max(1, (st["brightness_p95"] - st["brightness_p5"]) / 12)), 1)
    else:
        s["exposure"] = round(max(1, 10 - max(0, clip - 3) * 0.4 - max(0, abs(st["brightness_mean"] - 128) - 70) * 0.08), 1)
    c = st["contrast_rms"]
    s["contrast"] = round(10 if 40 <= c <= 85 else max(1, 10 - (40 - c) * 0.25 if c < 40 else 10 - (c - 85) * 0.15), 1)
    nz = m["noise_sigma"]
    s["noise"] = round(10 if nz < 2.5 else max(1, 10 - (nz - 2.5) * 1.2), 1)
    n = m["palette_significant_colors"]
    s["palette_discipline"] = round(10 if n <= maxcol else max(2, 10 - (n - maxcol) * 1.5), 1)
    sp = st["saturation_p95"]
    s["saturation_restraint"] = round(10 if sp < 0.85 or kind == "illustration" else max(3, 10 - (sp - 0.85) * 40), 1)
    ws = m["whitespace_ratio"]
    s["whitespace"] = round(10 if wlo <= ws <= whi else max(2, 10 - (wlo - ws) * 20 if ws < wlo else 10 - (ws - whi) * 20), 1)
    comp = m["composition"]
    if wants_thirds:
        d = min(comp["centroid_to_thirds_point"], comp["centroid_to_center"] + 0.05)
        s["composition"] = round(max(3, 10 - d * 30), 1)
    else:
        lr, tb = comp["balance_left_right"], comp["balance_top_bottom"]
        s["composition"] = round(max(3, 10 - (abs(lr - .5) + abs(tb - .5)) * 15), 1)
    weights = {"sharpness": 1.5, "exposure": 1.5, "contrast": 1, "noise": 1, "palette_discipline": 1.5,
               "saturation_restraint": 1, "whitespace": 1, "composition": 1.5}
    total = sum(s[k] * w for k, w in weights.items()) / sum(weights.values())
    return s, round(total * 10, 1)


def draw_palette(pal, path):
    W, H = 900, 140
    im = Image.new("RGB", (W, H), "white"); d = ImageDraw.Draw(im)
    x = 0
    try:
        font = ImageFont.load_default(size=13)
    except TypeError:
        font = ImageFont.load_default()
    for i, c in enumerate(pal):
        w = W - x if i == len(pal) - 1 else max(1, int(round(W * c["pct"] / 100)))
        d.rectangle([x, 0, x + w, 100], fill=c["hex"])
        if w > 120:
            d.text((x + 6, 106), f'{c["hex"]} {c["pct"]}%', fill="black", font=font)
        x += w
    im.save(path)


def draw_overlay(img, mag, comp, path):
    im = img.convert("RGB").copy()
    w, h = im.size
    heat = mag / (np.percentile(mag, 99) + 1e-9)
    heat = np.clip(heat, 0, 1)
    hm = Image.fromarray((heat * 255).astype(np.uint8)).resize((w, h))
    red = Image.new("RGB", (w, h), (255, 40, 40))
    im = Image.composite(red, im, hm.point(lambda v: int(v * 0.45)))
    d = ImageDraw.Draw(im)
    lw = max(1, w // 400)
    for f in (1 / 3, 2 / 3):
        d.line([(w * f, 0), (w * f, h)], fill="white", width=lw)
        d.line([(0, h * f), (w, h * f)], fill="white", width=lw)
    for f in (0.382, 0.618):
        d.line([(w * f, 0), (w * f, h)], fill=(255, 176, 0), width=lw)
        d.line([(0, h * f), (w, h * f)], fill=(255, 176, 0), width=lw)
    cx, cy = comp["centroid"][0] * w, comp["centroid"][1] * h
    r = max(6, w // 80)
    d.ellipse([cx - r, cy - r, cx + r, cy + r], fill=(230, 0, 0), outline="white", width=lw + 1)
    im.save(path)


def analyze(path, kind="photo", genre=None):
    img = ImageOps.exif_transpose(Image.open(path))
    W0, H0 = img.size
    has_alpha = img.mode in ("RGBA", "LA") or "transparency" in img.info
    rgb = img.convert("RGBA")
    if has_alpha:
        bg = Image.new("RGBA", rgb.size, (255, 255, 255, 255)); bg.alpha_composite(rgb); rgb = bg
    rgb = rgb.convert("RGB")
    small = rgb.copy(); small.thumbnail((ANALYSIS_MAX, ANALYSIS_MAX), Image.LANCZOS)
    a = np.asarray(small, dtype=np.float64)
    g = to_gray(a)
    pal = palette(a)
    lapvar, _ = sharpness_noise(g)
    full = np.asarray(rgb, dtype=np.float64)
    ch, cw = min(1024, full.shape[0]), min(1024, full.shape[1])
    y0, x0 = (full.shape[0] - ch) // 2, (full.shape[1] - cw) // 2
    _, noise = sharpness_noise(to_gray(full[y0:y0 + ch, x0:x0 + cw]))
    mag, gx, gy = edges(g)
    comp = composition(mag)
    comp["mirror_symmetry"] = round(symmetry(g), 3)
    ws, txt = block_maps(g, mag, gx, gy)
    m = {
        "file": os.path.abspath(path), "type": kind,
        "width": W0, "height": H0, "megapixels": round(W0 * H0 / 1e6, 2), "aspect": aspect_name(W0, H0),
        "has_alpha": has_alpha, "analysis_size": list(small.size), "opencv": cv2 is not None,
        "palette": pal,
        "palette_significant_colors": sum(1 for c in pal if c["pct"] >= 3),
        "palette_chromatic_colors": sum(1 for c in pal if c["chromatic"] and c["pct"] >= 3),
        "harmony_guess": harmony(pal),
        "tone": stats(a),
        "colorfulness": round(colorfulness(a), 1),
        "sharpness_laplacian_var": round(lapvar, 1),
        "noise_sigma": round(noise, 2),
        "composition": comp,
        "whitespace_ratio": round(ws, 3),
        "text_region_ratio_est": round(txt, 3),
    }
    flags = []
    if min(W0, H0) < 600 and kind not in ("capsule", "icon", "sprite", "game-ui"): flags.append(T("P1 低分辨率：短边 < 600px，大屏/印刷会糊", "P1 low resolution: short side < 600px, will blur on large screens/print"))
    if kind not in ("ui", "figure", "illustration", "poster") and lapvar < 60: flags.append(T("P0 疑似失焦/模糊（Laplacian 方差低）", "P0 likely out of focus/blurry (low Laplacian variance)"))
    if noise > 8 and kind not in ("capsule", "icon"): flags.append(T("P1 噪点偏高", "P1 high noise"))
    t = m["tone"]
    if kind in ("photo", "ai") and t["clip_light_pct"] > 5: flags.append(T("P1 高光溢出 >5%", "P1 clipped highlights > 5%"))
    if kind in ("photo", "ai") and t["clip_dark_pct"] > 8: flags.append(T("P2 暗部死黑 >8%", "P2 crushed shadows > 8%"))
    if t["saturation_p95"] > 0.92 and kind not in ("illustration",) and not genre: flags.append(T("P1 饱和度过高（AI/滤镜感风险）", "P1 saturation too high (AI/filter look risk)"))
    if m["palette_significant_colors"] > PROFILE[kind][1] + 2: flags.append(T("P1 色彩过多，缺主色", "P1 too many colors, no dominant color"))
    if comp["mirror_symmetry"] > 0.85 and kind in ("ai", "photo", "poster"): flags.append(T("P2 近乎镜像对称（AI 对称套路风险）", "P2 near-mirror symmetry (AI symmetry cliché risk)"))
    if kind in ("poster", "social", "product", "ui", "figure") and ws < PROFILE[kind][0][0]: flags.append(T("P1 留白不足，画面拥挤", "P1 not enough negative space, crowded"))
    m["genre"] = genre
    fg = None
    if kind in GAME_TYPES:
        gm, fg = game_metrics(rgb, a, g, mag, gx, gy, kind)
        m["game"] = gm
        gs = game_sub_scores(gm, kind, genre)
        b = gm["value_bands"]
        if max(b.values()) > 0.85: flags.append(T("P1 明度结构单一：一个明度带占比 >85%，灰度下主体难分离", "P1 flat value structure: one value band > 85%, subject hard to separate in grayscale"))
        if gm["figure_ground_gap"] < 60: flags.append(T("P1 图底明度差小（Otsu 两类均值差 < 60）：剪影/主体可读性风险", "P1 small figure-ground value gap (Otsu class mean gap < 60): silhouette/subject readability risk"))
        for tb in gm["thumbnails"]:
            if kind in ("capsule", "icon", "key-art") and (tb["contrast_rms"] < 35 or tb["contrast_retention"] < 0.75):
                flags.append(T(f"P0 缩略图 {tb['size']} 可读性差（对比 {tb['contrast_rms']}，保留 {tb['contrast_retention']}）：标题/主体在商店列表会糊", f"P0 thumbnail {tb['size']} poorly readable (contrast {tb['contrast_rms']}, retention {tb['contrast_retention']}): title/subject will blur in store listings"))
        if kind == "capsule" and m["text_region_ratio_est"] < 0.01:
            flags.append(T("P2 胶囊图未检测到明显文字区：Steam 要求可读的游戏名/Logo（人工确认）", "P2 no clear text region in capsule: Steam requires a readable game name/logo (check manually)"))
        if kind in ("game-screenshot",) and gm["hud_center_clutter_est"] > 0.15:
            flags.append(T("P2 画面中心 UI/直线元素偏多：可能遮挡玩法焦点", "P2 lots of UI/straight-line elements in the center: may block gameplay focus"))
        if kind == "sprite" and gm["unique_colors_exact"] and gm["unique_colors_exact"] > 256:
            flags.append(T("P1 精灵图色数 > 256：疑似插值缩放或 AI 噪色，像素不干净", "P1 sprite has > 256 colors: likely interpolated scaling or AI noise, pixels not clean"))
        flags.extend(genre_hints(m, genre))
    m["flags"] = flags
    m["sub_scores"], m["metric_score"] = sub_scores(m, kind)
    if kind in GAME_TYPES:
        base = {k: m["sub_scores"][k] for k in ("sharpness", "exposure", "noise", "palette_discipline", "saturation_restraint")}
        allsub = {**base, **gs}
        w = {"value_structure": 1.5, "figure_ground": 1.5, "shape_clarity": 1.2, "thumbnail_readability": 2 if kind in ("capsule", "icon", "key-art") else 0.8,
             "hud_balance": 1, "center_clean": 0.8, "color_count": 0.8, "sharpness": 1, "exposure": 0.8,
             "noise": 0.2 if kind in ("capsule", "icon") else 0.6,
             "palette_discipline": 1, "saturation_restraint": 0.7}
        m["sub_scores"] = allsub
        m["metric_score"] = round(sum(allsub[k] * w[k] for k in allsub) / sum(w[k] for k in allsub) * 10, 1)
    return m, rgb, mag, fg


REPORT_ZH = """# 图片审美评审：{name}

> 类型：{kind}｜尺寸 {w}×{h}（{aspect}）｜文件 `{file}`

## 1. 评分
| 部分 | 分数 | 说明 |
|---|---|---|
| 指标分（脚本，40%） | {ms}/100 | 清晰度、曝光、对比、噪点、色彩纪律、饱和克制、留白、构图代理 |
| 视觉分（Agent，60%） | __/100 | 按 references/image-aesthetics.md 各维度 1–10 打分 × 权重 |
| **总分** | __/100 | 0.4 × 指标分 + 0.6 × 视觉分 |

### 指标子分（0–10）
{subs}

## 2. 关键指标
- 主色（≥3%）：{pal}
- 显著色数 {ncol}（彩色 {nchrom}）｜配色关系猜测：**{harm}**
- 亮度均值 {bm}｜RMS 对比 {cr}｜高光溢出 {cl}%｜死黑 {cd}%｜饱和 p95 {sp}｜Colorfulness {cf}
- 清晰度（Laplacian 方差）{lv}｜噪点 σ≈{nz}
- 视觉重心 {cen}（到最近三分交点 {dt}，到中心 {dc}）｜镜像对称 {sym}｜三分带能量比 {tr}
- 留白比例 {ws}｜文字区域估计 {tx}

自动标记：
{flags}

附件：`palette.png`（色卡）、`thirds-overlay.png`（白=三分线，琥珀=黄金分割，红点=视觉重心，红色热区=边缘能量）

## 3. 视觉评审（Agent 填写，逐维度 1–10）
| 维度 | 权重 | 分 | 证据 / 问题 |
|---|---|---|---|
| 构图与主体 | | | |
| 光线 | | | |
| 色彩和谐 | | | |
| 图上文字/排版 | | | |
| 清晰与技术质量 | | | |
| 风格一致性 | | | |
| 情绪与叙事 | | | |
| AI 痕迹（如适用） | | | |

## 4. 问题清单
### P0 — 
### P1 — 
### P2 — 

## 5. 改进 / 重制建议（可执行：裁切比例、调色参数、配色 HEX、版式、生成提示词）

## 6. 一句话结论
"""


GAME_REPORT_ZH = """
## 2b. 游戏美术指标（{genre}）
- 明度三档（暗/中/亮）：{vb}｜5 档中使用 {vl} 档｜Otsu 图底可分离度 {sep}｜图底明度差 {fgg}
- 中心-边缘明度差 {cbl}｜中心/边缘细节比 {cbe}｜大形边缘保留 {ser}｜剪影边界复杂度 {mbc}｜强边缘密度 {ed}
- HUD 边缘覆盖估计 {hud}｜全画面 UI 样式覆盖 {uic}｜中心 UI 杂乱 {hcc}
- 色数：5-bit 色箱覆盖 95% = {cb}｜精确色数 {uc}｜像素游程众数 {prm}
- 缩略图：{thumbs}

附件：`grayscale.png`（灰度｜三值明度｜Otsu 图底）、`thumbnail.png`（商店尺寸缩略，彩色+灰度，Steam 深蓝底）

### 类型提示
{hints}
"""


REPORT_EN = """# Image aesthetics review: {name}

> Type: {kind} | Size {w}×{h} ({aspect}) | File `{file}`

## 1. Score
| Part | Score | Notes |
|---|---|---|
| Metric score (script, 40%) | {ms}/100 | sharpness, exposure, contrast, noise, palette discipline, saturation restraint, whitespace, composition proxies |
| Visual score (agent, 60%) | __/100 | rate each dimension 1–10 × weight per references/image-aesthetics.md |
| **Total** | __/100 | 0.4 × metric + 0.6 × visual |

### Metric sub-scores (0–10)
{subs}

## 2. Key metrics
- Main colors (≥3%): {pal}
- Significant colors {ncol} (chromatic {nchrom}) | harmony guess: **{harm}**
- Mean brightness {bm} | RMS contrast {cr} | clipped highlights {cl}% | crushed shadows {cd}% | saturation p95 {sp} | colorfulness {cf}
- Sharpness (Laplacian variance) {lv} | noise σ≈{nz}
- Visual centroid {cen} (to nearest thirds point {dt}, to center {dc}) | mirror symmetry {sym} | thirds-band energy ratio {tr}
- Whitespace ratio {ws} | text region est. {tx}

Automatic flags:
{flags}

Attachments: `palette.png` (swatches), `thirds-overlay.png` (white = thirds, amber = golden ratio, red dot = visual centroid, red heat = edge energy)

## 3. Visual review (agent fills in, 1–10 per dimension)
| Dimension | Weight | Score | Evidence / issue |
|---|---|---|---|
| Composition & subject | | | |
| Light | | | |
| Color harmony | | | |
| Text / typography on image | | | |
| Clarity & technical quality | | | |
| Style consistency | | | |
| Mood & story | | | |
| AI artifacts (if applicable) | | | |

## 4. Issues
### P0 — 
### P1 — 
### P2 — 

## 5. Improvement / restyle plan (actionable: crop ratio, grading values, palette HEX, layout, generation prompt)

## 6. One-line verdict
"""


GAME_REPORT_EN = """
## 2b. Game art metrics ({genre})
- Three value bands (dark/mid/light): {vb} | {vl} of 5 value levels used | Otsu figure-ground separability {sep} | figure-ground value gap {fgg}
- Center-border value gap {cbl} | center/border detail ratio {cbe} | big-shape edge retention {ser} | silhouette boundary complexity {mbc} | strong edge density {ed}
- HUD edge coverage est. {hud} | UI-like coverage {uic} | center UI clutter {hcc}
- Colors: 5-bit bins covering 95% = {cb} | exact colors {uc} | pixel run mode {prm}
- Thumbnails: {thumbs}

Attachments: `grayscale.png` (grayscale | 3-value | Otsu figure-ground), `thumbnail.png` (store-size thumbnails, color + grayscale, on Steam navy)

### Genre tips
{hints}
"""


def build_report(m):
    t, c = m["tone"], m["composition"]
    subs = "\n".join(f"- {k}: {v}" for k, v in m["sub_scores"].items())
    pal = T("、", ", ").join(f'`{p["hex"]}` {p["pct"]}%' for p in m["palette"] if p["pct"] >= 3)
    return T(REPORT_ZH, REPORT_EN).format(name=os.path.basename(m["file"]), kind=m["type"], w=m["width"], h=m["height"],
                         aspect=m["aspect"], file=m["file"], ms=m["metric_score"], subs=subs, pal=pal,
                         ncol=m["palette_significant_colors"], nchrom=m["palette_chromatic_colors"],
                         harm=m["harmony_guess"], bm=t["brightness_mean"], cr=t["contrast_rms"],
                         cl=t["clip_light_pct"], cd=t["clip_dark_pct"], sp=t["saturation_p95"],
                         cf=m["colorfulness"], lv=m["sharpness_laplacian_var"], nz=m["noise_sigma"],
                         cen=c["centroid"], dt=c["centroid_to_thirds_point"], dc=c["centroid_to_center"],
                         sym=c["mirror_symmetry"], tr=c["thirds_band_energy_ratio"], ws=m["whitespace_ratio"],
                         tx=m["text_region_ratio_est"],
                         flags="\n".join(f"- {f}" for f in m["flags"]) or T("- （无）", "- (none)")) + game_report(m)


def game_report(m):
    if "game" not in m:
        return ""
    gm = m["game"]; g = m.get("genre")
    hints = genre_tips(g) if g else [T("未指定 --genre：按 references/game-aesthetics.md 通用量表评审", "No --genre given: review with the generic rubric in references/game-aesthetics.md")]
    return T(GAME_REPORT_ZH, GAME_REPORT_EN).format(
        genre=(genre_name(g) if g else T("通用", "generic")), vb=gm["value_bands"], vl=gm["value_levels_used_5"],
        sep=gm["figure_ground_separability"], fgg=gm["figure_ground_gap"], cbl=gm["center_border_luma_gap"],
        cbe=gm["center_border_edge_ratio"], ser=gm["shape_edge_retention"], mbc=gm["mask_boundary_complexity"],
        ed=gm["edge_density"], hud=f'{gm["hud_edge_coverage_est"]:.1%}', uic=f'{gm["ui_like_coverage_est"]:.1%}',
        hcc=f'{gm["hud_center_clutter_est"]:.1%}', cb=gm["color_bins_95pct"], uc=gm["unique_colors_exact"],
        prm=gm["pixel_run_mode"],
        thumbs=T("；", "; ").join(T(f'{t["size"]} 对比 {t["contrast_rms"]}（保留 {t["contrast_retention"]}）边缘保留 {t["edge_retention"]}', f'{t["size"]} contrast {t["contrast_rms"]} (retention {t["contrast_retention"]}) edge retention {t["edge_retention"]}') for t in gm["thumbnails"]),
        hints="\n".join(f"- {h}" for h in hints))


def draw_gray(rgb, fg, path):
    im = rgb.copy(); im.thumbnail((800, 800))
    g = im.convert("L"); w, h = g.size
    v = np.asarray(g, dtype=np.float64)
    post = np.where(v < 85, 40, np.where(v > 170, 220, 128)).astype(np.uint8)
    m = Image.fromarray(fg.astype(np.uint8) * 255).resize((w, h), Image.NEAREST) if fg is not None else g
    out = Image.new("L", (w * 3 + 20, h), 255)
    out.paste(g, (0, 0)); out.paste(Image.fromarray(post), (w + 10, 0)); out.paste(m, (2 * w + 20, 0))
    out.save(path)


def draw_thumbs(rgb, kind, path):
    sizes = THUMBS.get(kind, [(320, 180)])
    tiles = []
    for size in sizes:
        t = downscale(rgb, size, fit=kind in ("capsule", "icon"))
        sc = max(1, 360 // t.size[1]) if t.size[1] < 180 else 1
        big = t.resize((t.size[0] * sc, t.size[1] * sc), Image.NEAREST)
        tiles += [t, big, big.convert("L").convert("RGB")]
    W = sum(x.size[0] for x in tiles) + 20 * (len(tiles) + 1); H = max(x.size[1] for x in tiles) + 40
    canvas = Image.new("RGB", (W, H), (0x1b, 0x28, 0x38)); x = 20
    for tl in tiles:
        canvas.paste(tl, (x, 20)); x += tl.size[0] + 20
    canvas.save(path)


def run(path, out=None, kind="photo", genre=None, lang=None):
    global LANG
    if lang:
        LANG = lang
    name = os.path.splitext(os.path.basename(path))[0]
    out = out or f"image-review-{name}"
    os.makedirs(out, exist_ok=True)
    m, rgb, mag, fg = analyze(path, kind, genre)
    draw_palette(m["palette"], os.path.join(out, "palette.png"))
    ov = rgb.copy(); ov.thumbnail((1600, 1600))
    draw_overlay(ov, mag, m["composition"], os.path.join(out, "thirds-overlay.png"))
    with open(os.path.join(out, "metrics.json"), "w", encoding="utf-8") as f:
        json.dump(m, f, ensure_ascii=False, indent=2)
    with open(os.path.join(out, "report.md"), "w", encoding="utf-8") as f:
        f.write(build_report(m))
    if kind in GAME_TYPES:
        draw_gray(rgb, fg, os.path.join(out, "grayscale.png"))
        draw_thumbs(rgb, kind, os.path.join(out, "thumbnail.png"))
    return m, out


def main():
    global LANG
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("image", nargs="?")
    ap.add_argument("--genre", choices=sorted(GENRES), help="game genre profile (game types only)")
    ap.add_argument("--list-genres", action="store_true")
    ap.add_argument("--out")
    ap.add_argument("--type", default="photo", choices=TYPES)
    ap.add_argument("--lang", choices=["en", "zh"], default=None, help=f"report language (default {LANG})")
    a = ap.parse_args()
    if a.lang:
        LANG = a.lang
    if a.list_genres:
        for k, v in GENRES.items():
            print(f"{k:14s} {genre_name(k)}")
        return
    if not a.image:
        ap.error("image is required")
    if a.genre and a.type not in GAME_TYPES:
        ap.error("--genre needs a game --type (" + "|".join(GAME_TYPES) + ")")
    if not os.path.isfile(a.image):
        sys.exit(f"not found: {a.image}")
    m, out = run(a.image, a.out, a.type, a.genre, a.lang)
    print(f"metric_score={m['metric_score']} colors={m['palette_significant_colors']} harmony={m['harmony_guess']} "
          f"sharp={m['sharpness_laplacian_var']} noise={m['noise_sigma']} whitespace={m['whitespace_ratio']}")
    for fl in m["flags"]:
        print("  " + fl)
    extra = ", grayscale.png, thumbnail.png" if a.type in GAME_TYPES else ""
    print(f"wrote {out}/metrics.json, report.md, palette.png, thirds-overlay.png{extra}")


if __name__ == "__main__":
    main()
