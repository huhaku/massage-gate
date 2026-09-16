<template>
  <el-row :gutter="16">
    <el-col :span="12">
      <el-card shadow="never" class="page-card">
        <template #header>全局参数</template>
        <el-form label-width="150px">
          <el-form-item label="日志保留天数">
            <el-input-number v-model="form.keep_days" :min="0" :max="3650" />
            <div class="tip">0 = 永久保留(每小时清理一次过期消息)</div>
          </el-form-item>
          <el-form-item label="默认投递超时(ms)">
            <el-input-number v-model="form.timeout_default_ms" :min="1000" :step="1000" />
            <div class="tip">出站通道未单独配置超时时使用</div>
          </el-form-item>
          <el-form-item label="退避起始(秒)">
            <el-input-number v-model="form.backoff_base_sec" :min="5" :max="3600" />
            <div class="tip">主备全部失败进入暂存后的首次重试间隔</div>
          </el-form-item>
          <el-form-item label="退避上限(秒)">
            <el-input-number v-model="form.backoff_max_sec" :min="60" :max="86400" />
            <div class="tip">指数退避(起始×2ⁿ)的最大间隔</div>
          </el-form-item>
          <el-form-item>
            <el-button type="primary" :loading="busy" @click="save">保存设置</el-button>
          </el-form-item>
        </el-form>
      </el-card>
    </el-col>
    <el-col :span="12">
      <el-card shadow="never" class="page-card">
        <template #header>修改密码</template>
        <el-form label-width="100px">
          <el-form-item label="原密码">
            <el-input v-model="pwd.old_password" type="password" show-password />
          </el-form-item>
          <el-form-item label="新密码">
            <el-input v-model="pwd.new_password" type="password" show-password placeholder="至少 6 位" />
          </el-form-item>
          <el-form-item label="确认新密码">
            <el-input v-model="pwd.confirm" type="password" show-password />
          </el-form-item>
          <el-form-item>
            <el-button type="primary" :loading="busyPwd" @click="changePwd">修改密码</el-button>
          </el-form-item>
        </el-form>
      </el-card>
    </el-col>
  </el-row>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { api, showError } from '../api'
import { ElMessage } from 'element-plus'

const form = ref({ keep_days: 30, timeout_default_ms: 10000, backoff_base_sec: 60, backoff_max_sec: 3600 })
const pwd = ref({ old_password: '', new_password: '', confirm: '' })
const busy = ref(false)
const busyPwd = ref(false)

onMounted(async () => {
  try {
    const s = await api.get('/api/settings')
    for (const k of Object.keys(form.value)) form.value[k] = parseInt(s[k]) || form.value[k]
  } catch (e) { showError(e) }
})

async function save() {
  busy.value = true
  try {
    const payload = {}
    for (const k of Object.keys(form.value)) payload[k] = String(form.value[k])
    await api.put('/api/settings', payload)
    ElMessage.success('已保存')
  } catch (e) { showError(e) } finally { busy.value = false }
}

async function changePwd() {
  if (pwd.value.new_password !== pwd.value.confirm) {
    ElMessage.warning('两次输入的新密码不一致'); return
  }
  busyPwd.value = true
  try {
    await api.put('/api/settings/password', pwd.value)
    ElMessage.success('密码已修改')
    pwd.value = { old_password: '', new_password: '', confirm: '' }
  } catch (e) { showError(e) } finally { busyPwd.value = false }
}
</script>

<style scoped>
.tip { color: #909399; font-size: 12px; line-height: 1.4; margin-top: 2px; }
</style>
