<template>
  <el-container class="layout-container">
    <el-aside width="200px" class="sidebar">
      <div class="logo">
        <h3>Edge5</h3>
      </div>
      <el-menu
        :default-active="activeMenu"
        router
        background-color="#304156"
        text-color="#bfcbd9"
        active-text-color="#409EFF"
      >
        <el-menu-item index="/dashboard">
          <el-icon><HomeFilled /></el-icon>
          <span>仪表盘</span>
        </el-menu-item>

        <el-sub-menu index="/system">
          <template #title>
            <el-icon><Setting /></el-icon>
            <span>系统管理</span>
          </template>
          <el-menu-item index="/system/user">用户管理</el-menu-item>
        </el-sub-menu>

        <el-menu-item index="/mqtt/config">
          <el-icon><Connection /></el-icon>
          <span>MQTT配置</span>
        </el-menu-item>

        <el-menu-item index="/device/list">
          <el-icon><Box /></el-icon>
          <span>设备管理</span>
        </el-menu-item>

        <el-menu-item index="/task/list">
          <el-icon><Document /></el-icon>
          <span>采集任务</span>
        </el-menu-item>
      </el-menu>
    </el-aside>

    <el-container>
      <el-header class="header">
        <div class="header-left">
          <el-breadcrumb separator="/">
            <el-breadcrumb-item :to="{ path: '/' }">首页</el-breadcrumb-item>
            <el-breadcrumb-item v-if="currentRoute.meta.title">
              {{ currentRoute.meta.title }}
            </el-breadcrumb-item>
          </el-breadcrumb>
        </div>

        <div class="header-right">
          <el-dropdown @command="handleCommand">
            <span class="user-info">
              <el-avatar :size="32" style="margin-right: 8px">
                {{ userStore.userInfo?.username?.[0]?.toUpperCase() }}
              </el-avatar>
              <span>{{ userStore.userInfo?.nickname || userStore.userInfo?.username }}</span>
            </span>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="profile">详细信息</el-dropdown-item>
                <el-dropdown-item command="changePassword">修改密码</el-dropdown-item>
                <el-dropdown-item command="logout">退出登录</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </el-header>

      <el-main class="main-content">
        <router-view />
      </el-main>
    </el-container>

    <!-- 用户详情弹窗 -->
    <el-dialog v-model="profileDialogVisible" title="用户详细信息" width="500px">
      <el-form :model="profileForm" label-width="80px">
        <el-form-item label="用户名">
          <el-input v-model="profileForm.username" disabled />
        </el-form-item>
        <el-form-item label="昵称">
          <el-input v-model="profileForm.nickname" />
        </el-form-item>
        <el-form-item label="邮箱">
          <el-input v-model="profileForm.email" />
        </el-form-item>
        <el-form-item label="电话">
          <el-input v-model="profileForm.phone" />
        </el-form-item>
        <el-form-item label="角色">
          <el-input v-model="profileForm.roleName" disabled />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="profileDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="updateProfile" :loading="profileLoading">保存</el-button>
      </template>
    </el-dialog>

    <!-- 修改密码弹窗 -->
    <el-dialog v-model="passwordDialogVisible" title="修改密码" width="400px">
      <el-form :model="passwordForm" label-width="80px">
        <el-form-item label="原密码">
          <el-input v-model="passwordForm.old_password" type="password" show-password />
        </el-form-item>
        <el-form-item label="新密码">
          <el-input v-model="passwordForm.new_password" type="password" show-password />
        </el-form-item>
        <el-form-item label="确认密码">
          <el-input v-model="passwordForm.confirm_password" type="password" show-password />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="passwordDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="changePassword" :loading="passwordLoading">确定</el-button>
      </template>
    </el-dialog>
  </el-container>
</template>

<script setup>
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useUserStore } from '@/stores/user'
import { ElMessageBox, ElMessage } from 'element-plus'
import request from '@/utils/request'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()

const currentRoute = computed(() => route)

const activeMenu = computed(() => route.path)

// 用户详情弹窗
const profileDialogVisible = ref(false)
const profileLoading = ref(false)
const profileForm = ref({
  username: '',
  nickname: '',
  email: '',
  phone: '',
  roleName: ''
})

// 修改密码弹窗
const passwordDialogVisible = ref(false)
const passwordLoading = ref(false)
const passwordForm = ref({
  old_password: '',
  new_password: '',
  confirm_password: ''
})

const handleCommand = async (command) => {
  if (command === 'logout') {
    await ElMessageBox.confirm('确定要退出登录吗？', '提示', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning'
    })
    userStore.logout()
    router.push('/login')
  } else if (command === 'changePassword') {
    passwordForm.value = {
      old_password: '',
      new_password: '',
      confirm_password: ''
    }
    passwordDialogVisible.value = true
  } else if (command === 'profile') {
    fetchUserDetail()
    profileDialogVisible.value = true
  }
}

// 获取用户详细信息
const fetchUserDetail = async () => {
  try {
    const res = await request.get('/user/detail')
    profileForm.value = {
      username: res.data?.username || '',
      nickname: res.data?.nickname || '',
      email: res.data?.email || '',
      phone: res.data?.phone || '',
      roleName: res.data?.role?.name || ''
    }
  } catch (error) {
    ElMessage.error('获取用户信息失败')
  }
}

// 更新用户信息
const updateProfile = async () => {
  profileLoading.value = true
  try {
    await request.put('/user/profile', {
      nickname: profileForm.value.nickname,
      email: profileForm.value.email,
      phone: profileForm.value.phone
    })
    ElMessage.success('更新成功')
    profileDialogVisible.value = false
    // 更新 store 中的用户信息
    userStore.getUserInfo()
  } catch (error) {
    ElMessage.error(error.response?.data?.message || '更新失败')
  } finally {
    profileLoading.value = false
  }
}

// 修改密码
const changePassword = async () => {
  if (!passwordForm.value.old_password) {
    ElMessage.warning('请输入原密码')
    return
  }
  if (!passwordForm.value.new_password) {
    ElMessage.warning('请输入新密码')
    return
  }
  if (passwordForm.value.new_password.length < 6) {
    ElMessage.warning('新密码至少6位')
    return
  }
  if (passwordForm.value.new_password !== passwordForm.value.confirm_password) {
    ElMessage.warning('两次输入的密码不一致')
    return
  }

  passwordLoading.value = true
  try {
    await request.put('/user/password', {
      old_password: passwordForm.value.old_password,
      new_password: passwordForm.value.new_password
    })
    ElMessage.success('密码修改成功')
    passwordDialogVisible.value = false
  } catch (error) {
    ElMessage.error(error.response?.data?.message || '密码修改失败')
  } finally {
    passwordLoading.value = false
  }
}
</script>

<style scoped>
.layout-container {
  height: 100vh;
}

.sidebar {
  background-color: #304156;
}

.logo {
  height: 60px;
  display: flex;
  align-items: center;
  justify-content: center;
  background-color: #2b3a4a;
}

.logo h3 {
  color: #fff;
  margin: 0;
}

.header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  background-color: #fff;
  box-shadow: 0 1px 4px rgba(0, 21, 41, 0.08);
}

.header-right .user-info {
  display: flex;
  align-items: center;
  cursor: pointer;
}

.main-content {
  background-color: #f0f2f5;
  padding: 20px;
}
</style>
