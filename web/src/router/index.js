import { createRouter, createWebHistory } from 'vue-router'
import { useUserStore } from '@/stores/user'

const routes = [
  {
    path: '/login',
    name: 'Login',
    component: () => import('@/views/login/Login.vue')
  },
  {
    path: '/',
    component: () => import('@/views/layout/Layout.vue'),
    redirect: '/dashboard',
    children: [
      {
        path: '/dashboard',
        name: 'Dashboard',
        component: () => import('@/views/layout/Dashboard.vue'),
        meta: { title: '仪表盘' }
      },
      {
        path: '/system/user',
        name: 'UserManagement',
        component: () => import('@/views/system/User.vue'),
        meta: { title: '用户管理' }
      },
      {
        path: '/mqtt/config',
        name: 'MqttConfig',
        component: () => import('@/views/mqtt/Config.vue'),
        meta: { title: 'MQTT配置' }
      },
      {
        path: '/device/list',
        name: 'DeviceList',
        component: () => import('@/views/device/List.vue'),
        meta: { title: '设备列表' }
      }
    ]
  }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

router.beforeEach((to, from, next) => {
  const token = localStorage.getItem('token')
  const whiteList = ['/login']

  if (whiteList.includes(to.path)) {
    next()
  } else {
    if (token) {
      const userStore = useUserStore()
      if (!userStore.userInfo) {
        userStore.getUserInfo()
      }
      next()
    } else {
      next('/login')
    }
  }
})

export default router
