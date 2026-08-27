export function debounce<Args extends unknown[]>(fn: (...args: Args) => void, delay: number) {
  let timeoutId: ReturnType<typeof setTimeout> | null = null;

  return function debounced(...args: Args) {
    if (timeoutId) {
      clearTimeout(timeoutId);
    }
    timeoutId = setTimeout(() => {
      fn(...args);
    }, delay);
  };
}
