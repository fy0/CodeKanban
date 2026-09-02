import { h, ref, toValue, type MaybeRefOrGetter, type Ref } from 'vue';
import { useDialog, useMessage } from 'naive-ui';
import { useRoute, useRouter } from 'vue-router';
import { ApiError } from '@/api';
import { webSessionApi } from '@/api/webSession';
import { useAppClipboard } from '@/composables/useAppClipboard';
import { useLocale } from '@/composables/useLocale';
import { useFileManagerStore } from '@/stores/fileManager';
import {
  getClickedMarkdownCodeCopyText,
  getClickedMarkdownLink,
  getClickedMarkdownLinkCopyHref,
  resolveNavigableHref,
} from '@/utils/messageLinkNavigation';
import {
  resolveMessageFileTarget,
  resolveMessageLocalFileName,
  resolveMessageLocalFilePath,
} from '@/utils/messageFileNavigation';
import { buildWorkspaceRouteQuery } from '@/utils/workspaceRoute';

type LocalFileSession = {
  id: string;
  projectId: string;
  cwd: string;
};

export type WebSessionLocalFileAction = '' | 'download' | 'open-file-view' | 'open-location';
export type WebSessionLocalFileTarget = {
  projectId: string;
  sessionId: string;
  path: string;
  name: string;
};

export type WebSessionLocalFileNavigationOptions = {
  currentSession: Readonly<Ref<LocalFileSession | null>>;
  fallbackProjectId: MaybeRefOrGetter<string>;
};

export function useWebSessionLocalFileNavigation({
  currentSession,
  fallbackProjectId,
}: WebSessionLocalFileNavigationOptions) {
  const dialog = useDialog();
  const message = useMessage();
  const route = useRoute();
  const router = useRouter();
  const { copyText } = useAppClipboard();
  const { t } = useLocale();
  const fileManagerStore = useFileManagerStore();
  const show = ref(false);
  const target = ref<WebSessionLocalFileTarget | null>(null);
  const action = ref<WebSessionLocalFileAction>('');

  function clear() {
    show.value = false;
    target.value = null;
    action.value = '';
  }

  function handleVisibilityChange(nextShow: boolean) {
    if (!nextShow && action.value) {
      return;
    }
    show.value = nextShow;
    if (!nextShow) {
      clear();
    }
  }

  function tryOpenLocalFile(rawHref: string) {
    const sourceSession = currentSession.value;
    const projectId = sourceSession?.projectId || toValue(fallbackProjectId);
    if (!sourceSession || !projectId) {
      return false;
    }
    const path = resolveMessageLocalFilePath(rawHref, window.location.href, {
      workingDirectory: sourceSession.cwd,
    });
    if (!path) {
      return false;
    }
    target.value = {
      projectId,
      sessionId: sourceSession.id,
      path,
      name: resolveMessageLocalFileName(path),
    };
    action.value = '';
    show.value = true;
    return true;
  }

  function formatActionError(error: unknown, fallback: string) {
    if (error instanceof ApiError) {
      switch (error.status) {
        case 400:
          return t('webSession.localFileInvalid');
        case 403:
          return t('webSession.localFileOutsideAllowedRoots');
        case 404:
          return t('webSession.localFileNotFound');
        case 501:
        case 503:
          return t('webSession.localFileManagerUnavailable');
      }
    }
    return error instanceof Error && error.message ? error.message : fallback;
  }

  async function openLocation() {
    const currentTarget = target.value;
    if (!currentTarget || action.value) {
      return;
    }
    action.value = 'open-location';
    try {
      await webSessionApi.openLocalFileLocation(
        currentTarget.projectId,
        currentTarget.sessionId,
        currentTarget.path
      );
      if (target.value !== currentTarget) {
        return;
      }
      message.success(t('webSession.localFileOpenLocationSuccess'));
      clear();
    } catch (error) {
      message.error(formatActionError(error, t('webSession.localFileOpenLocationFailed')));
    } finally {
      action.value = '';
    }
  }

  async function openInFileView() {
    const currentTarget = target.value;
    if (!currentTarget || action.value) {
      return;
    }
    action.value = 'open-file-view';
    try {
      const scopes = await fileManagerStore.ensureScopes(currentTarget.projectId);
      if (target.value !== currentTarget) {
        return;
      }
      const fileTarget = resolveMessageFileTarget(
        currentTarget.path,
        window.location.href,
        scopes,
        {
          preferredScopeId: fileManagerStore.getActiveScope(currentTarget.projectId)?.id,
        }
      );
      if (!fileTarget) {
        message.warning(t('webSession.localFileOutsideFileView'));
        return;
      }
      fileManagerStore.requestFileOpen(currentTarget.projectId, fileTarget);
      await router.replace({
        query: buildWorkspaceRouteQuery(route.query, 'files'),
      });
      if (target.value !== currentTarget) {
        return;
      }
      message.success(t('webSession.localFileOpenInFileViewSuccess'));
      clear();
    } catch (error) {
      message.error(formatActionError(error, t('webSession.localFileOpenInFileViewFailed')));
    } finally {
      action.value = '';
    }
  }

  async function download() {
    const currentTarget = target.value;
    if (!currentTarget || action.value) {
      return;
    }
    action.value = 'download';
    try {
      await webSessionApi.probeLocalFile(
        currentTarget.projectId,
        currentTarget.sessionId,
        currentTarget.path
      );
      if (target.value !== currentTarget) {
        return;
      }
      webSessionApi.startLocalFileDownload(
        currentTarget.projectId,
        currentTarget.sessionId,
        currentTarget.path
      );
      message.success(t('webSession.localFileDownloadStarted'));
      clear();
    } catch (error) {
      message.error(formatActionError(error, t('webSession.localFileDownloadFailed')));
    } finally {
      action.value = '';
    }
  }

  function handleTimelineLinkClick(event: MouseEvent) {
    if (event.defaultPrevented || typeof window === 'undefined') {
      return;
    }
    const codeText = getClickedMarkdownCodeCopyText(event.target, event.currentTarget);
    if (codeText) {
      event.preventDefault();
      event.stopPropagation();
      void copyText(codeText, {
        failureMessage: t('terminal.copyFailed'),
        successMessage: t('common.copySuccess'),
      });
      return;
    }
    const copyHref = getClickedMarkdownLinkCopyHref(event.target, event.currentTarget);
    if (copyHref) {
      event.preventDefault();
      event.stopPropagation();
      void copyText(copyHref, {
        failureMessage: t('terminal.copyFailed'),
        successMessage: t('common.linkCopied'),
      });
      return;
    }
    const anchor = getClickedMarkdownLink(event.target, event.currentTarget);
    if (!anchor) {
      return;
    }
    event.preventDefault();
    const rawHref = anchor.getAttribute('href') ?? '';
    if (tryOpenLocalFile(rawHref)) {
      return;
    }
    const href = resolveNavigableHref(rawHref, window.location.href);
    if (!href) {
      message.warning(t('common.invalidLink'));
      return;
    }
    dialog.warning({
      title: t('common.openLinkTitle'),
      content: () =>
        h('div', { class: 'web-session-close-confirm' }, [
          h('div', { class: 'web-session-close-confirm__message' }, [t('common.openLinkMessage')]),
          h('code', { class: 'web-session-close-confirm__href' }, href),
        ]),
      positiveText: t('common.openInNewTab'),
      negativeText: t('common.cancel'),
      onPositiveClick: () => {
        window.open(href, '_blank', 'noopener,noreferrer');
        message.success(t('common.openLinkSuccess'));
      },
    });
  }

  return {
    show,
    target,
    action,
    clear,
    handleVisibilityChange,
    openInFileView,
    openLocation,
    download,
    handleTimelineLinkClick,
  };
}
