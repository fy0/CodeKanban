import type {
  Alova,
  AlovaGenerics,
  AlovaMethodCreateConfig,
  Method,
  RequestBody,
  RespondedAlovaGenerics,
} from 'alova';
import { alovaInstance } from './index';
import { unwrapBody } from './response';

type CurrentAG = typeof alovaInstance extends Alova<infer AG> ? AG : AlovaGenerics;
type HttpMethod<Responded, Transformed> = Method<
  RespondedAlovaGenerics<CurrentAG, Responded, Transformed>
>;
type HttpConfig<Responded, Transformed> = AlovaMethodCreateConfig<
  CurrentAG,
  Responded,
  Transformed
>;

interface HttpClient {
  Get<Responded = unknown, Transformed = Responded>(
    path: string,
    config?: HttpConfig<Responded, Transformed>,
  ): HttpMethod<Responded, Transformed>;
  Post<Responded = unknown, Transformed = Responded>(
    path: string,
    data?: RequestBody,
    config?: HttpConfig<Responded, Transformed>,
  ): HttpMethod<Responded, Transformed>;
  Put<Responded = unknown, Transformed = Responded>(
    path: string,
    data?: RequestBody,
    config?: HttpConfig<Responded, Transformed>,
  ): HttpMethod<Responded, Transformed>;
  Patch<Responded = unknown, Transformed = Responded>(
    path: string,
    data?: RequestBody,
    config?: HttpConfig<Responded, Transformed>,
  ): HttpMethod<Responded, Transformed>;
  Delete<Responded = unknown, Transformed = Responded>(
    path: string,
    data?: RequestBody,
    config?: HttpConfig<Responded, Transformed>,
  ): HttpMethod<Responded, Transformed>;
}

const API_PREFIX = '/api/v1';
const ABSOLUTE_URL_PATTERN = /^([a-z][a-z\d+\-.]*:)?\/\//i;

const normalizePath = (path: string) => {
  if (!path) {
    return API_PREFIX;
  }

  if (ABSOLUTE_URL_PATTERN.test(path)) {
    return path;
  }

  const ensured = path.startsWith('/') ? path : `/${path}`;
  if (ensured.startsWith(API_PREFIX)) {
    return ensured;
  }
  return `${API_PREFIX}${ensured}`;
};

const enhanceConfig = <Responded, Transformed>(
  config?: HttpConfig<Responded, Transformed>,
) => {
  const nextConfig: HttpConfig<Responded, Transformed> = { ...config };
  const originalTransform = nextConfig.transform;

  nextConfig.transform = async (...args) => {
    const transformed = originalTransform ? await originalTransform(...args) : args[0];
    return unwrapBody(transformed) as Responded;
  };

  return nextConfig;
};

export const http: HttpClient = {
  Get(path, config) {
    return alovaInstance.Get(normalizePath(path), enhanceConfig(config));
  },
  Post(path, data, config) {
    return alovaInstance.Post(normalizePath(path), data, enhanceConfig(config));
  },
  Put(path, data, config) {
    return alovaInstance.Put(normalizePath(path), data, enhanceConfig(config));
  },
  Patch(path, data, config) {
    return alovaInstance.Patch(normalizePath(path), data, enhanceConfig(config));
  },
  Delete(path, data, config) {
    return alovaInstance.Delete(normalizePath(path), data, enhanceConfig(config));
  },
};

export type { HttpClient };
