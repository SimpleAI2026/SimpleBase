# 三步翻译完整提示词模板

本文件保存与开源项目 [andrewyng/translation-agent](https://github.com/andrewyng/translation-agent) 对齐的原始英文提示词。需要严格复刻其行为时直接套用。占位符 `{source_lang}`、`{target_lang}`、`{source_text}`、`{translation_1}`、`{reflection}`、`{country}` 在使用前替换为实际值。

---

## Step 1: Initial Translation

**System**:
```
You are an expert linguist, specializing in translation from {source_lang} to {target_lang}.
```

**User**:
```
This is an {source_lang} to {target_lang} translation, please provide the {target_lang} translation for this text. Do not provide any explanations or text apart from the translation.
{source_lang}: {source_text}

{target_lang}:
```

输出保存为 `translation_1`。

---

## Step 2: Reflection

**System**:
```
You are an expert linguist specializing in translation from {source_lang} to {target_lang}. You will be provided with a source text and its translation and your goal is to improve the translation.
```

### 2A — 当指定了 country（地区变体）时

**User**:
```
Your task is to carefully read a source text and a translation from {source_lang} to {target_lang}, and then give constructive criticism and helpful suggestions to improve the translation. The final style and tone of the translation should match the style of {target_lang} colloquially spoken in {country}.

The source text and initial translation, delimited by XML tags <SOURCE_TEXT></SOURCE_TEXT> and <TRANSLATION></TRANSLATION>, are as follows:

<SOURCE_TEXT>
{source_text}
</SOURCE_TEXT>

<TRANSLATION>
{translation_1}
</TRANSLATION>

When writing suggestions, pay attention to whether there are ways to improve the translation's
(i) accuracy (by correcting errors of addition, mistranslation, omission, or untranslated text),
(ii) fluency (by applying {target_lang} grammar, spelling and punctuation rules, and ensuring there are no unnecessary repetitions),
(iii) style (by ensuring the translations reflect the style of the source text and takes into account any cultural context),
(iv) terminology (by ensuring terminology use is consistent and reflects the source text domain; and by only ensuring you use equivalent idioms {target_lang}).

Write a list of specific, helpful and constructive suggestions for improving the translation.
Each suggestion should address one specific part of the translation.
Output only the suggestions and nothing else.
```

### 2B — 未指定 country 时

**User**:
```
Your task is to carefully read a source text and a translation from {source_lang} to {target_lang}, and then give constructive criticism and helpful suggestions to improve the translation.

The source text and initial translation, delimited by XML tags <SOURCE_TEXT></SOURCE_TEXT> and <TRANSLATION></TRANSLATION>, are as follows:

<SOURCE_TEXT>
{source_text}
</SOURCE_TEXT>

<TRANSLATION>
{translation_1}
</TRANSLATION>

When writing suggestions, pay attention to whether there are ways to improve the translation's
(i) accuracy (by correcting errors of addition, mistranslation, omission, or untranslated text),
(ii) fluency (by applying {target_lang} grammar, spelling and punctuation rules, and ensuring there are no unnecessary repetitions),
(iii) style (by ensuring the translations reflect the style of the source text and takes into account any cultural context),
(iv) terminology (by ensuring terminology use is consistent and reflects the source text domain; and by only ensuring you use equivalent idioms {target_lang}).

Write a list of specific, helpful and constructive suggestions for improving the translation.
Each suggestion should address one specific part of the translation.
Output only the suggestions and nothing else.
```

输出保存为 `reflection`。

---

## Step 3: Improved Translation

**System**:
```
You are an expert linguist, specializing in translation editing from {source_lang} to {target_lang}.
```

**User**:
```
Your task is to carefully read, then edit, a translation from {source_lang} to {target_lang}, taking into account a list of expert suggestions and constructive criticisms.

The source text, the initial translation, and the expert linguist suggestions are delimited by XML tags <SOURCE_TEXT></SOURCE_TEXT>, <TRANSLATION></TRANSLATION> and <EXPERT_SUGGESTIONS></EXPERT_SUGGESTIONS> as follows:

<SOURCE_TEXT>
{source_text}
</SOURCE_TEXT>

<TRANSLATION>
{translation_1}
</TRANSLATION>

<EXPERT_SUGGESTIONS>
{reflection}
</EXPERT_SUGGESTIONS>

Please take into account the expert suggestions when editing the translation. Edit the translation by ensuring:

(i) accuracy (by correcting errors of addition, mistranslation, omission, or untranslated text),
(ii) fluency (by applying {target_lang} grammar, spelling and punctuation rules and ensuring there are no unnecessary repetitions),
(iii) style (by ensuring the translations reflect the style of the source text)
(iv) terminology (inappropriate for context, inconsistent use), or
(v) other errors.

Output only the new translation and nothing else.
```

输出保存为 `translation_2` —— 即最终交付给用户的精稿。

---

## XML 标签使用规范

- 三个标签 `<SOURCE_TEXT>`、`<TRANSLATION>`、`<EXPERT_SUGGESTIONS>` 必须**一一闭合**，不要嵌套。
- 标签内部内容前后各加一个换行，便于模型识别边界。
- 如果原文本身包含 `<` `>` 或类似标签字符串（例如要翻译的就是一段 HTML），把原文整体放进 `<SOURCE_TEXT>` 即可，模型能够正确识别外层标签。

---

## 长文分块版本（Multi-chunk）

原项目在长文本时会把全文按块标注，每次只翻译其中一块，但把其余部分作为上下文提供，以保持术语和指代一致。三步都使用同一种结构：

`{tagged_text}` = 全文，其中当前块用 `<TRANSLATE_THIS>` 包裹，其余块原样保留。

**Step 1 User**：
```
Your task is to provide a professional translation from {source_lang} to {target_lang} of PART of a text.

The source text is below, delimited by XML tags <SOURCE_TEXT> and </SOURCE_TEXT>. Translate only the part within the source text delimited by <TRANSLATE_THIS> and </TRANSLATE_THIS>. You can use the rest of the source text as context, but do not translate any of the other text. Do not output anything other than the translation of the indicated part of the text.

<SOURCE_TEXT>
{tagged_text}
</SOURCE_TEXT>

To reiterate, you should translate only this part of the text, shown here again between <TRANSLATE_THIS> and </TRANSLATE_THIS>:
<TRANSLATE_THIS>
{chunk_to_translate}
</TRANSLATE_THIS>

Output only the translation of the portion you are asked to translate, and nothing else.
```

**Step 2 User（分块反思）**：
```
Your task is to carefully read a source text and part of a translation of that text from {source_lang} to {target_lang}, and then give constructive criticism and helpful suggestions for improving the translation.

The source text is below, delimited by XML tags <SOURCE_TEXT> and </SOURCE_TEXT>, and the part that has been translated is delimited by <TRANSLATE_THIS> and </TRANSLATE_THIS> within the source text. You can use the rest of the source text as context for critiquing the translated part.

<SOURCE_TEXT>
{tagged_text}
</SOURCE_TEXT>

To reiterate, only part of the text is being translated, shown here again between <TRANSLATE_THIS> and </TRANSLATE_THIS>:
<TRANSLATE_THIS>
{chunk_to_translate}
</TRANSLATE_THIS>

The translation of the indicated part, delimited below by <TRANSLATION> and </TRANSLATION>, is as follows:
<TRANSLATION>
{translation_1_chunk}
</TRANSLATION>

The required glossary is:
<GLOSSARY>
{glossary}
</GLOSSARY>

When writing suggestions, pay attention to whether there are ways to improve the translation's:
(i) accuracy (by correcting errors of addition, mistranslation, omission, or untranslated text),
(ii) fluency (by applying {target_lang} grammar, spelling and punctuation rules, and ensuring there are no unnecessary repetitions),
(iii) style (by ensuring the translations reflect the style of the source text and take into account any cultural context),
(iv) terminology (by ensuring terminology use is consistent with the glossary and reflects the source text domain),
(v) structure (every heading and paragraph present, Markdown and code blocks preserved).

Write a list of specific, helpful and constructive suggestions for improving the translation. Each suggestion should address one specific part of the translation. Output only the suggestions and nothing else.
```
指定了 `country` 时，在第一段末尾追加："The final style and tone of the translation should match the style of {target_lang} colloquially spoken in {country}."

**Step 3 User（分块精稿）**：
```
Your task is to carefully read, then improve, a translation from {source_lang} to {target_lang}, taking into account a set of expert suggestions and constructive criticisms.

The source text is below, delimited by XML tags <SOURCE_TEXT> and </SOURCE_TEXT>, and the part that has been translated is delimited by <TRANSLATE_THIS> and </TRANSLATE_THIS> within the source text. You can use the rest of the source text as context, but need to provide a translation only of the part indicated by <TRANSLATE_THIS> and </TRANSLATE_THIS>.

<SOURCE_TEXT>
{tagged_text}
</SOURCE_TEXT>

To reiterate, only part of the text is being translated, shown here again between <TRANSLATE_THIS> and </TRANSLATE_THIS>:
<TRANSLATE_THIS>
{chunk_to_translate}
</TRANSLATE_THIS>

<TRANSLATION>
{translation_1_chunk}
</TRANSLATION>

<EXPERT_SUGGESTIONS>
{reflection_chunk}
</EXPERT_SUGGESTIONS>

Glossary (must be followed):
<GLOSSARY>
{glossary}
</GLOSSARY>

Taking into account the expert suggestions, rewrite the translation to improve it, paying attention to accuracy, fluency, style and terminology. Output only the new translation of the indicated part and nothing else.
```

## 术语表生成（长文分块前）

```
Read the {source_lang} document below and build a translation glossary for its recurring domain terms. Choose ONE standard, idiomatic {target_lang} rendering per term, as used in mainstream {target_lang} literature of that domain (prefer the most widely used term over a literal one). Output one line per term in the form: source term => target rendering

<SOURCE_TEXT>
{source_text}
</SOURCE_TEXT>
```
Step 1 分块模板中加入同一个 `<GLOSSARY>` 块，并要求严格遵守。

> 若由外部脚本调用模型、模型输出可能夹带推理文字，可在每个提示末尾要求"把结果完整放在 <OUTPUT></OUTPUT> 之间"，再只提取标签内内容。
