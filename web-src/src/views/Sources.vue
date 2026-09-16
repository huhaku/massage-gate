<template>
  <div>
    <el-card shadow="never" class="page-card">
      <div style="display:flex;justify-content:space-between;align-items:center">
        <span style="color:#909399;font-size:13px">
          入站通道对外暴露 <span class="mono">{{ origin }}/i/{通道标识}</span>,各协议客户端把推送地址指向这里即可
        </span>
        <el-button type="primary" @click="openEdit()"><el-icon><Plus /></el-icon>新增入站通道</el-button>
      </div>
    </el-card>

    <el-card shadow="never">
      <el-table :data="list" size="small">
        <el-table-column prop="id" label="ID" width="56" />
        <el-table-column prop="name" label="名称" min-width="130" />
        <el-table-column label="类型" width="120">
          <template #default="{ row }"><el-tag size="small">{{ SOURCE_TYPES[row.type] }}</el-tag></template>
        </el-table-column>
        <el-table-column prop="code" label="标识" width="120" class-name="mono" />
        <el-table-column label="接入地址" min-width="300">
          <template #default="{ row }">
            <span class="mono" style="font-size:12px">{{ origin }}/i/{{ row.code }}</span>
            <el-button link size="small" @click="copyUrl(row)">复制</el-button>
          </template>
        </el-table-column>
        <el-table-column label="鉴权 Token" width="100">
          <template #default="{ row }">{{ parseCfg(row.config).token ? '✔' : '—' }}</template>
        </el-table-column>
        <el-table-column label="启用" width="80">
          <template #default="{ row }">
            <el-switch v-model="row.enabled" @change="toggle(row)" />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="130" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button link type="danger" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="dlg" :title="form.id ? '编辑入站通道' : '新增入站通道'" width="560px">
      <el-form label-width="110px">
        <el-form-item label="名称"><el-input v-model="form.name" /></el-form-item>
        <el-form-item label="类型">
          <el-radio-group v-model="form.type">
            <el-radio-button v-for="(label, k) in SOURCE_TYPES" :key="k" :value="k">{{ label }}</el-radio-button>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="通道标识">
          <el-input v-model="form.code" placeholder="小写字母/数字/连字符,如 my-gotify" class="mono" />
        </el-form-item>
        <el-form-item label="鉴权 Token">
          <el-input v-model="form.token" placeholder="留空 = 不鉴权;客户端以 ?token= 或 X-Token 头携带" class="mono" />
        </el-form-item>
        <el-form-item v-if="form.type === 'webhook'" label="标题字段">
          <el-input v-model="form.title_field" placeholder="默认 title,支持 a.b 点路径,逗号分隔多个候选" />
        </el-form-item>
        <el-form-item v-if="form.type === 'webhook'" label="内容字段">
          <el-input v-model="form.body_field" placeholder="默认 message,body,content,text" />
        </el-form-item>
        <template v-if="form.type === 'telegram'">
          <el-form-item label="Bot Token">
            <el-input v-model="form.bot_token" placeholder="可选,用于校验消息来源" class="mono" />
          </el-form-item>
          <el-alert :closable="false" type="info" title="设置 Telegram Bot Webhook 指向此地址即可接收消息" style="margin-bottom:12px" />
        </template>
        <template v-if="form.type === 'wecom'">
          <el-form-item label="Token">
            <el-input v-model="form.token" placeholder="企业微信应用配置的 Token(用于消息解密)" class="mono" />
          </el-form-item>
          <el-form-item label="EncodingAESKey">
            <el-input v-model="form.encoding_aes_key" placeholder="消息加密密钥(可选)" class="mono" />
          </el-form-item>
          <el-alert :closable="false" type="info" title="在企业微信后台配置回调 URL 指向此地址" style="margin-bottom:12px" />
        </template>
        <template v-if="form.type === 'dingtalk'">
          <el-form-item label="Token">
            <el-input v-model="form.token" placeholder="机器人 Outgoing 配置的 Token(可选)" class="mono" />
          </el-form-item>
          <el-form-item label="加签密钥">
            <el-input v-model="form.secret" placeholder="安全设置选『加签』时填写(可选)" class="mono" />
          </el-form-item>
          <el-alert :closable="false" type="info" title="在钉钉群机器人中开启 Outgoing 机制,POST 地址填此 URL" style="margin-bottom:12px" />
        </template>
        <el-alert :closable="false" type="info" :title="hint" style="margin-top:4px" />
      </el-form>
      <template #footer>
        <el-button @click="dlg = false">取消</el-button>
        <el-button type="primary" :loading="busy" @click="save">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { api, showError, SOURCE_TYPES } from '../api'
import { ElMessage, ElMessageBox } from 'element-plus'

const origin = window.location.origin
const list = ref([])
const dlg = ref(false)
const busy = ref(false)
const emptyForm = () => ({ id: 0, name: '', code: '', type: 'webhook', enabled: true, token: '', title_field: '', body_field: '', bot_token: '', encoding_aes_key: '', secret: '' })
const form = ref(emptyForm())

const hint = computed(() => ({
  gotify: 'Gotify 客户端服务器地址填 ' + origin + '/i/' + (form.value.code || '{code}') + ',发送路径 /i/{code}/message?token=xxx',
  ntfy: 'ntfy 客户端 / curl 推送到 ' + origin + '/i/' + (form.value.code || '{code}') + '/{topic},支持 JSON 与 Header 两种发布格式',
  bark: 'Bark 客户端服务器填 ' + origin + '/i/' + (form.value.code || '{code}') + ',支持 GET 路径式与 POST JSON',
  webhook: '调用方 POST 任意 JSON 到 ' + origin + '/i/' + (form.value.code || '{code}') + ',按字段配置提取标题/内容',
  telegram: '设置 Telegram Bot Webhook: https://api.telegram.org/bot{TOKEN}/setWebhook?url=' + origin + '/i/' + (form.value.code || '{code}'),
  wecom: '企业微信应用回调地址: ' + origin + '/i/' + (form.value.code || '{code}'),
  dingtalk: '钉钉群机器人 Outgoing POST 地址: ' + origin + '/i/' + (form.value.code || '{code}'),
}[form.type]))

const parseCfg = (s) => { try { return JSON.parse(s || '{}') } catch { return {} } }

async function load() {
  try { list.value = await api.get('/api/sources') } catch (e) { showError(e) }
}
onMounted(load)

function openEdit(row) {
  if (row) {
    const cfg = parseCfg(row.config)
    form.value = { ...row, token: cfg.token || '', title_field: cfg.title_field || '', body_field: cfg.body_field || '', bot_token: cfg.bot_token || '', encoding_aes_key: cfg.encoding_aes_key || '', secret: cfg.secret || '' }
  } else {
    form.value = emptyForm()
  }
  dlg.value = true
}

function buildConfig() {
  const cfg = {}
  if (form.value.token) cfg.token = form.value.token
  if (form.value.type === 'webhook') {
    if (form.value.title_field) cfg.title_field = form.value.title_field
    if (form.value.body_field) cfg.body_field = form.value.body_field
  }
  if (form.value.type === 'telegram') {
    if (form.value.bot_token) cfg.bot_token = form.value.bot_token
  }
  if (form.value.type === 'wecom') {
    if (form.value.token) cfg.token = form.value.token
    if (form.value.encoding_aes_key) cfg.encoding_aes_key = form.value.encoding_aes_key
  }
  if (form.value.type === 'dingtalk') {
    if (form.value.token) cfg.token = form.value.token
    if (form.value.secret) cfg.secret = form.value.secret
  }
  return JSON.stringify(cfg)
}

async function toggle(row) {
  try { await api.put('/api/sources/' + row.id, row); ElMessage.success('已更新') }
  catch (e) { showError(e); load() }
}

async function save() {
  busy.value = true
  try {
    const payload = { ...form.value, config: buildConfig() }
    delete payload.token
    if (payload.id) await api.put('/api/sources/' + payload.id, payload)
    else await api.post('/api/sources', payload)
    ElMessage.success('已保存')
    dlg.value = false
    load()
  } catch (e) { showError(e) } finally { busy.value = false }
}

async function remove(row) {
  await ElMessageBox.confirm(`删除入站通道「${row.name}」?相关路由不会自动删除。`, '确认', { type: 'warning' })
  try { await api.del('/api/sources/' + row.id); load() } catch (e) { showError(e) }
}

function copyUrl(row) {
  navigator.clipboard.writeText(origin + '/i/' + row.code)
  ElMessage.success('已复制')
}
</script>
