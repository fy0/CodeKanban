import { useMessage } from 'naive-ui';

export function getErrorMessage(error: unknown, fallback = ''): string {
  if (error instanceof Error && error.message) {
    return error.message;
  }
  if (typeof error === 'string' && error) {
    return error;
  }
  if (error && typeof error === 'object' && 'message' in error) {
    const message = Reflect.get(error, 'message');
    if (typeof message === 'string' && message) {
      return message;
    }
  }
  return fallback;
}

export function setupErrorHandler() {
  const message = useMessage();
  const handler = (event: PromiseRejectionEvent) => {
    const reason = event.reason;
    message.error(getErrorMessage(reason, '操作失败，请稍后重试'));
    if (import.meta.env.DEV) {
      console.error('Unhandled promise rejection:', reason);
    }
  };

  window.addEventListener('unhandledrejection', handler);
  return () => {
    window.removeEventListener('unhandledrejection', handler);
  };
}
