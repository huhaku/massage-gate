<template>
  <el-container style="height: 100vh">
    <el-aside width="200px" style="background:#1d2935">
      <div style="color:#fff;padding:18px 16px;font-weight:600;font-size:16px">
        📨 massage-gate
        <div style="font-size:11px;font-weight:400;color:#9aa7b4;margin-top:2px">消息中转网关</div>
      </div>
      <el-menu :default-active="$route.path" router background-color="#1d2935"
        text-color="#cfd8e3" active-text-color="#409eff" style="border-right:none">
        <el-menu-item index="/"><el-icon><Odometer /></el-icon>仪表盘</el-menu-item>
        <el-menu-item index="/sources"><el-icon><Download /></el-icon>入站通道</el-menu-item>
        <el-menu-item index="/targets"><el-icon><Upload /></el-icon>出站通道</el-menu-item>
        <el-menu-item index="/routes"><el-icon><Share /></el-icon>路由规则</el-menu-item>
        <el-menu-item index="/messages"><el-icon><Bell /></el-icon>消息中心</el-menu-item>
        <el-menu-item index="/settings"><el-icon><Setting /></el-icon>系统设置</el-menu-item>
      </el-menu>
    </el-aside>
    <el-container>
      <el-header style="background:#fff;border-bottom:1px solid #e6e8eb;display:flex;align-items:center;justify-content:space-between">
        <span style="font-weight:600">{{ $route.meta.title }}</span>
        <el-dropdown @command="onCmd">
          <span style="cursor:pointer;display:flex;align-items:center;gap:6px">
            <el-icon><User /></el-icon>{{ username }}<el-icon><ArrowDown /></el-icon>
          </span>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item command="logout">退出登录</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </el-header>
      <el-main style="padding:16px">
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'

const router = useRouter()
const username = ref('')

onMounted(async () => {
  try {
    username.value = (await api.get('/api/me')).username || ''
  } catch { /* ignore */ }
})

async function onCmd(cmd) {
  if (cmd === 'logout') {
    try { await api.post('/api/logout') } catch { /* ignore */ }
    router.push('/login')
  }
}
</script>
