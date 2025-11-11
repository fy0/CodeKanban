import { useMessage } from 'naive-ui';

export function setupErrorHandler() {
  const message = useMessage();
  const handler = (event: PromiseRejectionEvent) => {
    const reason = event.reason;
    if (reason && typeof reason === 'object' && typeof reason.message === 'string') {
      message.error(reason.message);
    } else if (typeof reason === 'string') {
      message.error(reason);
    } else {
      message.error('操作失败，请稍后重试');
    }
    if (import.meta.env.DEV) {
      // eslint-disable-next-line no-console
      console.error('Unhandled promise rejection:', reason);
    }
  };

  window.addEventListener('unhandledrejection', handler);
  return () => {
    window.removeEventListener('unhandledrejection', handler);
  };
}
