import './assets/main.css';
import 'katex/dist/katex.min.css';
import './styles/chat-markdown.css';
import './styles/project-badges.css';
import './styles/variables.css';
import './styles/fixes.css';
import './styles/appearance.css';
import './styles/appearance-designs.css';

import { createApp } from 'vue';

import dayjs from 'dayjs';
import 'dayjs/locale/zh-cn';
import 'dayjs/locale/en';
import relativeTime from 'dayjs/plugin/relativeTime';

import App from './App.vue';
import router from './router';
import i18n from './i18n';
import { pinia } from './stores/pinia';

// 引入字体: 通用字体 / 等宽字体
import 'vfonts/Lato.css';
import 'vfonts/FiraCode.css';

// 配置dayjs
const currentLocale = localStorage.getItem('app-locale') || 'zh-CN';
dayjs.locale(currentLocale === 'zh-CN' ? 'zh-cn' : 'en');
dayjs.extend(relativeTime);

// naive-ui 样式冲突处理
const meta = document.createElement('meta');
meta.name = 'naive-ui-style';
document.head.appendChild(meta);

const app = createApp(App);

app.use(pinia);
app.use(router);
app.use(i18n);

app.mount('#app');
