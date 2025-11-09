import { createRouter, createWebHistory } from 'vue-router';

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
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
      path: '/pty-test',
      name: 'pty-test',
      component: () => import('@/views/PtyTest.vue'),
    },
  ],
});

export default router;
