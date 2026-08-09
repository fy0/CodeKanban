export type WebSessionQuickInputScope = 'global' | 'project';

export const WEB_SESSION_QUICK_INPUT_PAGE_SIZE = 6;

export function filterWebSessionQuickInputItems(items: string[], query: string) {
  const normalizedQuery = query.trim().toLocaleLowerCase();
  if (!normalizedQuery) {
    return items;
  }
  return items.filter(item => item.toLocaleLowerCase().includes(normalizedQuery));
}

export function paginateWebSessionQuickInputItems(
  items: string[],
  page: number,
  pageSize = WEB_SESSION_QUICK_INPUT_PAGE_SIZE
) {
  const normalizedPageSize = Math.max(1, Math.trunc(pageSize));
  const normalizedPage = Math.max(1, Math.trunc(page));
  const start = (normalizedPage - 1) * normalizedPageSize;
  return items.slice(start, start + normalizedPageSize);
}
