type BodyEnvelope<T> = {
  body?: T | null;
};

type MaybeBody<T> = T | BodyEnvelope<T> | null | undefined;

const hasProp = (value: unknown, key: string): boolean => {
  return typeof value === 'object' && value !== null && key in (value as Record<string, unknown>);
};

export const unwrapBody = <T>(response: MaybeBody<T>): T | undefined => {
  if (!response) {
    return undefined;
  }
  if (hasProp(response, 'body')) {
    const { body } = response as BodyEnvelope<T>;
    return (body ?? undefined) as T | undefined;
  }
  return response as T;
};

export const extractItems = <T>(response: MaybeBody<{ items?: T[] | null } | T[]>): T[] => {
  const payload = unwrapBody(response);
  if (Array.isArray(payload)) {
    return payload as T[];
  }
  if (hasProp(payload, 'items')) {
    const items = (payload as { items?: T[] | null }).items;
    return Array.isArray(items) ? items : [];
  }
  return [];
};

export const extractItem = <T>(response: MaybeBody<{ item?: T } | T>): T | undefined => {
  const payload = unwrapBody(response);
  if (!payload) {
    return undefined;
  }
  if (hasProp(payload, 'item')) {
    const item = (payload as { item?: T }).item;
    return item ?? undefined;
  }
  return payload as T;
};

export type { MaybeBody };
