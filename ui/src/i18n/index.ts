import { createI18n } from 'vue-i18n';
import zhCN from './locales/zh-CN';
import enUS from './locales/en-US';

export type MessageSchema = typeof zhCN;

/**
 * 检测浏览器语言并映射到支持的语言
 */
function detectBrowserLocale(): 'zh-CN' | 'en-US' {
  if (typeof navigator === 'undefined') {
    return 'zh-CN';
  }
  // 获取浏览器语言列表
  const browserLanguages = navigator.languages || [navigator.language];

  for (const lang of browserLanguages) {
    const lowercaseLang = lang.toLowerCase();

    // 检查是否为中文
    if (lowercaseLang.startsWith('zh')) {
      return 'zh-CN';
    }

    // 检查是否为英文
    if (lowercaseLang.startsWith('en')) {
      return 'en-US';
    }
  }

  // 默认返回中文
  return 'zh-CN';
}

function getLocaleStorage() {
  if (typeof localStorage !== 'undefined') {
    return localStorage;
  }
  return null;
}

const localeStorage = getLocaleStorage();
// 从 localStorage 获取保存的语言，如果没有则根据浏览器语言自动检测
const savedLocale = localeStorage?.getItem('app-locale');
const initialLocale = savedLocale || detectBrowserLocale();

// 如果是首次访问（没有保存的语言），保存检测到的语言
if (!savedLocale && localeStorage) {
  localeStorage.setItem('app-locale', initialLocale);
}

const i18n = createI18n<[MessageSchema], 'zh-CN' | 'en-US'>({
  legacy: false, // 使用 Composition API 模式
  locale: initialLocale,
  fallbackLocale: 'zh-CN',
  messages: {
    'zh-CN': zhCN,
    'en-US': enUS,
  },
});

export default i18n;
