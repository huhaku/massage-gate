<template>
  <div>
    <el-row :gutter="12">
      <el-col :span="5" v-for="c in cards" :key="c.label">
        <el-card shadow="never">
          <div style="color:#909399;font-size:13px">{{ c.label }}</div>
          <div style="font-size:26px;font-weight:600;margin-top:6px;color:c.color">{{ c.value }}</div>
        </el-card>
      </el-col>
      <el-col :span="4">
        <el-card shadow="never">
          <div style="color:#909399;font-size:13px">启用通道(入/出)</div>
          <div style="font-size:26px;font-weight:600;margin-top:6px">{{ stat.sources_enabled }} / {{ stat.targets_enabled }}</div>
        </el-card>
      </el-col>
    </el-row>

    <el-card shadow="never" style="margin-top:14px" class="page-card">
      <template #header>最近消息</template>
      <el-table :data="stat.recent || []" size="small">
        <el-table-column prop="id" label="ID" width="64" />
        <el-table-column prop="title" label="标题" min-width="160" show-overflow-tooltip />
        <el-table-column prop="body" label="内容" min-width="260" show-overflow-tooltip />
        <el-table-column label="接收时间" width="170">
          <template #default="{ row }">{{ fmtTime(row.received_at) }}</template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-card shadow="never">
      <template #header>最近投递错误</template>
      <el-empty v-if="!(stat.recent_errors || []).length" description="暂无错误" :image-size="60" />
      <ul v-else style="margin:0;padding-left:18px;color:#f56c6c;font-size:13px;line-height:1.9">
        <li v-for="(e, i) in stat.recent_errors" :key="i" class="mono">{{ e }}</li>
      </ul>
    </el-card>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { api, fmtTime, showError } from '../api'

const stat = ref({})
const cards = computed(() => [
  { label: '累计接收消息', value: stat.value.messages_total ?? '-' },
  { label: '24h 接收', value: stat.value.messages_24h ?? '-' },
  { label: '已送达', value: stat.value.del_success ?? '-' },
  { label: '待投递', value: stat.value.del_pending ?? '-' },
  { label: '暂存重试中', value: stat.value.del_queued ?? '-' },
  { label: '无可用目标', value: stat.value.del_dead ?? '-' },
])

async function load() {
  try { stat.value = await api.get('/api/dashboard') } catch (e) { showError(e) }
}
onMounted(load)
</script>
