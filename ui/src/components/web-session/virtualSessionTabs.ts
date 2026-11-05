export const SESSION_TAB_WIDTH = 200;
export const SESSION_TAB_OVERSCAN = 2;

export function getSessionTabRange(count: number, scrollLeft: number, viewportWidth: number) {
  const start = Math.max(0, Math.floor(scrollLeft / SESSION_TAB_WIDTH) - SESSION_TAB_OVERSCAN);
  const end = Math.min(
    count,
    Math.ceil((scrollLeft + viewportWidth) / SESSION_TAB_WIDTH) + SESSION_TAB_OVERSCAN
  );
  return { start: Math.min(start, Math.max(0, count - 1)), end };
}

export function getSessionTabScrollLeft(index: number, scrollLeft: number, viewportWidth: number) {
  const left = index * SESSION_TAB_WIDTH;
  const right = left + SESSION_TAB_WIDTH;
  if (left < scrollLeft) return left;
  if (right > scrollLeft + viewportWidth) return Math.max(0, right - viewportWidth);
  return scrollLeft;
}

export function getSessionTabDropIndex(sourceIndex: number, offset: number, count: number) {
  const insertion = Math.max(0, Math.min(count, Math.round(offset / SESSION_TAB_WIDTH)));
  return Math.max(0, Math.min(count - 1, insertion > sourceIndex ? insertion - 1 : insertion));
}
