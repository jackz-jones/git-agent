import { createRouter, createWebHashHistory } from 'vue-router'

// 路由骨架：首页 / 工作区 / 设置；任务 9 再完善具体内容。
export const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    {
      path: '/',
      name: 'home',
      component: () => import('./views/HomeView.vue'),
    },
    {
      path: '/workspace/:id',
      name: 'workspace',
      component: () => import('./views/WorkspaceView.vue'),
      props: true,
    },
    {
      path: '/settings',
      name: 'settings',
      component: () => import('./views/SettingsView.vue'),
    },
    {
      path: '/:pathMatch(.*)*',
      redirect: '/',
    },
  ],
})
