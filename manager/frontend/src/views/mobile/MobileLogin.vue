<template>
  <div class="mobile-login-container">
    <div class="mobile-login-header">
      <h1>小智管理系统</h1>
      <p>智能语音助手管理平台</p>
    </div>
    
    <van-tabs v-model:active="activeTab" class="mobile-login-tabs">
      <van-tab title="登录" name="login">
        <van-form @submit="handleLogin" class="mobile-login-form">
          <van-cell-group inset>
            <van-field
              v-model="loginForm.username"
              name="username"
              label="用户名"
              placeholder="请输入用户名"
              :rules="[{ required: true, message: '请输入用户名' }]"
            />
            <van-field
              v-model="loginForm.password"
              type="password"
              name="password"
              label="密码"
              placeholder="请输入密码"
              :rules="[{ required: true, message: '请输入密码' }]"
            />
          </van-cell-group>
          
          <div class="mobile-login-actions">
            <van-button
              round
              block
              type="primary"
              native-type="submit"
              :loading="loading"
              loading-text="登录中..."
              class="mobile-login-button"
            >
              登录
            </van-button>
          </div>
        </van-form>
      </van-tab>
      
      <van-tab title="注册" name="register">
        <van-form @submit="handleRegister" class="mobile-login-form">
          <van-cell-group inset>
            <van-field
              v-model="registerForm.username"
              name="username"
              label="用户名"
              placeholder="请输入用户名"
              :rules="[{ required: true, message: '请输入用户名' }]"
            />
            <van-field
              v-model="registerForm.email"
              name="email"
              label="邮箱"
              placeholder="请输入邮箱"
              :rules="[
                { required: true, message: '请输入邮箱' },
                { pattern: /^[^\s@]+@[^\s@]+\.[^\s@]+$/, message: '请输入正确的邮箱格式' }
              ]"
            />
            <van-field
              v-model="registerForm.password"
              type="password"
              name="password"
              label="密码"
              placeholder="请输入密码（至少6位）"
              :rules="[
                { required: true, message: '请输入密码' },
                { pattern: /^.{6,}$/, message: '密码长度不能少于6位' }
              ]"
            />
            <van-field
              v-model="registerForm.confirmPassword"
              type="password"
              name="confirmPassword"
              label="确认密码"
              placeholder="请确认密码"
              :rules="[
                { required: true, message: '请确认密码' },
                { validator: validateConfirmPassword }
              ]"
            />
          </van-cell-group>
          
          <div class="mobile-login-actions">
            <van-button
              round
              block
              type="primary"
              native-type="submit"
              :loading="loading"
              loading-text="注册中..."
              class="mobile-login-button"
            >
              注册
            </van-button>
          </div>
        </van-form>
      </van-tab>
    </van-tabs>
  </div>
</template>

<script setup>
import { ref, reactive } from 'vue'
import { useRouter } from 'vue-router'
import { showToast, showSuccessToast, showFailToast } from 'vant'
import { useAuthStore } from '../../stores/auth'

const router = useRouter()
const authStore = useAuthStore()

const activeTab = ref('login')
const loading = ref(false)

const loginForm = reactive({
  username: '',
  password: ''
})

const registerForm = reactive({
  username: '',
  email: '',
  password: '',
  confirmPassword: ''
})

// 自定义验证器：确认密码
const validateConfirmPassword = (val) => {
  if (val !== registerForm.password) {
    return '两次输入密码不一致'
  }
  return true
}

const handleLogin = async () => {
  loading.value = true
  const result = await authStore.login(loginForm)
  loading.value = false
  
  if (result.success) {
    showSuccessToast('登录成功')
    // 根据用户角色跳转到不同页面
    if (authStore.user?.role === 'admin') {
      router.push('/dashboard')
    } else {
      router.push('/console')
    }
  } else {
    showFailToast(result.message || '登录失败')
  }
}

const handleRegister = async () => {
  loading.value = true
  const result = await authStore.register(registerForm)
  loading.value = false
  
  if (result.success) {
    showSuccessToast('注册成功，请登录')
    activeTab.value = 'login'
    // 清空注册表单
    Object.assign(registerForm, {
      username: '',
      email: '',
      password: '',
      confirmPassword: ''
    })
  } else {
    showFailToast(result.message || '注册失败')
  }
}
</script>

<style scoped>
.mobile-login-container {
  min-height: 100vh;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  padding: 40px 16px 20px;
  display: flex;
  flex-direction: column;
}

.mobile-login-header {
  text-align: center;
  color: white;
  margin-bottom: 30px;
}

.mobile-login-header h1 {
  font-size: 28px;
  font-weight: 600;
  margin-bottom: 8px;
}

.mobile-login-header p {
  font-size: 14px;
  opacity: 0.9;
}

.mobile-login-tabs {
  flex: 1;
  background: white;
  border-radius: 12px;
  overflow: hidden;
}

.mobile-login-form {
  padding: 20px 0;
}

.mobile-login-actions {
  padding: 20px 16px;
}

.mobile-login-button {
  height: 44px;
  font-size: 16px;
  font-weight: 500;
}

:deep(.van-tabs__nav) {
  background: white;
}

:deep(.van-tabs__line) {
  background: #409EFF;
}
</style>
