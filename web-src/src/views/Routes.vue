<template>
  <div>
    <el-card shadow="never" class="page-card">
      <div style="display:flex;justify-content:space-between;align-items:center">
        <span style="color:#909399;font-size:13px">
          路由 = 入站通道 → 出站目标(多对多)。主目标按序尝试,失败自动切换备用;主备全部失败则消息进入暂存队列退避重试,不丢消息
        </span>
        <el-button type="primary" @click="openEdit()"><el-icon><Plus /></el-icon>新增路由</el-button>
      </div>
    </el-card>

    <el-card shadow="never">
      <el-table :data="list" size="small">
        <el-table-column prop="id" label="ID" width="56" />
        <el-table-column prop="name" label="名称" min-width="140" />
        <el-table-column label="入站通道" width="140">
          <template #default="{ row }">{{ sourceName(row.source_id) }}</template>
        </el-table-column>
        <el-table-column label="目标链" min-width="260">
          <template #default="{ row }">
            <el-tag v-for="t in row.targets.filter(t => t.role === 'primary')" :key="t.id" size="small" class="mr4">
              主: {{ targetName(t.target_id) }}
            </el-tag>
            <el-tag v-for="t in row.targets.filter(t => t.role === 'backup')" :key="t.id" size="small" type="warning" class="mr4">
              备: {{ targetName(t.target_id) }}
            </el-tag>
            <span v-if="!row.targets.length" style="color:#f56c6c">⚠ 未配置目标</span>
          </template>
        </el-table-column>
        <el-table-column prop="filter_keyword" label="关键字过滤" width="110">
          <template #default="{ row }">{{ row.filter_keyword || '—' }}</template>
        </el-table-column>
        <el-table-column label="优先级" width="110">
          <template #default="{ row }">{{ priLabel(row) }}</template>
        </el-table-column>
        <el-table-column label="启用" width="80">
          <template #default="{ row }"><el-switch v-model="row.enabled" @change="quickSave(row)" /></template>
        </el-table-column>
        <el-table-column label="操作" width="130" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button link type="danger" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="dlg" :title="form.id ? '编辑路由' : '新增路由'" width="680px">
      <el-form label-width="110px">
        <el-form-item label="名称"><el-input v-model="form.name" /></el-form-item>
        <el-form-item label="入站通道">
          <el-select v-model="form.source_id" style="width:100%">
            <el-option v-for="s in sources" :key="s.id" :label="`${s.name} (${s.type})`" :value="s.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="出站目标">
          <div style="width:100%">
            <div v-for="(t, i) in form.targets" :key="i" style="display:flex;gap:8px;margin-bottom:8px">
              <el-select v-model="t.target_id" style="flex:1">
                <el-option v-for="tg in targets" :key="tg.id" :label="`${tg.name} (${TARGET_TYPES[tg.type]})`" :value="tg.id" />
              </el-select>
              <el-select v-model="t.role" style="width:110px">
                <el-option label="主目标" value="primary" />
                <el-option label="备用目标" value="backup" />
              </el-select>
              <el-button :disabled="i === 0" @click="move(i, -1)"><el-icon><Top /></el-icon></el-button>
              <el-button :disabled="i === form.targets.length - 1" @click="move(i, 1)"><el-icon><Bottom /></el-icon></el-button>
              <el-button type="danger" plain @click="form.targets.splice(i, 1)"><el-icon><Delete /></el-icon></el-button>
            </div>
            <el-button size="small" @click="form.targets.push({ target_id: null, role: 'primary', sort: form.targets.length })">
              <el-icon><Plus /></el-icon>添加目标(先主后备)
            </el-button>
          </div>
        </el-form-item>
        <el-form-item label="关键字过滤">
          <el-input v-model="form.filter_keyword" placeholder="留空 = 不过滤;填写后标题或正文包含该关键字才转发" />
        </el-form-item>
        <el-form-item label="优先级">
          <el-select v-model="form.priority_mode" style="width:170px">
            <el-option label="透传" value="passthrough" />
            <el-option label="固定值" value="fixed" />
            <el-option label="映射表" value="map" />
          </el-select>
          <el-input-number v-if="form.priority_mode === 'fixed'" v-model="form.priority_value" style="margin-left:8px" :min="-5" :max="10" />
          <el-input v-if="form.priority_mode === 'map'" v-model="priMapText" style="margin-left:8px;flex:1"
            placeholder="源:目标,逗号分隔,如 5:3,4:2,9:1" class="mono" />
        </el-form-item>
        <el-form-item label="标题模板">
          <el-input v-model="form.title_tpl" placeholder="留空 = 原样;可用 {{title}} {{body}} {{priority}} {{source}} {{time}}" class="mono" />
        </el-form-item>
        <el-form-item label="内容模板">
          <el-input v-model="form.body_tpl" type="textarea" :rows="3" placeholder="留空 = 原样;可用 {{title}} {{body}} {{priority}} {{source}} {{time}}" class="mono" />
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
import { ref, computed, onMounted } from 'vue'
import { api, showError, TARGET_TYPES } from '../api'
import { ElMessage, ElMessageBox } from 'element-plus'

const list = ref([])
const sources = ref([])
const targets = ref([])
const dlg = ref(false)
const busy = ref(false)
const priMapText = ref('')

const emptyForm = () => ({
  id: 0, name: '', source_id: null, enabled: true, filter_keyword: '',
  priority_mode: 'passthrough', priority_value: 0, priority_map: '', title_tpl: '', body_tpl: '',
  targets: [],
})
const form = ref(emptyForm())

const sourceName = (id) => sources.value.find((s) => s.id === id)?.name || '#' + id
const targetName = (id) => targets.value.find((t) => t.id === id)?.name || '#' + id

const priLabel = (row) => ({
  passthrough: '透传',
  fixed: `固定 ${row.priority_value}`,
  map: '映射表',
}[row.priority_mode] || '透传')

async function load() {
  try {
    list.value = await api.get('/api/routes')
    sources.value = await api.get('/api/sources')
    targets.value = await api.get('/api/targets')
  } catch (e) { showError(e) }
}
onMounted(load)

function openEdit(row) {
  if (row) {
    form.value = { ...emptyForm(), ...row, targets: (row.targets || []).map((t) => ({ ...t })) }
    priMapText.value = Object.entries(safeParse(row.priority_map)).map(([k, v]) => `${k}:${v}`).join(',')
  } else {
    form.value = emptyForm()
    priMapText.value = ''
  }
  dlg.value = true
}

function safeParse(s) { try { return JSON.parse(s || '{}') } catch { return {} } }

function move(i, d) {
  const arr = form.value.targets
  const j = i + d
  if (j < 0 || j >= arr.length) return
  ;[arr[i], arr[j]] = [arr[j], arr[i]]
}

async function quickSave(row) {
  // 行内启用开关:取全量数据再保存
  await api.put('/api/routes/' + row.id, { ...row })
  ElMessage.success('已更新')
}

async function save() {
  busy.value = true
  try {
    const payload = { ...form.value }
    payload.targets = (form.value.targets || [])
      .filter((t) => t.target_id)
      .map((t, i) => ({ target_id: t.target_id, role: t.role, sort: i }))
    if (form.value.priority_mode === 'map') {
      const m = {}
      for (const pair of priMapText.value.split(',')) {
        const [k, v] = pair.split(':').map((x) => x.trim())
        if (k !== '' && v !== '' && !isNaN(+v)) m[k] = +v
      }
      payload.priority_map = JSON.stringify(m)
    }
    if (payload.id) await api.put('/api/routes/' + payload.id, payload)
    else await api.post('/api/routes', payload)
    ElMessage.success('已保存')
    dlg.value = false
    load()
  } catch (e) { showError(e) } finally { busy.value = false }
}

async function remove(row) {
  await ElMessageBox.confirm(`删除路由「${row.name}」?`, '确认', { type: 'warning' })
  try { await api.del('/api/routes/' + row.id); load() } catch (e) { showError(e) }
}
</script>

<style scoped>
.mr4 { margin-right: 4px; }
</style>
