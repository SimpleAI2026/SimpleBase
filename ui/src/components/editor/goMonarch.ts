/**
 * Go 语言的 Monarch 语法定义（ui-gofunction-plan §8.5）。
 * Monaco 无内置 Go；此处只做高亮（关键字/类型/注释/字符串/struct tag），
 * 不做 LSP / 补全。
 */

export const goMonarchLanguage = {
  id: 'go',
  extensions: ['.go'],
  keywords: [
    'break',
    'case',
    'chan',
    'const',
    'continue',
    'default',
    'defer',
    'else',
    'fallthrough',
    'for',
    'func',
    'go',
    'goto',
    'if',
    'import',
    'interface',
    'map',
    'package',
    'range',
    'return',
    'select',
    'struct',
    'switch',
    'type',
    'var'
  ],
  namespaceKeywords: ['package', 'import'],
  typeKeywords: [
    'bool',
    'byte',
    'complex64',
    'complex128',
    'error',
    'float32',
    'float64',
    'int',
    'int8',
    'int16',
    'int32',
    'int64',
    'rune',
    'string',
    'uint',
    'uint8',
    'uint16',
    'uint32',
    'uint64',
    'uintptr',
    'any'
  ],
  operators: [
    '=',
    '>',
    '<',
    '!',
    '~',
    '?',
    ':',
    '==',
    '<=',
    '>=',
    '!=',
    '&&',
    '||',
    '++',
    '--',
    '+',
    '-',
    '*',
    '/',
    '&',
    '|',
    '^',
    '%',
    '<<',
    '>>',
    '>>>',
    '+=',
    '-=',
    '*=',
    '/=',
    '&=',
    '|=',
    '^=',
    '%=',
    '<<=',
    '>>=',
    '>>>='
  ],
  symbols: /[=><!~?:&|+\-*/^%]+/,
  escapes: /\\(?:[abfnrtv\\"']|x[0-9A-Fa-f]{1,4}|u[0-9A-Fa-f]{4}|U[0-9A-Fa-f]{8})/,
  tokenizer: {
    root: [
      // struct tag（反引号字符串）
      [/`[^`]*`/, 'string'],

      // 注释
      [/\/\/.*$/, 'comment'],
      [/\/\*/, 'comment', '@comment'],

      // 字符串
      [/"([^"\\]|\\.)*$/, 'string.invalid'],
      [/"/, { token: 'string.quote', bracket: '@open', next: '@string' }],

      // 数字
      [/[0-9]+\.[0-9]+([eE][-+]?[0-9]+)?/, 'number.float'],
      [/0[xX][0-9a-fA-F]+/, 'number.hex'],
      [/[0-9]+/, 'number'],

      // 声明
      [
        /[a-zA-Z_$][\w$]*/,
        {
          cases: {
            '@namespaceKeywords': 'keyword',
            '@keywords': 'keyword',
            '@typeKeywords': 'type',
            '@default': 'identifier'
          }
        }
      ],

      // 空白
      { include: '@whitespace' },

      // 定界符与运算符
      [/[{}()[\]]/, '@brackets'],
      [/[<>](?!@symbols)/, '@brackets'],
      [/@symbols/, { cases: { '@operators': 'operator', '@default': '' } }],

      // 标点
      [/[;,.]/, 'delimiter']
    ],
    comment: [
      [/[^/*]+/, 'comment'],
      [/\*\//, 'comment', '@pop'],
      [/[/*]/, 'comment']
    ],
    string: [
      [/[^\\"]+/, 'string'],
      [/@escapes/, 'string.escape'],
      [/\\./, 'string.escape.invalid'],
      [/"/, { token: 'string.quote', bracket: '@close', next: '@pop' }]
    ],
    whitespace: [
      [/[ \t\r\n]+/, ''],
      [/\/\*/, 'comment', '@comment'],
      [/\/\/.*$/, 'comment']
    ]
  }
}
