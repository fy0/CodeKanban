export const SESSION_TAB_WIDTH = 200;
export const SESSION_TAB_MIN_WIDTH = 140;
export const SESSION_TAB_OVERSCAN = 2;

export function getSessionTabWidth(count: number, viewportWidth: number) {
  if (count <= 0 || viewportWidth <= 0) return SESSION_TAB_WIDTH;
  return Math.min(
    SESSION_TAB_WIDTH,
    Math.max(SESSION_TAB_MIN_WIDTH, Math.floor(viewportWidth / count))
  );
}

export function getSessionTabRange(
  count: number,
  scrollLeft: number,
  viewportWidth: number,
  tabWidth = SESSION_TAB_WIDTH
) {
  const start = Math.max(0, Math.floor(scrollLeft / tabWidth) - SESSION_TAB_OVERSCAN);
  const end = Math.min(
    count,
    Math.ceil((scrollLeft + viewportWidth) / tabWidth) + SESSION_TAB_OVERSCAN
  );
  return { start: Math.min(start, Math.max(0, count - 1)), end };
}

export function getSessionTabScrollLeft(
  index: number,
  scrollLeft: number,
  viewportWidth: number,
  tabWidth = SESSION_TAB_WIDTH
) {
  const left = index * tabWidth;
  const right = left + tabWidth;
  if (left < scrollLeft) return left;
  if (right > scrollLeft + viewportWidth) return Math.max(0, right - viewportWidth);
  return scrollLeft;
}

export function getSessionTabDropIndex(
  sourceIndex: number,
  offset: number,
  count: number,
  tabWidth = SESSION_TAB_WIDTH
) {
  const insertion = Math.max(0, Math.min(count, Math.round(offset / tabWidth)));
  return Math.max(0, Math.min(count - 1, insertion > sourceIndex ? insertion - 1 : insertion));
}
