import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { Project } from '@/types/models';

const { getMock, listMock, refreshCommitInfoMock, worktreeListMock } = vi.hoisted(() => ({
  getMock: vi.fn(),
  listMock: vi.fn(),
  refreshCommitInfoMock: vi.fn(),
  worktreeListMock: vi.fn(),
}));

vi.mock('@/api/project', () => ({
  projectApi: {
    clearAccess: vi.fn(),
    create: vi.fn(),
    delete: vi.fn(),
    get: getMock,
    list: listMock,
    markAccess: vi.fn(),
    update: vi.fn(),
  },
  systemApi: {
    openEditor: vi.fn(),
    openExplorer: vi.fn(),
  },
  worktreeApi: {
    create: vi.fn(),
    delete: vi.fn(),
    list: worktreeListMock,
    refreshCommitInfo: refreshCommitInfoMock,
    sync: vi.fn(),
  },
}));

import { useProjectStore } from '@/stores/project';

function createStorageMock() {
  const store = new Map<string, string>();
  return {
    getItem(key: string) {
      return store.has(key) ? store.get(key)! : null;
    },
    setItem(key: string, value: string) {
      store.set(key, String(value));
    },
    removeItem(key: string) {
      store.delete(key);
    },
    clear() {
      store.clear();
    },
  };
}

function makeProject(overrides: Partial<Project> & Pick<Project, 'id' | 'name'>): Project {
  return {
    id: overrides.id,
    name: overrides.name,
    path: `/tmp/${overrides.id}`,
    description: null,
    defaultBranch: 'main',
    worktreeBasePath: null,
    remoteUrl: null,
    hidePath: false,
    priority: null,
    lastSyncAt: null,
    lastAccessedAt: null,
    createdAt: '2026-05-14T00:00:00.000Z',
    updatedAt: '2026-05-14T00:00:00.000Z',
    ...overrides,
  };
}

describe('project store loading states', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.stubGlobal('localStorage', createStorageMock());
    getMock.mockReset();
    listMock.mockReset();
    refreshCommitInfoMock.mockReset();
    worktreeListMock.mockReset();
    worktreeListMock.mockResolvedValue([]);
    refreshCommitInfoMock.mockResolvedValue([]);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('tracks project list loading independently', async () => {
    let resolveList: ((value: { items: Project[]; total: number }) => void) | null = null;
    listMock.mockImplementation(
      () =>
        new Promise<{ items: Project[]; total: number }>(resolve => {
          resolveList = resolve;
        })
    );

    const store = useProjectStore();
    const pending = store.fetchProjects();

    expect(store.projectsLoading).toBe(true);
    expect(store.projectDetailLoading).toBe(false);
    expect(store.loading).toBe(true);

    resolveList?.({ items: [makeProject({ id: 'project-1', name: 'Project 1' })], total: 1 });
    await pending;

    expect(store.projectsLoading).toBe(false);
    expect(store.projectDetailLoading).toBe(false);
    expect(store.loading).toBe(false);
  });

  it('tracks project detail loading independently', async () => {
    const project = makeProject({ id: 'project-2', name: 'Project 2' });
    let resolveProject: ((value: Project) => void) | null = null;
    getMock.mockImplementation(
      () =>
        new Promise<Project>(resolve => {
          resolveProject = resolve;
        })
    );

    const store = useProjectStore();
    const pending = store.fetchProject(project.id);

    expect(store.projectsLoading).toBe(false);
    expect(store.projectDetailLoading).toBe(true);
    expect(store.loading).toBe(true);

    resolveProject?.(project);
    await pending;

    expect(store.projectsLoading).toBe(false);
    expect(store.projectDetailLoading).toBe(false);
    expect(store.loading).toBe(false);
  });
});
