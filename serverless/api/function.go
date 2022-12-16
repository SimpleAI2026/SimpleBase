package api

import (
	"github.com/linkxzhou/gongx/packages/server"
)

func FunctionMetrics(c server.Context) interface{} {
	code := 0
	message := emptyString
	data := map[string]interface{}{
		"access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1aWQiOiI2MzRmNzMzMmNjYWNiOGU2NmI1MjlmNDgiLCJleHAiOjE2Njg4MjkyMzUsImlhdCI6MTY2ODc0MjgzNX0._1pf26on-X0RQFzTPzY6JL1Ny0j_YGevjxKbeFNjTLY",
		"username":     "zhoulv",
		"uid":          "634f7332ccacb8e66b529f48",
		"expire":       1668829235,
	}

	return CommonResp(code, message, data)
}

func FunctionCreate(c server.Context) interface{} {
	code := 0
	message := emptyString
	data := map[string]interface{}{
		"insertedId": "111",
	}

	return CommonResp(code, message, data)
}

func FunctionPublish(c server.Context) interface{} {
	code := 0
	message := emptyString

	return CommonResp(code, message, nil)
}

func FunctionInfo(c server.Context) interface{} {
	code := 0
	message := emptyString
	data := map[string]interface{}{
		"_id":          "634f74161e1d2d44d065cca2",
		"name":         "Helloworld",
		"code":         "\n\nimport cloud from '@/cloud-sdk'\n\nexports.main = async function (ctx: FunctionContext) {\n  // body, query 为请求参数, auth 是授权对象\n  const { auth, body, query } = ctx\n\n  // 数据库操作\n  const db = cloud.database()\n  const r = await db.collection('admins').get()\n  console.log(r)\n\n  return r.data\n}\n",
		"description":  "",
		"enableHTTP":   true,
		"status":       1,
		"compiledCode": "\"use strict\";\nObject.defineProperty(exports, \"__esModule\", { value: true });\nconst cloud_sdk_1 = require(\"@/cloud-sdk\");\nexports.main = async function (ctx) {\n    const { auth, body, query } = ctx;\n    const db = cloud_sdk_1.default.database();\n    const r = await db.collection('admins').get();\n    console.log(r);\n    return r.data;\n};\n",
		"tags":         []interface{}{},
		"triggers": []interface{}{
			map[string]interface{}{
				"_id":        "6376fe5c7fd9b2915dfb7bbd",
				"name":       "测试触发器",
				"type":       "event",
				"event":      "1111",
				"duration":   nil,
				"status":     1,
				"desc":       "",
				"created_at": "2022-11-18T03:39:08.403Z",
				"updated_at": "2022-11-18T03:39:08.403Z",
			},
		},
		"label":      "Helloworld",
		"version":    1,
		"hash":       "557cf05419f30d5a0c1bae67a696cb79",
		"created_at": "2022-10-19T03:50:46.395Z",
		"updated_at": "2022-11-18T03:39:08.403Z",
		"created_by": "634f7332ccacb8e66b529f48",
		"appid":      "u3nnf8",
	}

	return CommonResp(code, message, data)
}

func FunctionPublishedList(c server.Context) interface{} {
	code := 0
	message := emptyString
	data := []interface{}{
		map[string]interface{}{
			"_id":          "634f74161e1d2d44d065cca2",
			"name":         "Helloworld",
			"code":         "\n\nimport cloud from '@/cloud-sdk'\n\nexports.main = async function (ctx: FunctionContext) {\n  // body, query 为请求参数, auth 是授权对象\n  const { auth, body, query } = ctx\n\n  // 数据库操作\n  const db = cloud.database()\n  const r = await db.collection('admins').get()\n  console.log(r)\n\n  return r.data\n}\n",
			"description":  "",
			"enableHTTP":   true,
			"status":       1,
			"compiledCode": "\"use strict\";\nObject.defineProperty(exports, \"__esModule\", { value: true });\nconst cloud_sdk_1 = require(\"@/cloud-sdk\");\nexports.main = async function (ctx) {\n    const { auth, body, query } = ctx;\n    const db = cloud_sdk_1.default.database();\n    const r = await db.collection('admins').get();\n    console.log(r);\n    return r.data;\n};\n",
			"tags":         []interface{}{},
			"triggers":     []interface{}{},
			"label":        "Helloworld",
			"version":      0,
			"hash":         "557cf05419f30d5a0c1bae67a696cb79",
			"created_at":   "2022-10-19T03:50:46.395Z",
			"updated_at":   "2022-10-19T03:50:46.395Z",
			"created_by":   "634f7332ccacb8e66b529f48",
			"appid":        "u3nnf8",
		},
	}

	return CommonResp(code, message, data)
}

func FunctionChangeHistory(c server.Context) interface{} {
	code := 0
	message := emptyString
	data := []interface{}{}

	return CommonResp(code, message, data)
}

func FunctionList(c server.Context) interface{} {
	code := 0
	message := emptyString
	data := map[string]interface{}{
		"list": []interface{}{
			map[string]interface{}{
				"_id":         "634f74161e1d2d44d065cca2",
				"name":        "Helloworld",
				"code":        "\n\nimport cloud from '@/cloud-sdk'\n\nexports.main = async function (ctx: FunctionContext) {\n  // body, query 为请求参数, auth 是授权对象\n  const { auth, body, query } = ctx\n\n  // 数据库操作\n  const db = cloud.database()\n  const r = await db.collection('admins').get()\n  console.log(r)\n\n  return r.data\n}\n",
				"description": "",
				"enableHTTP":  true,
				"status":      1,
				"tags":        []interface{}{},
				"triggers": []interface{}{
					map[string]interface{}{
						"_id":        "6376fe5c7fd9b2915dfb7bbd",
						"name":       "测试触发器",
						"type":       "event",
						"event":      "1111",
						"duration":   nil,
						"status":     1,
						"desc":       "",
						"created_at": "2022-11-18T03:39:08.403Z",
						"updated_at": "2022-11-18T03:39:08.403Z",
					},
				},
				"label":      "Helloworld",
				"version":    1,
				"hash":       "557cf05419f30d5a0c1bae67a696cb79",
				"created_at": "2022-10-19T03:50:46.395Z",
				"updated_at": "2022-11-18T03:39:08.403Z",
				"created_by": "634f7332ccacb8e66b529f48",
				"appid":      "u3nnf8",
			}, map[string]interface{}{
				"_id":         "634f7374b151ccd9cd673f47",
				"name":        "admin-create",
				"code":        "import cloud from '@/cloud-sdk'\nimport * as crypto from 'crypto'\nconst db = cloud.database()\n\nexports.main = async function (ctx: FunctionContext) {\n  const uid = ctx.auth?.uid\n  if (!uid) {\n    return 'Unauthorized'\n  }\n\n  // 权限验证\n  const code = await checkPermission(uid, 'admin.create')\n  if (code) {\n    return 'Permission denied'\n  }\n\n  const { username, password, avatar, name, roles } = ctx.body\n  if (!username || !password) {\n    return 'username or password cannot be empty'\n  }\n\n  // 验证用户是否已存在\n  const { total } = await db.collection('admins').where({ username }).count()\n  if (total > 0) {\n    return 'username already exists'\n  }\n\n  // 验证 roles 是否合法\n  const { total: valid_count } = await db.collection('roles')\n    .where({\n      name: db.command.in(roles)\n    }).count()\n\n  if (valid_count !== roles.length) {\n    return 'invalid roles'\n  }\n\n  // add admin\n  const r = await db.collection('admins')\n    .add({\n      username,\n      name: name ?? null,\n      avatar: avatar ?? null,\n      roles: roles ?? [],\n      password: hashPassword(password),\n      created_at: Date.now(),\n      updated_at: Date.now()\n    })\n\n  return {\n    ...r,\n    uid: r.id\n  }\n}\n\n\n/**\n * 通过 user id 获取权限列表\n * @param role_ids \n * @returns \n */\nasync function getPermissions(uid: string) {\n  const db = cloud.database()\n  // 查用户\n  const { data: admin } = await db.collection('admins')\n    .where({ _id: uid })\n    .getOne()\n\n\n  // 查角色\n  const { data: roles } = await db.collection('roles')\n    .where({\n      name: {\n        $in: admin.roles ?? []\n      }\n    })\n    .get()\n\n  if (!roles) {\n    return { permissions: [], roles: [], user: admin }\n  }\n\n  const permissions = []\n  for (const role of roles) {\n    const perms = role.permissions ?? []\n    permissions.push(...perms)\n  }\n\n  return {\n    permissions,\n    roles: roles.map(role => role.name),\n    user: admin\n  }\n}\n\n\n\n/**\n * 判断用户是否有权限\n * @param uid 用户ID\n * @param permission 权限名\n * @returns 0 表示用户有权限， 401 表示用户未登录， 403 表示用户未授权\n */\nasync function checkPermission(uid: string, permission: string): Promise<number> {\n  if (!uid) {\n    return 401\n  }\n  const { permissions } = await getPermissions(uid)\n\n  if (!permissions.includes(permission)) {\n    return 403\n  }\n  return 0\n}\n\n/**\n * @param {string} content\n * @return {string}\n */\nfunction hashPassword(content: string): string {\n  return crypto\n    .createHash('sha256')\n    .update(content)\n    .digest('hex')\n}\n\n",
				"label":       "Admin: 创建管理员",
				"hash":        "8fae3d2468e59e4f268f28d84655edf4",
				"tags":        []string{"后台管理"},
				"description": "",
				"enableHTTP":  true,
				"status":      1,
				"triggers":    []interface{}{},
				"debugParams": "{\"username\":\"laf\",\"password\":\"laf\",\"avatar\":\"avatar\",\"name\":\"lalala\",\"roles\":[\"superadmin\"]}",
				"version":     2,
				"created_at":  "2022-10-19T03:48:02.819Z",
				"updated_at":  "2022-10-19T03:48:02.819Z",
				"created_by":  "634f7332ccacb8e66b529f48",
				"appid":       "u3nnf8",
			}, map[string]interface{}{
				"_id":         "634f7374b151ccd9cd673f48",
				"name":        "admin-edit",
				"code":        "import cloud from '@/cloud-sdk'\nimport * as crypto from 'crypto'\n\nconst db = cloud.database()\n\nexports.main = async function (ctx: FunctionContext) {\n\n  const uid = ctx.auth?.uid\n  if (!uid) return { code: '401', error: '未授权访问' }\n\n  // 权限验证\n  const code = await checkPermission(uid, 'admin.edit')\n  if (code) {\n    return { code: '403', error: 'Permission denied' }\n  }\n\n  // 参数验证\n  const { _id, username, password, avatar, name, roles } = ctx.body\n  if (!_id) {\n    return { code: 'INVALID_PARAM', error: 'admin id cannot be empty' }\n  }\n\n  // 验证 user _id 是否合法\n  const { data: admin } = await db.collection('admins').where({ _id: _id }).getOne()\n  if (!admin) {\n    return { code: 'INVALID_PARAM', error: 'user not exists' }\n  }\n\n  // 验证 roles 是否合法\n  const { total: valid_count } = await db.collection('roles')\n    .where({\n      name: db.command.in(roles)\n    }).count()\n\n  if (valid_count !== roles.length) {\n    return { code: 'INVALID_PARAM', error: 'invalid roles' }\n  }\n\n  const old = admin\n\n  // update admim\n  const data = { updated_at: Date.now() }\n\n  // update password\n  if (password) {\n    data['password'] = hashPassword(password)\n  }\n\n  // username\n  if (username && username != old.username) {\n    const { total } = await db.collection('admins').where({ username }).count()\n    if (total) return { code: 'INVALID_PARAM', error: 'username already exists' }\n    data['username'] = username\n  }\n\n  // avatar\n  if (avatar && avatar != old.avatar) {\n    data['avatar'] = avatar\n  }\n\n  // name\n  if (name && name != old.name) {\n    data['name'] = name\n  }\n\n  // roles\n  if (roles) {\n    data['roles'] = roles\n  }\n\n  console.log(_id, data)\n  const r = await db.collection('admins')\n    .where({ _id: _id })\n    .update(data)\n\n  return {\n    code: 0,\n    data: { ...r, _id }\n  }\n}\n\n\n/**\n * 通过 user id 获取权限列表\n * @param role_ids \n * @returns \n */\nasync function getPermissions(uid: string) {\n  const db = cloud.database()\n  // 查用户\n  const { data: admin } = await db.collection('admins')\n    .where({ _id: uid })\n    .getOne()\n\n\n  // 查角色\n  const { data: roles } = await db.collection('roles')\n    .where({\n      name: {\n        $in: admin.roles ?? []\n      }\n    })\n    .get()\n\n  if (!roles) {\n    return { permissions: [], roles: [], user: admin }\n  }\n\n  const permissions = []\n  for (const role of roles) {\n    const perms = role.permissions ?? []\n    permissions.push(...perms)\n  }\n\n  return {\n    permissions,\n    roles: roles.map(role => role.name),\n    user: admin\n  }\n}\n\n/**\n * 判断用户是否有权限\n * @param uid 用户ID\n * @param permission 权限名\n * @returns 0 表示用户有权限， 401 表示用户未登录， 403 表示用户未授权\n */\nasync function checkPermission(uid: string, permission: string): Promise<number> {\n  if (!uid) {\n    return 401\n  }\n  const { permissions } = await getPermissions(uid)\n\n  if (!permissions.includes(permission)) {\n    return 403\n  }\n  return 0\n}\n\n/**\n * @param {string} content\n * @return {string}\n */\nfunction hashPassword(content: string): string {\n  return crypto\n    .createHash('sha256')\n    .update(content)\n    .digest('hex')\n}\n\n",
				"label":       "Admin: 编辑管理员",
				"hash":        "f7d31a0d2b2aba882c9b94d1a6063be7",
				"tags":        []string{"后台管理"},
				"description": "",
				"enableHTTP":  true,
				"status":      1,
				"triggers":    []interface{}{},
				"debugParams": "{\"_id\":\"616fb53f91881afb7d3f609f\",\"username\":\"admin\",\"password\":\"123456\",\"name\":\"lalala\",\"roles\":[\"superadmin\"]}",
				"version":     4,
				"created_at":  "2022-10-19T03:48:02.846Z",
				"updated_at":  "2022-10-19T03:48:02.846Z",
				"created_by":  "634f7332ccacb8e66b529f48",
				"appid":       "u3nnf8",
			}, map[string]interface{}{
				"_id":         "634f7374b151ccd9cd673f49",
				"name":        "admin-getinfo",
				"code":        "import cloud from '@/cloud-sdk'\n\nexports.main = async function (ctx: FunctionContext) {\n  const db = cloud.database()\n  const uid = ctx.auth?.uid\n  if (!uid) return { code: 'NO_AUTH', error: \"permission denied\" }\n\n  const { data: admin } = await db.collection('admins')\n    .where({ _id: uid })\n    .getOne()\n\n  delete admin['password']\n  const { permissions } = await getPermissions(admin._id)\n\n  return {\n    error_code: \"0\",\n    data: {\n      ...admin,\n      permissions\n    }\n  }\n}\n\n\n/**\n * 通过 user id 获取权限列表\n * @param role_ids \n * @returns \n */\nasync function getPermissions(uid: string) {\n  const db = cloud.database()\n  // 查用户\n  const { data: admin } = await db.collection('admins')\n    .where({ _id: uid })\n    .getOne()\n\n\n  // 查角色\n  const { data: roles } = await db.collection('roles')\n    .where({\n      name: {\n        $in: admin.roles ?? []\n      }\n    })\n    .get()\n\n  if (!roles) {\n    return { permissions: [], roles: [], user: admin }\n  }\n\n  const permissions = []\n  for (const role of roles) {\n    const perms = role.permissions ?? []\n    permissions.push(...perms)\n  }\n\n  return {\n    permissions,\n    roles: roles.map(role => role.name),\n    user: admin\n  }\n}",
				"label":       "Admin: 获取管理员信息",
				"hash":        "a6926d3662f6c97a3428b6821f68107d",
				"tags":        []string{"后台管理"},
				"description": "",
				"enableHTTP":  true,
				"status":      1,
				"triggers":    []interface{}{},
				"debugParams": "{\"code\":\"laf\"}",
				"version":     2,
				"created_at":  "2022-10-19T03:48:02.870Z",
				"updated_at":  "2022-10-19T03:48:02.870Z",
				"created_by":  "634f7332ccacb8e66b529f48",
				"appid":       "u3nnf8",
			}, map[string]interface{}{
				"_id":         "634f7374b151ccd9cd673f4a",
				"name":        "admin-login",
				"code":        "\n\nimport cloud from '@/cloud-sdk'\nimport * as crypto from 'crypto'\n\nexports.main = async function (ctx: FunctionContext) {\n  const db = cloud.database()\n\n  const { username, password } = ctx.body\n  if (!username || !password)\n    return { code: 'INVALID_PARAM', error: \"账号和密码不可为空\" }\n\n  const { data: admin} = await db.collection('admins')\n    .where({ username, password: hashPassword(password) })\n    .getOne()\n\n  if (!admin)\n    return { code: 'INVALID_PARAM', error: \"账号或密码错误\" }\n\n  // 默认 token 有效期为 7 天\n  const expire = Math.floor(Date.now() / 1000) + 60 * 60 * 24 * 7\n  const payload = {\n    uid: admin._id,\n    type: 'admin',\n    exp: expire\n  }\n\n  const access_token = cloud.getToken(payload)\n  return {\n    code: 0,\n    data: {\n      access_token,\n      uid: admin._id,\n      expire\n    }\n  }\n}\n\n\n/**\n * @param {string} content\n * @return {string}\n */\nfunction hashPassword(content: string): string {\n  return crypto\n    .createHash('sha256')\n    .update(content)\n    .digest('hex')\n}\n\n",
				"label":       "Admin: 管理员登陆",
				"hash":        "784ca5db40f43ba5e264428e807ea542",
				"tags":        []string{"后台管理"},
				"description": "",
				"enableHTTP":  true,
				"status":      1,
				"triggers":    []interface{}{},
				"debugParams": "{\n  \"code\": \"laf\"\n}",
				"version":     5,
				"created_at":  "2022-10-19T03:48:02.886Z",
				"updated_at":  "2022-10-19T03:48:02.886Z",
				"created_by":  "634f7332ccacb8e66b529f48",
				"appid":       "u3nnf8",
			}, map[string]interface{}{
				"_id":         "634f7374b151ccd9cd673f4b",
				"name":        "aliyun-sms-service",
				"code":        "\nimport cloud from '@/cloud-sdk'\n\n/**\n * @body phone string 手机号\n * @body code string | number 验证码\n */\nexports.main = async function (ctx) {\n  // 加载短信配置\n  const config = await loadAliSmsConfigs()\n\n  const phone = ctx.body?.phone\n  if (!phone) {\n    return { code: 'INVALID_PARAM', error: 'invalid phone' }\n  }\n  const code = ctx.body?.code\n  if (!code) {\n    return { code: 'INVALID_PARAM', error: 'invalid code' }\n  }\n\n  const params = {\n    AccessKeyId: config.accessKeyId,\n    AccessKeySecret: config.accessKeySecret,\n    ApiEntryPoint: config.api_entrypoint,\n    Action: 'SendSms',\n    Version: '2017-05-25',\n    PhoneNumbers: phone,\n    SignName: config.signName,\n    TemplateCode: config.templateCode,\n    TemplateParam: `{\"code\": ${code}}`\n  }\n\n  const data = await cloud.invoke('invoke-aliyun-api', { body: params })\n  console.log(data)\n\n  return {\n    code: 0,\n    data: data\n  }\n}\n\n\n\n/**\n * 加载阿里云短信配置\n */\nasync function loadAliSmsConfigs() {\n  const db = cloud.database()\n  const { data: config } = await db.collection('sys_config')\n    .where({ key: 'alisms' })\n    .getOne()\n\n  const value = config?.value\n\n  if (!value) {\n    throw new Error('加载短信配置失败，是否配置？')\n  }\n\n  return {\n    accessKeyId: value?.accessKeyId,          // 阿里云访问 Key ID\n    accessKeySecret: value?.accessKeySecret,  // 阿里云访问 Key Secret\n    api_entrypoint: value?.api_entrypoint ?? 'https://dysmsapi.aliyuncs.com',\n    signName: value?.signName,          // 短信签名，修改为你的签名，如: \"灼灼信息\"\n    templateCode: value?.templateCode   // 短信模板ID，如 'SMS_217850726'\n  }\n}",
				"label":       "Service: 短信服务",
				"hash":        "2f111560f9c5eb2dbca096eccd066ff7",
				"tags":        []string{"服务"},
				"description": "阿里云通信-发短信内部调用服务，不开启外网访问",
				"enableHTTP":  false,
				"status":      1,
				"triggers":    []interface{}{},
				"debugParams": "{\n  \"phone\": \"13184211245\",\n  \"code\": \"1234\"\n}",
				"version":     7,
				"created_at":  "2022-10-19T03:48:02.904Z",
				"updated_at":  "2022-10-19T03:48:02.904Z",
				"created_by":  "634f7332ccacb8e66b529f48",
				"appid":       "u3nnf8",
			}, map[string]interface{}{
				"_id":         "634f7374b151ccd9cd673f4c",
				"name":        "app-login-password",
				"code":        "import * as crypto from 'crypto'\nimport cloud from '@/cloud-sdk'\n\n/**\n * @body username string 用户名，即手机号\n * @body password string 密码\n */\n\nexports.main = async function (ctx: FunctionContext) {\n  const db = cloud.database()\n\n  // 参数验证\n  const { username, password } = ctx.body\n  if (!username || !password) {\n    return { code: 'INVALID_PARAM', error: '用户名或密码不可为空' }\n  }\n\n  // 验证用户名与密码是否正确\n  const { data: user } = await db.collection('biz_user')\n    .where({\n      username,\n      password: hashPassword(password)\n    })\n    .getOne()\n\n  if (!user)\n    return { code: 'INVALID_PARAM', error: '用户名或密码不正确' }\n\n  // 默认 token 有效期为 7 天\n  const expire = Math.floor(Date.now() / 1000) + 60 * 60 * 24 * 7\n  const access_token = cloud.getToken({\n    uid: user._id,\n    exp: expire\n  })\n\n  delete user['password']\n\n  return {\n    code: 0,\n    data: { access_token, user, expire }\n  }\n}\n\n/**\n * @param {string} content\n * @return {string}\n */\nfunction hashPassword(content: string): string {\n  return crypto\n    .createHash('sha256')\n    .update(content)\n    .digest('hex')\n}\n\n",
				"label":       "App: 用户名+密码登陆",
				"hash":        "0e24fa405b9fcaac4a57abc329f7dd68",
				"tags":        []string{"用户", "App"},
				"description": "App: 用户名+密码登陆",
				"enableHTTP":  true,
				"status":      1,
				"triggers":    []interface{}{},
				"debugParams": "{\"username\":\"18758887214\",\"password\":\"123456\"}",
				"version":     13,
				"created_at":  "2022-10-19T03:48:02.975Z",
				"updated_at":  "2022-10-19T03:48:02.975Z",
				"created_by":  "634f7332ccacb8e66b529f48",
				"appid":       "u3nnf8",
			}, map[string]interface{}{
				"_id":         "634f7374b151ccd9cd673f4d",
				"name":        "app-quick-login",
				"code":        "\nimport cloud from '@/cloud-sdk'\n\n/**\n * @body username string 用户名，即手机号\n * @body code number 验证码\n * \n * 1. 验证码检查\n * 2. 用户不存在则为注册，并登录\n * 3. 用户存在，则直接完成登录\n * 4. 使验证码失效\n */\n\nexports.main = async function (ctx: FunctionContext) {\n  const db = cloud.database()\n\n  // 参数验证\n  let { username, code } = ctx.body\n  if (!username || !code) {\n    return { code: 'INVALID_PARAM', error: '用户名或验证码不可为空' }\n  }\n\n  code = parseInt(code)\n  console.log(ctx.body)\n\n  // 验证码是否正确\n  const { total } = await db.collection('sys_sms_history')\n    .where({\n      phone: username,\n      code: code,\n      type: 'login',\n      status: 1,\n      created_at: db.command.gte(Date.now() - 10 * 60 * 1000)\n    })\n    .count()\n\n  console.log(total, 'total')\n  if (total === 0) {\n    return { code: 'INVALID_CODE', error: '无效的验证码' }\n  }\n\n  let { data: user } = await db.collection('users')\n    .where({ phone: username })\n    .getOne()\n\n  // 若用户不存在，则注册并完成登录\n  if (!user) {\n    const { id } = await db.collection('users')\n      .add({\n        nickname: username,\n        username,\n        phone: username,\n        created_at: new Date(),\n        updated_at: new Date()\n      })\n\n    const r = await db.collection('users').where({ _id: id }).getOne()\n    user = r.data\n  }\n\n  // 使验证码失效\n  await db.collection('sys_sms_history')\n    .where({\n      phone: username,\n      code,\n      type: 'login'\n    })\n    .update({\n      status: 0\n    })\n\n  delete user['password']\n\n  // 默认 token 有效期为 7 天\n  const expire = Math.floor(Date.now() / 1000) + 60 * 60 * 24 * 7\n  const access_token = cloud.getToken({\n    uid: user._id,\n    exp: expire,\n  })\n\n  return {\n    code: 0,\n    data: { access_token, user, expire }\n  }\n}",
				"label":       "App: 手机号+短信快速登陆",
				"hash":        "a7dfe0498d6e90849cc438bf49306376",
				"tags":        []string{"App", "用户"},
				"description": "App: 手机号+短信快速登陆，新用户自动注册并登陆，老用户直接登陆",
				"enableHTTP":  true,
				"status":      1,
				"triggers":    []interface{}{},
				"debugParams": "{\"username\":\"17700001111\",\"code\":\"5836\"}",
				"version":     3,
				"created_at":  "2022-10-19T03:48:02.995Z",
				"updated_at":  "2022-10-19T03:48:02.995Z",
				"created_by":  "634f7332ccacb8e66b529f48",
				"appid":       "u3nnf8",
			}, map[string]interface{}{
				"_id":         "634f7374b151ccd9cd673f4f",
				"name":        "init-app-rbac",
				"code":        "/**\n * 本函数可用于初始化一套 RBAC 必要的数据，通常不需要删除此云函数，也不要开启 HTTP 调用。\n */\nimport cloud from '@/cloud-sdk'\nimport * as assert from 'assert'\nimport * as crypto from 'crypto'\nconst db = cloud.database()\n\nexports.main = async function (ctx) {\n\n  // 创建 RBAC 初始权限\n  await createInitialPermissions()\n\n  // 创建 RBAC 初始角色\n  await createFirstRole()\n\n  // 创建初始管理员\n  await createFirstAdmin(\"admin\", \"123456\")\n\n  return 'ok'\n}\n\n/**\n * 预置 RBAC 权限\n */\nconst permissions = [\n  { name: 'role.create', label: '创建角色' },\n  { name: 'role.read', label: '读取角色' },\n  { name: 'role.edit', label: '编辑角色' },\n  { name: 'role.delete', label: '删除角色' },\n\n  { name: 'permission.create', label: '创建权限' },\n  { name: 'permission.read', label: '读取权限' },\n  { name: 'permission.edit', label: '编辑权限' },\n  { name: 'permission.delete', label: '删除权限' },\n\n  { name: 'admin.create', label: '创建管理员' },\n  { name: 'admin.read', label: '获取管理员' },\n  { name: 'admin.edit', label: '编辑管理员' },\n  { name: 'admin.delete', label: '删除管理员' },\n]\n\n\n\n// 创建初始管理员\nasync function createFirstAdmin(username: string, password: string) {\n  try {\n\n    const { total } = await db.collection('admins').count()\n    if (total > 0) {\n      console.log('admin already exists')\n      return\n    }\n\n    await cloud.mongo.db.collection('admins').createIndex('username', { unique: true })\n\n    const { data } = await db.collection('roles').get()\n    const roles = data.map(it => it.name)\n\n    const r_add = await db.collection('admins').add({\n      username,\n      avatar: \"https://static.dingtalk.com/media/lALPDe7szaMXyv3NAr3NApw_668_701.png\",\n      name: 'Admin',\n      roles,\n      created_at: Date.now(),\n      updated_at: Date.now()\n    })\n    assert.ok(r_add.id, 'add admin occurs error')\n\n    await db.collection('password').add({\n      uid: r_add.id,\n      password: hashPassword(password),\n      type: 'login',\n      created_at: Date.now(),\n      updated_at: Date.now()\n    })\n\n    return r_add.id\n  } catch (error) {\n    console.error(error.message)\n  }\n}\n\n// 创建初始角色\nasync function createFirstRole() {\n  try {\n    await cloud.mongo.db.collection('roles').createIndex('name', { unique: true })\n    const r_perm = await db.collection('permissions').get()\n    assert(r_perm.ok, 'get permissions failed')\n\n    const permissions = r_perm.data.map(it => it.name)\n    const r_add = await db.collection('roles').add({\n      name: 'superadmin',\n      label: '超级管理员',\n      description: '系统初始化的超级管理员',\n      permissions,\n      created_at: Date.now(),\n      updated_at: Date.now()\n    })\n\n    assert.ok(r_add.id, 'add role occurs error')\n\n    return r_add.id\n  } catch (error) {\n    if (error.code == 11000) {\n      return console.log('permissions already exists')\n    }\n\n    console.error(error.message)\n  }\n}\n\n// 创建初始权限\nasync function createInitialPermissions() {\n\n  // 创建唯一索引\n  await cloud.mongo.db.collection('permissions').createIndex('name', { unique: true })\n  for (const perm of permissions) {\n    try {\n      const data = {\n        ...perm,\n        created_at: Date.now(),\n        updated_at: Date.now()\n      }\n      await db.collection('permissions').add(data)\n    } catch (error) {\n      if (error.code == 11000) {\n        console.log('permissions already exists')\n        continue\n      }\n      console.error(error.message)\n    }\n  }\n\n  return true\n}\n\n\n/**\n * @param {string} content\n * @return {string}\n */\nfunction hashPassword(content: string): string {\n  return crypto\n    .createHash('sha256')\n    .update(content)\n    .digest('hex')\n}\n\n",
				"label":       "Init: 初始化应用 RBAC 数据",
				"hash":        "96e609119b61e808cfa55213231b1a76",
				"tags":        []string{"初始化"},
				"description": "初始化一个经典的 RBAC，包括基础的权限、角色、初始管理员",
				"enableHTTP":  false,
				"status":      1,
				"triggers":    []interface{}{},
				"debugParams": "{\"code\":\"laf\"}",
				"version":     2,
				"created_at":  "2022-10-19T03:48:03.177Z",
				"updated_at":  "2022-10-19T03:48:03.177Z",
				"created_by":  "634f7332ccacb8e66b529f48",
				"appid":       "u3nnf8",
			}, map[string]interface{}{
				"_id":         "634f7374b151ccd9cd673f50",
				"name":        "init-sys-region",
				"code":        "\nimport cloud from '@/cloud-sdk'\nimport { ObjectId } from 'mongodb'\nconst data_url = 'https://a5bec5b2-ebd0-4803-b29b-0a1766aa7edf_data.fs.lafyun.com/sys_region.json'\n\nexports.main = async function (ctx) {\n  \n  const db = cloud.mongo.db\n  await db.collection('sys_region').createIndex({ code: 1 }, { unique: true})\n\n  const data = await loadData()\n  for (const item of data) {\n    await insertItem(item, 1)\n    console.log(`completed: ${item.name}`)\n  }\n\n  return 'ok'\n}\n\nasync function loadData() {\n  const res = await cloud.fetch(data_url)\n  return res.data\n}\n\nasync function insertItem(item: any, level: number) {\n  const db = cloud.mongo.db \n  const _data = {\n    _id: new ObjectId().toHexString(),\n    code: item.id,\n    parent: item.parentId,\n    label: item.label,\n    level\n  }\n  try {\n      const res = await db.collection('sys_region')\n        .insertOne(_data)\n\n      console.log(`i.${item.label}`, res.insertedId)\n  } catch (error) {\n    if (error.code != 11000) {\n      throw error\n    }\n  }\n\n  if (!item.children?.length) {\n    return\n  }\n\n  // recursively insert children\n  for (const sub of item.children) {\n    await insertItem(sub, level + 1)\n  }\n}",
				"label":       "Init: 初始化系统行政区域",
				"hash":        "ce4c1886a9c6b756cf52338b8f21adfe",
				"tags":        []string{"初始化"},
				"description": "",
				"enableHTTP":  false,
				"status":      1,
				"triggers":    []interface{}{},
				"debugParams": "{\"code\":\"laf\"}",
				"version":     9,
				"created_at":  "2022-10-19T03:48:03.202Z",
				"updated_at":  "2022-10-19T03:48:03.202Z",
				"created_by":  "634f7332ccacb8e66b529f48",
				"appid":       "u3nnf8",
			},
		},
		"total": 4,
		"limit": 20,
		"page":  1,
	}

	return CommonResp(code, message, data)
}

func FunctionLogsInfo(c server.Context) interface{} {
	code := 0
	message := emptyString
	data := map[string]interface{}{
		"list": []interface{}{
			map[string]interface{}{
				"_id":        "638b492c33b3bd96cb937ee9",
				"requestId":  "trigger_613a347c1ce6a2f7b5d49c16",
				"method":     "trigger",
				"func_id":    "634f7374b151ccd9cd673f51",
				"func_name":  "initializer",
				"logs":       []string{"invoked by trigger: 监听应用启动就绪事件 (613a347c1ce6a2f7b5d49c16)", "[2022/12/03 13:03:40] - 初始化 RBAC", "[2022/12/03 13:03:40] - 初始化应用集合索引", "[2022/12/03 13:03:40] - 初始化 shared_utils", "[2022/12/03 13:03:40] - Error: self-signed certificate\n    at TLSSocket.onConnectSecure (node:_tls_wrap:1538:34)\n    at TLSSocket.emit (node:events:513:28)\n    at TLSSocket._finishInit (node:_tls_wrap:952:8)\n    at ssl.onhandshakedone (node:_tls_wrap:733:12) {\n  code: 'DEPTH_ZERO_SELF_SIGNED_CERT',\n  config: {\n    url: 'https://a5bec5b2-ebd0-4803-b29b-0a1766aa7edf_data.fs.lafyun.com/sys_region.json',\n    headers: {\n      Accept: 'application/json, text/plain, */*',\n      'User-Agent': 'axios/0.21.4'\n    },\n    transformRequest: [ [Function: transformRequest] ],\n    transformResponse: [ [Function: transformResponse] ],\n    timeout: 0,\n    adapter: [Function: httpAdapter],\n    xsrfCookieName: 'XSRF-TOKEN',\n    xsrfHeaderName: 'X-XSRF-TOKEN',\n    maxContentLength: -1,\n    maxBodyLength: -1,\n    validateStatus: [Function: validateStatus],\n    transitional: {\n      silentJSONParsing: true,\n      forcedJSONParsing: true,\n      clarifyTimeoutError: false\n    },\n    method: 'get',\n    data: undefined\n  },\n  request: <ref *1> Writable {\n    _writableState: WritableState {\n      objectMode: false,\n      highWaterMark: 16384,\n      finalCalled: false,\n      needDrain: false,\n      ending: false,\n      ended: false,\n      finished: false,\n      destroyed: false,\n      decodeStrings: true,\n      defaultEncoding: 'utf8',\n      length: 0,\n      writing: false,\n      corked: 0,\n      sync: true,\n      bufferProcessing: false,\n      onwrite: [Function: bound onwrite],\n      writecb: null,\n      writelen: 0,\n      afterWriteTickInfo: null,\n      buffered: [],\n      bufferedIndex: 0,\n      allBuffers: true,\n      allNoop: true,\n      pendingcb: 0,\n      constructed: true,\n      prefinished: false,\n      errorEmitted: false,\n      emitClose: true,\n      autoDestroy: true,\n      errored: null,\n      closed: false,\n      closeEmitted: false,\n      [Symbol(kOnFinished)]: []\n    },\n    _events: [Object: null prototype] {\n      response: [Function: handleResponse],\n      error: [Function: handleRequestError]\n    },\n    _eventsCount: 2,\n    _maxListeners: undefined,\n    _options: {\n      maxRedirects: 21,\n      maxBodyLength: 10485760,\n      protocol: 'https:',\n      path: '/sys_region.json',\n      method: 'GET',\n      headers: [Object],\n      agent: undefined,\n      agents: [Object],\n      auth: undefined,\n      hostname: 'a5bec5b2-ebd0-4803-b29b-0a1766aa7edf_data.fs.lafyun.com',\n      port: null,\n      nativeProtocols: [Object],\n      pathname: '/sys_region.json'\n    },\n    _ended: true,\n    _ending: true,\n    _redirectCount: 0,\n    _redirects: [],\n    _requestBodyLength: 0,\n    _requestBodyBuffers: [],\n    _onNativeResponse: [Function (anonymous)],\n    _currentRequest: ClientRequest {\n      _events: [Object: null prototype],\n      _eventsCount: 7,\n      _maxListeners: undefined,\n      outputData: [],\n      outputSize: 0,\n      writable: true,\n      destroyed: true,\n      _last: true,\n      chunkedEncoding: false,\n      shouldKeepAlive: false,\n      maxRequestsOnConnectionReached: false,\n      _defaultKeepAlive: true,\n      useChunkedEncodingByDefault: false,\n      sendDate: false,\n      _removedConnection: false,\n      _removedContLen: false,\n      _removedTE: false,\n      _contentLength: 0,\n      _hasBody: true,\n      _trailer: '',\n      finished: true,\n      _headerSent: true,\n      _closed: true,\n      socket: [TLSSocket],\n      _header: 'GET /sys_region.json HTTP/1.1\\r\\n' +\n        'Accept: application/json, text/plain, */*\\r\\n' +\n        'User-Agent: axios/0.21.4\\r\\n' +\n        'Host: a5bec5b2-ebd0-4803-b29b-0a1766aa7edf_data.fs.lafyun.com\\r\\n' +\n        'Connection: close\\r\\n' +\n        '\\r\\n',\n      _keepAliveTimeout: 0,\n      _onPendingData: [Function: nop],\n      agent: [Agent],\n      socketPath: undefined,\n      method: 'GET',\n      maxHeaderSize: undefined,\n      insecureHTTPParser: undefined,\n      path: '/sys_region.json',\n      _ended: false,\n      res: null,\n      aborted: false,\n      timeoutCb: null,\n      upgradeOrConnect: false,\n      parser: null,\n      maxHeadersCount: null,\n      reusedSocket: false,\n      host: 'a5bec5b2-ebd0-4803-b29b-0a1766aa7edf_data.fs.lafyun.com',\n      protocol: 'https:',\n      _redirectable: [Circular *1],\n      [Symbol(kCapture)]: false,\n      [Symbol(kNeedDrain)]: false,\n      [Symbol(corked)]: 0,\n      [Symbol(kOutHeaders)]: [Object: null prototype],\n      [Symbol(kUniqueHeaders)]: null\n    },\n    _currentUrl: 'https://a5bec5b2-ebd0-4803-b29b-0a1766aa7edf_data.fs.lafyun.com/sys_region.json',\n    [Symbol(kCapture)]: false\n  },\n  response: undefined,\n  isAxiosError: true,\n  toJSON: [Function: toJSON]\n}"},
				"time_usage": 546.456,
				"created_by": "trigger_613a347c1ce6a2f7b5d49c16",
				"trigger_id": "613a347c1ce6a2f7b5d49c16",
				"created_at": "2022-12-03T13:03:40.613Z",
			}, map[string]interface{}{
				"_id":        "638b492c33b3bd96cb937ee8",
				"requestId":  "invoke",
				"method":     "call",
				"func_id":    "634f7374b151ccd9cd673f50",
				"func_name":  "init-sys-region",
				"logs":       []string{"[2022/12/03 13:03:40] - self-signed certificate", "[2022/12/03 13:03:40] - Error: self-signed certificate\n    at TLSSocket.onConnectSecure (node:_tls_wrap:1538:34)\n    at TLSSocket.emit (node:events:513:28)\n    at TLSSocket._finishInit (node:_tls_wrap:952:8)\n    at ssl.onhandshakedone (node:_tls_wrap:733:12)"},
				"time_usage": 362.557,
				"data":       nil,
				"error":      "Error: self-signed certificate",
				"created_at": "2022-12-03T13:03:40.595Z",
			}, map[string]interface{}{
				"_id":        "638b492c33b3bd96cb937ee7",
				"requestId":  "invoke",
				"method":     "call",
				"func_id":    "634f7374b151ccd9cd673f4e",
				"func_name":  "init_shared_utils",
				"logs":       []string{},
				"time_usage": 1.641,
				"data":       "ok",
				"error":      nil,
				"created_at": "2022-12-03T13:03:40.213Z",
			}, map[string]interface{}{
				"_id":        "638b492c33b3bd96cb937ee6",
				"requestId":  "invoke",
				"method":     "call",
				"func_id":    "634f7374b151ccd9cd673f4f",
				"func_name":  "init-app-rbac",
				"logs":       []string{"[2022/12/03 13:03:40] - permissions already exists", "[2022/12/03 13:03:40] - permissions already exists", "[2022/12/03 13:03:40] - permissions already exists", "[2022/12/03 13:03:40] - permissions already exists", "[2022/12/03 13:03:40] - permissions already exists", "[2022/12/03 13:03:40] - permissions already exists", "[2022/12/03 13:03:40] - permissions already exists", "[2022/12/03 13:03:40] - permissions already exists", "[2022/12/03 13:03:40] - permissions already exists", "[2022/12/03 13:03:40] - permissions already exists", "[2022/12/03 13:03:40] - permissions already exists", "[2022/12/03 13:03:40] - permissions already exists", "[2022/12/03 13:03:40] - permissions already exists", "[2022/12/03 13:03:40] - admin already exists"},
				"time_usage": 101.056,
				"data":       "ok",
				"error":      nil,
				"created_at": "2022-12-03T13:03:40.176Z",
			},
		},
		"total": 4,
		"limit": 20,
		"page":  1,
	}

	return CommonResp(code, message, data)
}
