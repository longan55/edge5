<template>
  <div class="register-container">
    <el-card class="register-card">
      <template #header>
        <div class="card-header">
          <h2>Edge5 注册</h2>
        </div>
      </template>

      <el-form :model="registerForm" :rules="rules" ref="formRef" label-width="90px">
        <el-form-item label="用户名" prop="username">
          <el-input v-model="registerForm.username" placeholder="请输入用户名" />
        </el-form-item>

        <el-form-item label="密码" prop="password">
          <el-input
            v-model="registerForm.password"
            type="password"
            placeholder="请输入密码"
            show-password
            @keyup.enter="handleRegister"
          />
        </el-form-item>

        <el-form-item label="确认密码" prop="confirmPassword">
          <el-input
            v-model="registerForm.confirmPassword"
            type="password"
            placeholder="请再次输入密码"
            show-password
            @keyup.enter="handleRegister"
          />
        </el-form-item>

        <el-form-item label="昵称" prop="nickname">
          <el-input v-model="registerForm.nickname" placeholder="请输入昵称（可选）" />
        </el-form-item>

        <el-form-item label="邮箱" prop="email">
          <el-input v-model="registerForm.email" placeholder="请输入邮箱（可选）" />
        </el-form-item>

        <el-form-item label="手机号" prop="phone">
          <el-input v-model="registerForm.phone" placeholder="请输入手机号（可选）" />
        </el-form-item>

        <el-form-item label="验证码" prop="captcha">
          <el-input v-model="registerForm.captcha" placeholder="请输入验证码" style="width: 60%" />
          <img :src="captchaImage" @click="refreshCaptcha" class="captcha-img" alt="验证码" />
        </el-form-item>

        <el-form-item>
          <el-button type="primary" @click="handleRegister" :loading="loading" style="width: 100%">
            注册
          </el-button>
        </el-form-item>

        <div class="footer-link">
          已有账号？
          <el-link type="primary" @click="goLogin">去登录</el-link>
        </div>
      </el-form>
    </el-card>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import request from '@/utils/request'
import { ElMessage } from 'element-plus'
import { useUserStore } from '@/stores/user'

const router = useRouter()
const userStore = useUserStore()

const formRef = ref(null)
const loading = ref(false)
const captchaImage = ref('')
const captchaId = ref('')

const registerForm = reactive({
  username: '',
  password: '',
  confirmPassword: '',
  nickname: '',
  email: '',
  phone: '',
  role_id: 1,
  captchaId: '',
  captcha: ''
})

const validateConfirmPassword = (rule, value, callback) => {
  if (!value) return callback(new Error('请再次输入密码'))
  if (value !== registerForm.password) return callback(new Error('两次输入的密码不一致'))
  callback()
}

const rules = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }],
  confirmPassword: [{ validator: validateConfirmPassword, trigger: 'blur' }],
  captcha: [{ required: true, message: '请输入验证码', trigger: 'blur' }]
}

const refreshCaptcha = async () => {
  try {
    const res = await request.get('/captcha')
    captchaId.value = res.data.captcha_id
    captchaImage.value = res.data.captcha
    registerForm.captchaId = res.data.captcha_id
  } catch (error) {
    console.error('获取验证码失败:', error)
  }
}

const handleRegister = async () => {
  await formRef.value.validate(async (valid) => {
    if (!valid) return

    loading.value = true
    try {
      await request.post('/register', {
        username: registerForm.username,
        password: registerForm.password,
        nickname: registerForm.nickname,
        email: registerForm.email,
        phone: registerForm.phone,
        role_id: registerForm.role_id,
        captcha_id: captchaId.value || registerForm.captchaId,
        captcha: registerForm.captcha
      })

      ElMessage.success('注册成功，请登录')
      router.push('/login')
    } catch (error) {
      refreshCaptcha()
    } finally {
      loading.value = false
    }
  })
}

const goLogin = () => {
  router.push('/login')
}

onMounted(() => {
  refreshCaptcha()
  // 若用户已登录则直接跳转首页（可选）
  if (localStorage.getItem('token')) {
    if (userStore.userInfo) {
      router.push('/')
    }
  }
})
</script>

<style scoped>
.register-container {
  width: 100%;
  height: 100vh;
  display: flex;
  justify-content: center;
  align-items: center;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
}

.register-card {
  width: 500px;
}

.card-header {
  text-align: center;
}

.card-header h2 {
  margin: 0;
  color: #333;
}

.captcha-img {
  width: 120px;
  height: 32px;
  margin-left: 10px;
  cursor: pointer;
  border-radius: 4px;
}

.footer-link {
  margin-top: 12px;
  text-align: center;
}
</style>
