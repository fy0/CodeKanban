import { existsSync, readFileSync } from 'node:fs';
import { fileURLToPath, URL } from 'node:url';

import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';
import tailwindcss from '@tailwindcss/vite';
import AutoImport from 'unplugin-auto-import/vite';
import { NaiveUiResolver } from 'unplugin-vue-components/resolvers';
import Components from 'unplugin-vue-components/vite';

const stripWrappingQuotes = (value: string) => value.replace(/^['"]|['"]$/g, '');

const readConfigScalar = (content: string, key: string) => {
  const matched = content.match(new RegExp(`^${key}:\\s*(.+)\\s*$`, 'm'));
  if (!matched) {
    return '';
  }

  const rawValue = stripWrappingQuotes(matched[1].trim());
  if (!rawValue || rawValue === 'null') {
    return '';
  }

  return rawValue;
};

const normalizeProxyTarget = (address: string) => {
  const trimmedAddress = address.trim();
  if (!trimmedAddress) {
    return '';
  }

  if (trimmedAddress.includes('://')) {
    return trimmedAddress;
  }

  if (trimmedAddress.startsWith(':')) {
    return `http://127.0.0.1${trimmedAddress}`;
  }

  if (trimmedAddress.startsWith('0.0.0.0:')) {
    return `http://127.0.0.1:${trimmedAddress.slice('0.0.0.0:'.length)}`;
  }

  if (trimmedAddress.startsWith('[::]:')) {
    return `http://127.0.0.1:${trimmedAddress.slice('[::]:'.length)}`;
  }

  return `http://${trimmedAddress}`;
};

const resolveApiProxyTarget = () => {
  const configPaths = [
    fileURLToPath(new URL('../config.yaml', import.meta.url)),
    fileURLToPath(new URL('../data/config.yaml', import.meta.url)),
  ];

  for (const configPath of configPaths) {
    if (!existsSync(configPath)) {
      continue;
    }

    const content = readFileSync(configPath, 'utf8');
    const domain = readConfigScalar(content, 'domain');
    if (domain) {
      return normalizeProxyTarget(domain);
    }

    const serveAt = readConfigScalar(content, 'serveAt');
    if (serveAt) {
      return normalizeProxyTarget(serveAt);
    }
  }

  return 'http://127.0.0.1:3007';
};

const apiProxyTarget = resolveApiProxyTarget();

export default defineConfig({
  base: './',
  server: {
    allowedHosts: true,
    proxy: {
      '/api': {
        target: apiProxyTarget,
        changeOrigin: true,
        ws: true,
      },
    },
  },
  build: {
    rollupOptions: {
      output: {
        chunkFileNames: 'assets/[name]-[hash].js',
        entryFileNames: 'assets/[name]-[hash].js',
        assetFileNames: 'assets/[name]-[hash].[ext]',
      },
    },
  },
  plugins: [
    vue(),
    ...tailwindcss(),
    AutoImport({
      imports: [
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
    alias: [
      {
        find: '@',
        replacement: fileURLToPath(new URL('./src', import.meta.url)),
      },
      {
        find: 'highlight.js/lib/core',
        replacement: fileURLToPath(
          new URL('./node_modules/highlight.js/es/core.js', import.meta.url)
        ),
      },
      {
        find: 'highlight.js/lib/languages',
        replacement: fileURLToPath(
          new URL('./node_modules/highlight.js/es/languages', import.meta.url)
        ),
      },
    ],
  },
});
