import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { Project } from '@/types/models';

const { clearAccessMock, markAccessMock } = vi.hoisted(() => ({
  clearAccessMock: vi.fn(),
  markAccessMock: vi.fn(),
}));

vi.mock('@/api/project', () => ({
  projectApi: {
    clearAccess: clearAccessMock,
    create: vi.fn(),
    delete: vi.fn(),
    get: vi.fn(),
    list: vi.fn(),
    markAccess: markAccessMock,
    update: vi.fn(),
  },
  systemApi: {
    openEditor: vi.fn(),
    openExplorer: vi.fn(),
  },
  worktreeApi: {
    create: vi.fn(),
    delete: vi.fn(),
    list: vi.fn(),
    refreshCommitInfo: vi.fn(),
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

describe('project recent ordering', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.stubGlobal('localStorage', createStorageMock());
    markAccessMock.mockReset();
    clearAccessMock.mockReset();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it('uses backend access timestamps instead of localStorage order', () => {
    localStorage.setItem('recent_projects', JSON.stringify(['old-project', 'new-project']));
    const store = useProjectStore();

    store.projects = [
      makeProject({
        id: 'old-project',
        name: 'Old Project',
        lastAccessedAt: '2026-05-14T01:00:00.000Z',
      }),
      makeProject({
        id: 'new-project',
        name: 'New Project',
        lastAccessedAt: '2026-05-14T02:00:00.000Z',
      }),
      makeProject({ id: 'never-opened', name: 'Never Opened' }),
    ];

    expect(store.recentProjects.map(project => project.id)).toEqual(['new-project', 'old-project']);
  });

  it('records and clears recent state through backend APIs', async () => {
    const store = useProjectStore();
    const project = makeProject({ id: 'project-1', name: 'Project 1' });
    const accessedProject = { ...project, lastAccessedAt: '2026-05-14T03:00:00.000Z' };
    store.projects = [project];
    markAccessMock.mockResolvedValue(accessedProject);
    clearAccessMock.mockResolvedValue({ ...project, lastAccessedAt: null });

    store.addRecentProject(project.id);

    expect(markAccessMock).toHaveBeenCalledWith(project.id);
    expect(store.projects[0].lastAccessedAt).toBeNull();
    await Promise.resolve();
    expect(store.projects[0].lastAccessedAt).toBe(accessedProject.lastAccessedAt);

    store.removeRecentProject(project.id);

    expect(clearAccessMock).toHaveBeenCalledWith(project.id);
    expect(store.projects[0].lastAccessedAt).toBe(accessedProject.lastAccessedAt);
    await Promise.resolve();
    expect(store.projects[0].lastAccessedAt).toBeNull();
  });

  it('throttles repeated access writes for the same project', async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-05-14T00:00:00.000Z'));
    const store = useProjectStore();
    const project = makeProject({ id: 'project-throttle', name: 'Throttle' });
    markAccessMock.mockResolvedValue(project);

    store.addRecentProject(project.id);
    store.addRecentProject(project.id);
    expect(markAccessMock).toHaveBeenCalledTimes(1);

    await vi.advanceTimersByTimeAsync(60_000);
    store.addRecentProject(project.id);
    expect(markAccessMock).toHaveBeenCalledTimes(2);
  });
});
