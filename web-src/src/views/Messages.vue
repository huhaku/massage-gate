<template>
  <div>
    <el-card shadow="never" class="page-card">
      <div style="display:flex;gap:10px;align-items:center;flex-wrap:wrap">
        <el-radio-group v-model="status" @change="load(1)">
          <el-radio-button value="all">全部</el-radio-button>
          <el-radio-button value="pending">待投递</el-radio-button>
          <el-radio-button value="queued">暂存重试</el-radio-button>
          <el-radio-button value="success">已送达</el-radio-button>
          <el-radio-button value="dead">无可用目标</el-radio-button>
          <el-radio-button value="unrouted">未命中路由</el-radio-button>
        </el-radio-group>
        <el-select v-model="sourceID" placeholder="全部来源" clearable style="width:160px" @change="load(1)">
          <el-option v-for="s in sources" :key="s.id" :label="s.name" :value="s.id" />
        </el-select>
        <el-input v-model="q" placeholder="搜索标题/内容" clearable style="width:220px" @keyup.enter="load(1)" @clear="load(1)">
          <template #append><el-button @click="load(1)"><el-icon><Search /></el-icon></el-button></template>
        </el-input>
        <el-button :loading="loading" @click="load()"><el-icon><Refresh /></el-icon>刷新</el-button>
        <span style="flex:1"></span>
        <span style="color:#909399;font-size:13px">共 {{ total }} 条 · {{ autoHint }}</span>
        <el-switch v-model="auto" active-text="自动刷新" />
      </div>
    </el-card>

    <el-card shadow="never">
      <el-table :data="list" size="small" v-loading="loading">
        <el-table-column prop="id" label="ID" width="64" />
        <el-table-column prop="title" label="标题" min-width="150" show-overflow-tooltip />
        <el-table-column prop="body" label="内容" min-width="240" show-overflow-tooltip />
        <el-table-column label="优先级" width="70">
          <template #default="{ row }">{{ row.priority }}</template>
        </el-table-column>
        <el-table-column label="状态" width="110">
          <template #default="{ row }">
            <el-tag size="small" :type="(STATUS[row.status] || STATUS.unrouted).type">
              {{ (STATUS[row.status] || STATUS.unrouted).label }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="投递" width="64">
          <template #default="{ row }">{{ row.delivery_total }}</template>
        </el-table-column>
        <el-table-column label="接收时间" width="165">
          <template #default="{ row }">{{ fmtTime(row.received_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="150" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="detail(row)">详情</el-button>
            <el-button link type="warning" @click="retry(row)">重试</el-button>
            <el-button link type="danger" @click="remove(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-pagination style="margin-top:12px;justify-content:flex-end" layout="prev, pager, next"
        :total="total" :page-size="size" :current-page="page" @current-change="load" />
    </el-card>

    <el-drawer v-model="drawer" title="消息详情" size="600px">
      <template v-if="cur.message">
        <el-descriptions :column="2" border size="small">
          <el-descriptions-item label="ID">{{ cur.message.id }}</el-descriptions-item>
          <el-descriptions-item label="接收时间">{{ fmtTime(cur.message.received_at) }}</el-descriptions-item>
          <el-descriptions-item label="标题" :span="2">{{ cur.message.title || '(空)' }}</el-descriptions-item>
          <el-descriptions-item label="内容" :span="2">
            <div style="white-space:pre-wrap">{{ cur.message.body || '(空)' }}</div>
          </el-descriptions-item>
          <el-descriptions-item label="优先级">{{ cur.message.priority }}</el-descriptions-item>
          <el-descriptions-item label="标签">{{ cur.message.tags || '—' }}</el-descriptions-item>
          <el-descriptions-item label="点击链接" :span="2">{{ cur.message.click_url || '—' }}</el-descriptions-item>
        </el-descriptions>

        <h4 style="margin:16px 0 8px">原始报文</h4>
        <pre class="mono" style="background:#f4f4f5;padding:10px;border-radius:6px;font-size:12px;overflow:auto;max-height:200px">{{ prettyRaw }}</pre>

        <h4 style="margin:16px 0 8px">投递记录</h4>
        <el-table :data="cur.deliveries" size="small" border>
          <el-table-column prop="route" label="路由" min-width="100" />
          <el-table-column label="状态" width="105">
            <template #default="{ row }">
              <el-tag size="small" :type="(STATUS[row.status] || STATUS.pending).type">
                {{ (STATUS[row.status] || STATUS.pending).label }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="attempts" label="次数" width="56" />
          <el-table-column prop="cycles" label="轮数" width="56" />
          <el-table-column label="送达时间" width="150">
            <template #default="{ row }">{{ row.sent_at ? fmtTime(row.sent_at) : '—' }}</template>
          </el-table-column>
          <el-table-column label="下次尝试" width="150">
            <template #default="{ row }">{{ row.status === 'queued' || row.status === 'pending' ? fmtTime(row.next_at) : '—' }}</template>
          </el-table-column>
          <el-table-column label="最后错误" min-width="160">
            <template #default="{ row }">
              <span class="mono" style="font-size:12px;color:#f56c6c">{{ row.last_error || '—' }}</span>
            </template>
          </el-table-column>
        </el-table>
      </template>
    </el-drawer>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { api, fmtTime, showError, STATUS } from '../api'
import { ElMessage, ElMessageBox } from 'element-plus'

const list = ref([])
const sources = ref([])
const total = ref(0)
const page = ref(1)
const size = 20
const status = ref('all')
const sourceID = ref(null)
const q = ref('')
const loading = ref(false)
const drawer = ref(false)
const cur = ref({})
const auto = ref(true)
let timer = null

const autoHint = computed(() => (auto.value ? '每 10s 自动刷新' : ''))

const prettyRaw = computed(() => {
  try { return JSON.stringify(JSON.parse(cur.value.message?.raw || '{}'), null, 2) } catch { return cur.value.message?.raw || '' }
})

async function load(p) {
  if (p) page.value = p
  loading.value = true
  try {
    const data = await api.get('/api/messages', {
      page: page.value, size, status: status.value,
      source_id: sourceID.value || undefined, q: q.value || undefined,
    })
    list.value = data.list
    total.value = data.total
  } catch (e) { showError(e) } finally { loading.value = false }
}

async function loadSources() {
  try { sources.value = await api.get('/api/sources') } catch { /* ignore */ }
}

onMounted(() => {
  load()
  loadSources()
  timer = setInterval(() => { if (auto.value && !drawer.value) load() }, 10000)
})
onUnmounted(() => clearInterval(timer))

async function detail(row) {
  try { cur.value = await api.get('/api/messages/' + row.id); drawer.value = true } catch (e) { showError(e) }
}

async function retry(row) {
  try {
    await api.post(`/api/messages/${row.id}/retry`)
    ElMessage.success('已重新入队投递')
    load()
  } catch (e) { showError(e) }
}

async function remove(row) {
  await ElMessageBox.confirm(`删除消息 #${row.id} 及其投递记录?`, '确认', { type: 'warning' })
  try { await api.del('/api/messages/' + row.id); load() } catch (e) { showError(e) }
}
</script>
