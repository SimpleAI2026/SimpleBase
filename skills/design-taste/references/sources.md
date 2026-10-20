# Sources & Licenses

This skill is an original synthesis. Ideas were summarized and rewritten in our own words from the skills below; no large verbatim passages were copied. Specific color values in `minimalist.md` are common, generic starting values used as examples. Check each upstream repository for the authoritative, current license before reusing their text.

| Local skill (name) | Upstream | License (as observed locally) | Used for |
|---|---|---|---|
| frontend-design | https://github.com/anthropics/skills (skills/frontend-design) | Apache-2.0 (LICENSE.txt bundled) | direction-first thinking, tone commitment, anti-generic aesthetics |
| canvas-design | https://github.com/anthropics/skills (skills/canvas-design) | Apache-2.0 (LICENSE.txt bundled) | philosophy → canvas process for posters/art |
| theme-factory | https://github.com/anthropics/skills (skills/theme-factory) | Apache-2.0 (LICENSE.txt bundled) | theme = palette + fonts, theming slides/docs |
| ui-ux-pro-max | https://github.com/nextlevelbuilder/ui-ux-pro-max-skill | MIT per upstream repo (no LICENSE file in local copy; verify) | UX rules, accessibility, palette/type reasoning |
| design-taste-frontend (taste-skill) | https://github.com/Leonxlnx/taste-skill | MIT (GitHub API, verified 2026-10-13) | anti-slop list, pre-flight checks |
| redesign-existing-projects (taste-redesign) | https://github.com/Leonxlnx/taste-skill | Not stated in local copy — verify upstream | audit-first redesign workflow |
| minimalist-ui (taste-minimalist) | https://github.com/Leonxlnx/taste-skill | Not stated in local copy — verify upstream | warm monochrome editorial style (paraphrased) |
| impeccable | https://github.com/pbakaus/impeccable | Apache-2.0 per upstream (no LICENSE in local copy; verify) | audit/critique/polish/bolder/quieter/distill vocabulary, motion & typography guidance |
| design-system (ui-ux-design-system) | ClaudeKit skills (author: claudekit), e.g. https://github.com/mrgoonie/claudekit-skills | MIT (frontmatter) | 3-tier tokens, component specs |
| oil-ui | https://github.com/oil-oil/oil-ui | MIT (LICENSE bundled) | exploring/comparing directions, screenshot-based review |

Not carried over: bundled datasets (CSV/JSON catalogs), fonts, vendor JS, search/update scripts, and the auto-update hooks of some upstream skills. The skill's own scripts (`review_site.py`, `review_image.py`) are original code and perform no downloads.

## Image aesthetics sources (verified 2026-10-13)

Used for `image-aesthetics.md`, `image-workflows.md` and `review_image.py`. All summarized in our own words; no code or long text copied. Licenses from the GitHub API unless noted.

| Source | URL | License | What we took (paraphrased) |
|---|---|---|---|
| Taste Skill (Leonxlnx) | https://github.com/Leonxlnx/taste-skill | MIT | anti-slop stance; repo also has `imagegen-frontend-*`, `image-to-code-skill` (not copied) |
| Apito — "9 个照片处理 Skill" roundup | https://apito.ai/zh/blog/tools/photo-enhancement-skills-agents-apito/ | article (all rights reserved) | idea: judge subject/space/color/mood before choosing zine, collage, product or editorial treatment |
| UIED / 优设 coverage | https://www.uied.cn/posts/922020 , https://www.uisdc.com/travel-poster-skill | articles | recipe overview; "one composition, one material, one color source, ≤1 decoration" rule (Yingzao) |
| gc-minimal-zine-poster | https://github.com/LiamGvchi/gc-minimal-zine-poster | MIT | 3:5 aged paper, 70–90% empty, subject 8–25%, one saturated accent |
| gathered-scenes-zine-skill (拾景纸刊) | https://github.com/kzzzp/gathered-scenes-zine-skill | MIT | keep-photo vs distill split; separate photo colors from added print color; series color card |
| photo-abstract-editorial | https://github.com/ZzzLc0405/photo-abstract-editorial | NOASSERTION (no SPDX license detected — treat as all rights reserved) | photo on top + ivory abstract panel idea only |
| Yingzao 营造 (guizang) | https://github.com/op7418/guizang-yingzao-skill | none detected | palette from the photo; one-of-each restraint rule |
| nature-skills / nature-figure | https://github.com/yuan1z0825/nature-skills | Apache-2.0 | conclusion-first figure contract, hero panel, lowercase bold panel labels, one hue family per method, figure QA |
| Nature research figure guide | https://research-figure-guide.nature.com/figures/building-and-exporting-figure-panels/ | Springer Nature web page | 5–7 pt text, standard fonts, embedded fonts, avoid red/green and rainbow, avoid decorative icons |
| happy-figure / happy-figure-skill (Datawhale) | https://github.com/BAIKEMARK/happy-figure-skill (tutorial: https://github.com/datawhalechina/happy-figure) | NOASSERTION (check repo) | scientific-figure prompts by domain × figure type × model; text-light generation |
| colorgram.py | https://github.com/obskyr/colorgram.py | MIT | palette-extraction idea (we use our own numpy k-means) |
| Real-ESRGAN | https://github.com/xinntao/Real-ESRGAN | BSD-3-Clause | optional upscale step (not bundled) |
| SPO — step-by-step preference optimization (MSRA co-authors, e.g. Yuhui Yuan) | https://arxiv.org/abs/2406.04314 , code https://github.com/RockeyCoss/SPO (MIT) | paper; code MIT | aesthetics improve by accumulating fine-detail preferences per denoising step, judged separately from layout |
| SGPO — stage-guided per-step optimization (Peking U., not MSRA) | https://arxiv.org/abs/2608.06768 | arXiv | early/mid/late denoising stages; over-optimizing aesthetic reward → reward hacking (repetitive, over-amplified detail) — informs the "AI slop" checklist |
| AVA dataset (Murray et al., CVPR 2012) | https://ieeexplore.ieee.org/document/6247954 | research dataset (image rights with photographers) | aesthetic score distributions, style/semantic tags |
| NIMA (Talebi & Milanfar, 2017) | https://arxiv.org/abs/1709.05424 ; impl. https://github.com/idealo/image-quality-assessment (Apache-2.0) | paper / Apache-2.0 | predict score distribution, not a single number |
| LAION improved aesthetic predictor | https://github.com/christophschuhmann/improved-aesthetic-predictor | Apache-2.0 | CLIP-embedding + MLP 1–10 aesthetic score (used as reward "AES" in diffusion RL papers) |
| Q-Align / OneAlign | https://github.com/Q-Future/Q-Align | S-Lab License 1.0 (non-commercial) | text-defined rating levels (bad…excellent) for IQA/IAA scoring |
| AesBench | https://github.com/yipoh/AesBench | Apache-2.0 | MLLM aesthetic perception / empathy / assessment / interpretation |
| UNIAA | https://arxiv.org/abs/2404.09619 ; https://github.com/KwaiVGI/Uniaa | license not stated in repo metadata — verify | attribute set: content/theme, composition, color, light, focus, sentiment (mirrors our D1–D7) |
| Hasler & Süsstrunk colorfulness (2003) | https://doi.org/10.1117/12.477378 | paper | colorfulness formula in `review_image.py` |
| Immerkær fast noise estimation (1996) | https://doi.org/10.1006/cviu.1996.0060 | paper | noise sigma in `review_image.py` |
| Okabe & Ito colorblind-safe palette | https://jfly.uni-koeln.de/color/ | web page | categorical palette for figures |

Unverified / flagged: no single "MSRA stage-wise aesthetic preference" paper was found under that exact description; the closest real works are SPO (MSRA co-authored) and SGPO (PKU, stage-wise). "Figma vectorize" as part of happy-figure appears only as a tutorial chapter topic (矢量化重构, marked in progress), not as a tool. X search (2 queries) surfaced no image-aesthetic skill beyond the above.

## Game art sources (game-aesthetics.md, game-workflows.md, review_image.py game types)

Researched 2026-10-13. Summarised in our own words; no text, images, or code copied. Licenses are as stated by the source or the GitHub API.

| Source | URL | License | What we took (paraphrased) |
|---|---|---|---|
| Steamworks — Store Graphical Assets | https://partner.steamgames.com/doc/store/assets/standard | Valve docs (all rights reserved) | small capsule 462×174 auto-scaled to 184×69 & 120×45; logo must stay legible, may nearly fill capsule; small capsule may use different art |
| Steamworks — Graphical Asset Rules | https://partner.steamgames.com/doc/store/assets/rules | Valve docs | base capsules = artwork + game name + official subtitle only; no review scores, awards, discount or CTA text; PG-13 |
| Steam capsule checklist (Steam Analyzer) | https://www.steamanalyser.com/steam-capsule-checklist | article | 120×45 grayscale test on Steam navy `#1b2838`; design title for smallest size first |
| Immutable — Steam small capsule | https://www.immutable.com/insights/steam-small-capsule | article | one subject; multiple characters merge when reduced |
| Apple — Creating your product page / asset best practices / screenshot specs | https://developer.apple.com/app-store/product-page/ , https://developer.apple.com/app-store/asset-best-practices/ , https://developer.apple.com/help/app-store-connect/reference/app-information/screenshot-specifications/ | Apple docs | first 1–3 screenshots appear in search; show the app/game in use; up to 10; no alpha |
| App Store Review Guideline 2.3.3 (via Gummicube) | https://www.gummicube.com/blog/aso-academy/app-store-screenshot-dimensions-and-guidelines/ | article quoting Apple | screenshots show the app in use, not only title art/splash |
| Game UI Database | https://www.gameuidatabase.com/ | site (screenshots © respective publishers) | reference archive filterable by HUD element/genre — used as a "look up conventions" pointer only |
| Gamasutra/Game Developer — UI design in video games (diegetic / non-diegetic / spatial / meta, after Fagerholt & Lorentzon) | https://www.gamedeveloper.com/design/user-interface-design-in-video-games | article | UI taxonomy; competitive games favour non-diegetic HUD |
| Unity blog — immersive UI | https://unity.com/blog/games/how-to-immerse-your-players-through-effective-ui-and-game-design | article | genre conventions (e.g. FPS health bottom), study genre peers |
| Riot — Clarity in League; League VFX style guide | https://www.leagueoflegends.com/en-us/news/dev/clarity-in-league/ , https://nexus.leagueoflegends.com/en-us/2017/10/dev-leagues-vfx-style-guide/ | Riot pages / public PDF | VFX intensity ∝ gameplay impact, hitbox-accurate shapes, skin clarity parity, anticipation→dissipation |
| GDC 2012 The Art of Diablo III (Lichtner), coverage | https://gameranx.com/features/id/5666/article/gdc-2012-the-art-of-diablo-iii/ | article | legibility as #1 principle; class/monster silhouettes; environment as canvas |
| GDC 2015 The Art of Firewatch (Jane Ng) | https://www.thumbsticks.com/gdc-2015-the-art-of-firewatch/ , https://www.youtube.com/watch?v=ZYnS3kKTcGg | article / talk video | color script per story beat; layered colors for depth; flat shapes with strong silhouettes; detail density signals importance |
| GDC 2017 The Art of Overwatch (Petras, Tsang) | https://www.gdcvault.com/play/1024268/The-Art-of-Overwatch-Evolving | GDC Vault (talk not freely viewable) | hero silhouettes readable at distance incl. animation; only summary-level claims used |
| 80.lv — Shape Language and Readability | https://80.lv/articles/character-design-shape-language-and-readability | article | "detail everywhere = no detail"; rest areas; block big shapes first |
| Olgskaia-Mustonen thesis (Theseus) | https://www.theseus.fi/bitstream/handle/10024/904059/Olgskaia-Mustonen_Kristina.pdf | thesis (academic, all rights reserved) | silhouette + value patterning for 3D character readability |
| game-art SKILL.md (sickn33/agentic-awesome-skills) | https://github.com/sickn33/agentic-awesome-skills/blob/main/skills/game-development/game-art/SKILL.md | MIT | existing agent skill: style decision tree, "art serves gameplay", test at gameplay distance — ours adds scoring, genre profiles, metrics |
| Other agent skills found (not used beyond confirming the gap) | https://github.com/ianlintner/ai-pixel-art-image-generation (MIT), https://github.com/isabellagreco1997/sprite-forge (MIT), https://github.com/majidmanzarpour/blender-game-skills (MIT) | MIT | pixel-art generation / sprite tooling exists; no game-art *critique* rubric skill found |
| Otsu thresholding (1979) | https://doi.org/10.1109/TSMC.1979.4310076 | paper | figure-ground mask & separability in `review_image.py` |
| Test images (local only, not redistributed) | Steam public CDN, e.g. https://shared.akamai.steamstatic.com/store_item_assets/steam/apps/1145360/… | © respective publishers | Hades, Hollow Knight, Dota 2, Baldur's Gate 3 screenshots/capsules used only for local script testing |

Genre palettes (e.g. 石青/朱砂 pigments, 墨分五色), HUD conventions per genre and numeric targets are our synthesis from common practice — **not** from a single citable standard; treat numbers as heuristics.

Unverified / blocked:
- **X**: 3 searches (English readability tips, Steam capsule, Chinese 游戏美术 可读性) returned 1, 1, 0 posts — one indie dev asking for capsule readability feedback; nothing citable as guidance.
- **Facebook**: groups (e.g. "Creative Crossroads – Game Art Showcase", "Game Artists Connect") are only discoverable via third-party mentions; content requires login, not read or cited.
- GDC Vault talks (Overwatch) are paywalled; claims come from public coverage only. Riot VFX PDF not opened directly (summary via search results).
- Game UI Database content (screenshots) is copyrighted by publishers; used only as a pointer.
