import { fileURLToPath, URL } from 'node:url';

import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';
import vueJsx from '@vitejs/plugin-vue-jsx';
// import vueDevTools from 'vite-plugin-vue-devtools'
import tailwindcss from '@tailwindcss/vite';
import AutoImport from 'unplugin-auto-import/vite';
import { NaiveUiResolver } from 'unplugin-vue-components/resolvers';
import Components from 'unplugin-vue-components/vite';

// https://vite.dev/config/
export default defineConfig({
  base: './', // 使用相对路径，支持部署到任意路径
  build: {
    rollupOptions: {
      output: {
        // 自定义 chunk 文件名，去掉下划线前缀
        chunkFileNames: 'assets/[name]-[hash].js',
        // 同时也可以自定义入口文件和资源文件名
        entryFileNames: 'assets/[name]-[hash].js',
        assetFileNames: 'assets/[name]-[hash].[ext]',
      },
    },
  },
  plugins: [
    vue(),
    vueJsx(),
    tailwindcss(),
    // vueDevTools(),
    AutoImport({
      imports: [
        // 'vue', // 感觉vue自动引入有点乱，还是手动吧
        {
          'naive-ui': ['useDialog', 'useMessage', 'useNotification', 'useLoadingBar'],
        },
      ],
    }),
    Components({
      resolvers: [NaiveUiResolver()],
    }),
  ],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
});
