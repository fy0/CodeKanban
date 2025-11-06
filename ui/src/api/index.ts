import { createAlova } from 'alova';
import fetchAdapter from 'alova/fetch';
import { createApis, withConfigType, mountApis } from './createApis';

// 基础URL配置 - 根据环境动态设置
export let urlBase = import.meta.env.MODE === 'development'
  ? '//' + window.location.hostname + ":" + 3005
  : '//' + window.location.hostname;

export const alovaInstance = createAlova({
  baseURL: urlBase,
  requestAdapter: fetchAdapter(),
  beforeRequest: method => {},
  responded: res => {
    return res.json();
  }
});

export const $$userConfigMap = withConfigType({});

const api = createApis(alovaInstance, $$userConfigMap);

mountApis(api);

export default api;
export { api };

// @ts-ignore
export * from './globals.d.ts';
