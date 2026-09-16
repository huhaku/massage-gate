<template>
  <div>
    <el-card shadow="never" class="page-card">
      <div style="display:flex;justify-content:space-between;align-items:center">
        <span style="color:#909399;font-size:13px">出站通道是消息的推送目的地,可配置超时与重试次数;路由中将主目标失败后自动切换备用目标</span>
        <el-button type="primary" @click="openEdit()"><el-icon><Plus /></el-icon>新增出站通道</el-button>
      </div>
    </el-card>

    <el-card shadow="never">
      <el-table :data="list" size="small">
        <el-table-column prop="id" label="ID" width="56" />
        <el-table-column prop="name" label="名称" min-width="130" />
        <el-table-column label="类型" width="150">
          <template #default="{ row }"><el-tag size="small">{{ TARGET_TYPES[row.type] }}</el-tag></template>
        </el-table-column>
        <el-table-column label="超时" width="80">
          <template #default="{ row }">{{ row.timeout_ms || '默认' }}</template>
        </el-table-column>
        <el-table-column label="重试次数" width="90">
          <template #default="{ row }">{{ row.max_retry || 1 }}</template>
        </el-table-column>
        <el-table-column label="启用" width="80">
          <template #default="{ row }"><el-switch v-model="row.enabled" @change="toggle(row)" /></template>
        </el-table-column>
        <el-table-column label="操作" width="160" fixed="right">
          <template #default="{ row }">
            <el-button link type="success" :loading="testing === row.id" @click="test(row)">测试</el-button>
            <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button link type="danger" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="dlg" :title="form.id ? '编辑出站通道' : '新增出站通道'" width="600px">
      <el-form label-width="120px">
        <el-form-item label="名称"><el-input v-model="form.name" /></el-form-item>
        <el-form-item label="类型">
          <el-select v-model="form.type" style="width:100%" @change="onTypeChange">
            <el-option v-for="(label, k) in TARGET_TYPES" :key="k" :label="label" :value="k" />
          </el-select>
        </el-form-item>

        <template v-if="form.type === 'gotify'">
          <el-form-item label="服务器地址"><el-input v-model="f.url" placeholder="https://push.example.com" class="mono" /></el-form-item>
          <el-form-item label="应用 Token"><el-input v-model="f.token" class="mono" /></el-form-item>
        </template>
        <template v-else-if="form.type === 'ntfy'">
          <el-form-item label="服务器地址"><el-input v-model="f.url" placeholder="https://ntfy.sh" class="mono" /></el-form-item>
          <el-form-item label="主题 Topic"><el-input v-model="f.topic" class="mono" /></el-form-item>
          <el-form-item label="Token / 用户名">
            <el-input v-model="f.token" placeholder="访问令牌(可选)" class="mono" />
          </el-form-item>
          <el-form-item v-if="!f.token" label="用户名/密码">
            <div style="display:flex;gap:8px;width:100%">
              <el-input v-model="f.username" placeholder="可选" class="mono" />
              <el-input v-model="f.password" type="password" show-password placeholder="可选" class="mono" />
            </div>
          </el-form-item>
        </template>
        <template v-else-if="form.type === 'bark'">
          <el-form-item label="服务器地址"><el-input v-model="f.url" placeholder="https://api.day.app" class="mono" /></el-form-item>
          <el-form-item label="Device Key"><el-input v-model="f.device_key" class="mono" /></el-form-item>
          <el-form-item label="铃声 / 分组">
            <div style="display:flex;gap:8px;width:100%">
              <el-input v-model="f.sound" placeholder="可选,如 bell" />
              <el-input v-model="f.group" placeholder="可选" />
            </div>
          </el-form-item>
        </template>
        <template v-else-if="form.type === 'telegram'">
          <el-form-item label="Bot Token"><el-input v-model="f.bot_token" placeholder="123456:ABC-..." class="mono" /></el-form-item>
          <el-form-item label="Chat ID"><el-input v-model="f.chat_id" class="mono" /></el-form-item>
        </template>
        <template v-else-if="form.type === 'wecom'">
          <el-form-item label="Webhook 地址">
            <el-input v-model="f.url" placeholder="https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=xxx" class="mono" />
          </el-form-item>
          <el-form-item label="消息格式">
            <el-radio-group v-model="f.msg_type">
              <el-radio value="text">文本</el-radio>
              <el-radio value="markdown">Markdown</el-radio>
            </el-radio-group>
          </el-form-item>
        </template>
        <template v-else-if="form.type === 'feishu'">
          <el-form-item label="Webhook 地址">
            <el-input v-model="f.url" placeholder="https://open.feishu.cn/open-apis/bot/v2/hook/xxx" class="mono" />
          </el-form-item>
          <el-form-item label="签名密钥">
            <el-input v-model="f.secret" placeholder="机器人开启『签名校验』时填写(可选)" class="mono" />
          </el-form-item>
        </template>
        <template v-else-if="form.type === 'dingtalk'">
          <el-form-item label="Webhook 地址">
            <el-input v-model="f.url" placeholder="https://oapi.dingtalk.com/robot/send?access_token=xxx" class="mono" />
          </el-form-item>
          <el-form-item label="加签密钥">
            <el-input v-model="f.secret" placeholder="安全设置选『加签』时填写(可选)" class="mono" />
          </el-form-item>
          <el-form-item label="消息格式">
            <el-radio-group v-model="f.msg_type">
              <el-radio value="text">文本</el-radio>
              <el-radio value="markdown">Markdown</el-radio>
            </el-radio-group>
          </el-form-item>
        </template>
        <template v-else-if="form.type === 'webhook'">
          <el-form-item label="请求地址"><el-input v-model="f.url" class="mono" /></el-form-item>
          <el-form-item label="方法">
            <el-select v-model="f.method" style="width:120px"><el-option v-for="m in ['POST','PUT','PATCH']" :key="m" :label="m" :value="m" /></el-select>
          </el-form-item>
          <el-form-item label="请求头 JSON">
            <el-input v-model="f.headers" type="textarea" :rows="3" placeholder='{"Authorization":"Bearer xxx"}' class="mono" />
          </el-form-item>
          <el-alert :closable="false" type="info" title="未填模板时发送标准 JSON:{title, body, priority, tags, time, click}" />
        </template>
        <template v-else-if="form.type === 'custom'">
          <el-form-item label="请求地址"><el-input v-model="f.url" class="mono" /></el-form-item>
          <el-form-item label="方法">
            <el-select v-model="f.method" style="width:120px"><el-option v-for="m in ['POST','PUT','PATCH','GET']" :key="m" :label="m" :value="m" /></el-select>
          </el-form-item>
          <el-form-item label="请求头 JSON">
            <el-input v-model="f.headers" type="textarea" :rows="3" placeholder='{"Authorization":"Bearer xxx"}' class="mono" />
          </el-form-item>
          <el-form-item label="Body 模板">
            <el-input v-model="f.body_template" type="textarea" :rows="5" class="mono"
              placeholder='{"msgtype":"text","text":{"content":"{{title}}\n{{body}}"}}' />
          </el-form-item>
          <el-alert :closable="false" type="info"
            title="占位符:{{title}} {{body}} {{priority}} {{tags}} {{time}},JSON 字符串内用 {{title_json}} {{body_json}}(自动转义)" />
        </template>

        <el-divider />
        <el-form-item label="超时(ms)">
          <el-input-number v-model="form.timeout_ms" :min="0" :step="1000" />
          <span style="color:#909399;font-size:12px;margin-left:8px">0 = 全局默认</span>
        </el-form-item>
        <el-form-item label="重试次数">
          <el-input-number v-model="form.max_retry" :min="1" :max="10" />
          <span style="color:#909399;font-size:12px;margin-left:8px">单轮投递内该目标最多尝试次数,之后切换备用</span>
        </el-form-item>
        <el-form-item label="启用"><el-switch v-model="form.enabled" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dlg = false">取消</el-button>
        <el-button type="primary" :loading="busy" @click="save">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { api, showError, TARGET_TYPES } from '../api'
import { ElMessage, ElMessageBox } from 'element-plus'

const list = ref([])
const dlg = ref(false)
const busy = ref(false)
const testing = ref(0)

const emptyForm = () => ({
  id: 0, name: '', type: 'webhook', enabled: true, timeout_ms: 0, max_retry: 3,
})
const form = ref(emptyForm())
const f = reactive({ url: '', token: '', topic: '', username: '', password: '', device_key: '', sound: '', group: '', bot_token: '', chat_id: '', method: 'POST', headers: '', body_template: '' })

const CFG_FIELDS = {
  gotify: ['url', 'token'],
  ntfy: ['url', 'topic', 'token', 'username', 'password'],
  bark: ['url', 'device_key', 'sound', 'group'],
  telegram: ['bot_token', 'chat_id'],
  wecom: ['url', 'msg_type'],
  feishu: ['url', 'secret'],
  dingtalk: ['url', 'secret', 'msg_type'],
  webhook: ['url', 'method', 'headers'],
  custom: ['url', 'method', 'headers', 'body_template'],
}

async function load() {
  try { list.value = await api.get('/api/targets') } catch (e) { showError(e) }
}
onMounted(load)

function onTypeChange() {
  for (const k of Object.keys(f)) f[k] = k === 'method' ? 'POST' : k === 'msg_type' ? 'text' : ''
}

function openEdit(row) {
  onTypeChange()
  if (row) {
    form.value = { ...row }
    const cfg = JSON.parse(row.config || '{}')
    for (const k of Object.keys(f)) if (cfg[k] !== undefined) f[k] = typeof cfg[k] === 'object' ? JSON.stringify(cfg[k]) : cfg[k]
  } else {
    form.value = emptyForm()
  }
  dlg.value = true
}

async function toggle(row) {
  try { await api.put('/api/targets/' + row.id, row); ElMessage.success('已更新') }
  catch (e) { showError(e); load() }
}

async function save() {
  busy.value = true
  try {
    const cfg = {}
    for (const k of CFG_FIELDS[form.value.type] || []) {
      if (f[k] !== '' && f[k] !== undefined) cfg[k] = f[k]
    }
    const payload = { ...form.value, config: JSON.stringify(cfg) }
    if (form.value.id) await api.put('/api/targets/' + form.value.id, payload)
    else await api.post('/api/targets', payload)
    ElMessage.success('已保存')
    dlg.value = false
    load()
  } catch (e) { showError(e) } finally { busy.value = false }
}

async function test(row) {
  testing.value = row.id
  try {
    await api.post(`/api/targets/${row.id}/test`)
    ElMessage.success('测试消息已发送成功')
  } catch (e) { showError(e) } finally { testing.value = 0 }
}

async function remove(row) {
  await ElMessageBox.confirm(`删除出站通道「${row.name}」?将同时从所有路由中移除。`, '确认', { type: 'warning' })
  try { await api.del('/api/targets/' + row.id); load() } catch (e) { showError(e) }
}
</script>
