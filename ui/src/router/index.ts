import { createRouter, createWebHashHistory } from 'vue-router';

const router = createRouter({
  history: createWebHashHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/',
      name: 'projects',
      component: () => import('@/views/ProjectList.vue'),
    },
    {
      path: '/project/:id',
      name: 'project',
      component: () => import('@/views/ProjectWorkspace.vue'),
    },
    {
      path: '/project/:id/branches',
      name: 'project-branches',
      component: () => import('@/views/BranchManagement.vue'),
    },
    {
      path: '/guide',
      name: 'guide',
      component: () => import('@/views/UserGuide.vue'),
    },
    {
      path: '/settings',
      name: 'settings',
      component: () => import('@/views/GeneralSettings.vue'),
    },
  ],
});

export default router;
