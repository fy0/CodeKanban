import { createPinia, setActivePinia } from 'pinia';
import { ref } from 'vue';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { useWebSessionLocalFileNavigation } from '@/components/web-session/useWebSessionLocalFileNavigation';
import { useFileManagerStore } from '@/stores/fileManager';

const { copyTextMock, errorMock, listScopesMock, replaceMock, successMock, warningMock } =
  vi.hoisted(() => ({
    copyTextMock: vi.fn(),
    errorMock: vi.fn(),
    listScopesMock: vi.fn(),
    replaceMock: vi.fn(),
    successMock: vi.fn(),
    warningMock: vi.fn(),
  }));

vi.mock('naive-ui', () => ({
  useDialog: () => ({ warning: vi.fn() }),
  useMessage: () => ({
    error: errorMock,
    success: successMock,
    warning: warningMock,
  }),
}));

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: { session: 'session-1', tab: 'web' } }),
  useRouter: () => ({ replace: replaceMock }),
}));

vi.mock('@/api/fileManager', () => ({
  fileManagerApi: {
    listScopes: listScopesMock,
  },
}));

vi.mock('@/composables/useAppClipboard', () => ({
  useAppClipboard: () => ({ copyText: copyTextMock }),
}));

vi.mock('@/composables/useLocale', () => ({
  useLocale: () => ({ t: (key: string) => key }),
}));

describe('useWebSessionLocalFileNavigation', () => {
  beforeEach(() => {
    vi.stubGlobal('window', {
      location: {
        href: 'http://localhost:5173/projects/project-1?tab=web',
      },
    });
    setActivePinia(createPinia());
    copyTextMock.mockReset();
    errorMock.mockReset();
    listScopesMock.mockReset();
    replaceMock.mockReset();
    replaceMock.mockResolvedValue(undefined);
    successMock.mockReset();
    warningMock.mockReset();
  });

  it('opens a project file in the file view', async () => {
    listScopesMock.mockResolvedValue([
      {
        id: 'scope-1',
        kind: 'project',
        label: 'Project',
        rootPath: 'D:/codes/2025/codekanban',
      },
    ]);
    const navigation = useWebSessionLocalFileNavigation({
      currentSession: ref({
        id: 'session-1',
        projectId: 'project-1',
        cwd: 'D:/codes/2025/codekanban',
      }),
      fallbackProjectId: 'project-1',
    });
    navigation.show.value = true;
    navigation.target.value = {
      projectId: 'project-1',
      sessionId: 'session-1',
      name: 'README.md',
      path: 'D:/codes/2025/codekanban/docs/README.md',
    };

    await navigation.openInFileView();

    expect(useFileManagerStore().getOpenRequest('project-1')).toMatchObject({
      scopeId: 'scope-1',
      path: 'docs/README.md',
    });
    expect(replaceMock).toHaveBeenCalledWith({
      query: { session: 'session-1', tab: 'files' },
    });
    expect(successMock).toHaveBeenCalledWith('webSession.localFileOpenInFileViewSuccess');
    expect(navigation.show.value).toBe(false);
    expect(navigation.action.value).toBe('');
  });

  it('keeps the dialog open when the file is outside every file view scope', async () => {
    listScopesMock.mockResolvedValue([
      {
        id: 'scope-1',
        kind: 'project',
        label: 'Project',
        rootPath: 'D:/codes/2025/codekanban',
      },
    ]);
    const navigation = useWebSessionLocalFileNavigation({
      currentSession: ref({
        id: 'session-1',
        projectId: 'project-1',
        cwd: 'D:/codes/2025/codekanban',
      }),
      fallbackProjectId: 'project-1',
    });
    navigation.show.value = true;
    navigation.target.value = {
      projectId: 'project-1',
      sessionId: 'session-1',
      name: 'result.txt',
      path: 'D:/temp/result.txt',
    };

    await navigation.openInFileView();

    expect(warningMock).toHaveBeenCalledWith('webSession.localFileOutsideFileView');
    expect(replaceMock).not.toHaveBeenCalled();
    expect(useFileManagerStore().getOpenRequest('project-1')).toBeNull();
    expect(navigation.show.value).toBe(true);
    expect(navigation.action.value).toBe('');
  });
});
