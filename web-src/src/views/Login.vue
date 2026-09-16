<template>
  <div style="height:100vh;display:flex;align-items:center;justify-content:center;background:#1d2935">
    <el-card style="width:380px">
      <h3 style="margin:4px 0 16px;text-align:center">📨 massage-gate</h3>
      <el-form label-position="top" @keyup.enter="submit">
        <el-form-item label="用户名">
          <el-input v-model="form.username" autofocus />
        </el-form-item>
        <el-form-item :label="initialized ? '密码' : '设置密码(至少 6 位)'">
          <el-input v-model="form.password" type="password" show-password />
        </el-form-item>
        <el-form-item v-if="!initialized" label="确认密码">
          <el-input v-model="form.confirm" type="password" show-password />
        </el-form-item>
        <el-button type="primary" style="width:100%" :loading="busy" @click="submit">
          {{ initialized ? '登录' : '初始化管理员账号' }}
        </el-button>
        <div v-if="initialized" style="margin-top:12px;text-align:center">
          <el-popover placement="top" :width="320" trigger="click">
            <template #reference>
              <el-link type="primary" :underline="false">忘记密码?</el-link>
            </template>
            <div style="font-size:13px;line-height:1.6">
              <p style="margin:0 0 8px;font-weight:600">命令行重置密码:</p>
              <code style="display:block;background:#f5f7fa;padding:8px;border-radius:4px;white-space:pre-wrap">
./resetpw -password 新密码

# 指定数据目录:
./resetpw -data ./data -password 新密码</code>
              <p style="margin:8px 0 0;color:#909399">重置后使用新密码登录</p>
            </div>
          </el-popover>
        </div>
      </el-form>
    </el-card>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { api, showError } from '../api'
import { ElMessage } from 'element-plus'

const router = useRouter()
const initialized = ref(true)
const busy = ref(false)
const form = ref({ username: '', password: '', confirm: '' })

onMounted(async () => {
  try {
    initialized.value = (await api.get('/api/meta')).initialized
  } catch (e) { showError(e) }
})

async function submit() {
  busy.value = true
  try {
    if (!initialized.value) {
      if (form.value.password !== form.value.confirm) {
        ElMessage.warning('两次密码不一致'); return
      }
      await api.post('/api/init', form.value)
      ElMessage.success('初始化完成,已自动登录')
    } else {
      await api.post('/api/login', form.value)
    }
    router.push('/')
  } catch (e) {
    showError(e)
  } finally {
    busy.value = false
  }
}
</script>
