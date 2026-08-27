import { useClipboard } from '@vueuse/core';
import { useMessage } from 'naive-ui';
import { useLocale } from '@/composables/useLocale';

export interface AppClipboardCopyOptions {
  failureMessage?: string;
  successMessage?: string;
  unsupportedMessage?: string;
  onError?: (error: unknown) => void;
  onSuccess?: () => void;
}

export function useAppClipboard() {
  const { t } = useLocale();
  const message = useMessage();
  const clipboard = useClipboard({
    legacy: true,
  });

  async function copyText(value: string, options: AppClipboardCopyOptions = {}) {
    if (!value) {
      return false;
    }

    if (!clipboard.isSupported.value) {
      message.error(
        options.unsupportedMessage ?? options.failureMessage ?? t('terminal.copyFailed')
      );
      return false;
    }

    try {
      await clipboard.copy(value);
      if (options.successMessage) {
        message.success(options.successMessage);
      }
      options.onSuccess?.();
      return true;
    } catch (error) {
      options.onError?.(error);
      message.error(options.failureMessage ?? t('terminal.copyFailed'));
      return false;
    }
  }

  return {
    copied: clipboard.copied,
    copyText,
    isSupported: clipboard.isSupported,
    text: clipboard.text,
  };
}
