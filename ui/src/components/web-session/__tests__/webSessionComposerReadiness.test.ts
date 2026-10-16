import { readFileSync } from 'node:fs';
import {
  computed,
  effectScope,
  nextTick,
  reactive,
  ref,
  watch,
  type ComputedRef,
  type WritableComputedRef,
} from 'vue';
import ts from 'typescript';
import { afterEach, describe, expect, it, vi } from 'vitest';

import {
  isArchivedPreviewSession,
  isDraftSession,
  shouldLoadWebSessionSnapshotOnActivation,
} from '@/components/web-session/webSessionPanelSession';
import { createWebSessionSnapshotLoadController } from '@/utils/webSessionSnapshotLoadController';

const panelSource = readFileSync(new URL('../WebSessionPanel.vue', import.meta.url), 'utf8');
const setupSource = panelSource.split('<script setup lang="ts">')[1]!.split('</script>')[0]!;
const parsedSource = ts.createSourceFile('panel.ts', setupSource, ts.ScriptTarget.Latest, true);
const selectedNames = new Set([
  'currentDraftSessionId',
  'isComposerTargetReady',
  'composerText',
  'setComposerTarget',
  'activateTabById',
  'connectVisibleRealSession',
]);
const readinessSource = parsedSource.statements
  .filter(statement => {
    if (ts.isFunctionDeclaration(statement)) return selectedNames.has(statement.name?.text ?? '');
    if (ts.isVariableStatement(statement)) {
      return statement.declarationList.declarations.some(
        declaration => ts.isIdentifier(declaration.name) && selectedNames.has(declaration.name.text)
      );
    }
    return (
      ts.isExpressionStatement(statement) &&
      ts.isCallExpression(statement.expression) &&
      statement.expression.expression.getText(parsedSource) === 'watch' &&
      statement.getText(parsedSource).includes('setComposerTarget(')
    );
  })
  .map(statement => statement.getText(parsedSource))
  .join('\n');
const compiledSource = ts.transpileModule(readinessSource, {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext },
}).outputText;

type Session = { id: string; projectId: string; revision: string };
type Bindings = {
  currentDraftSessionId: ComputedRef<string>;
  isComposerTargetReady: ComputedRef<boolean>;
  composerText: WritableComputedRef<string>;
  setComposerTarget: (projectId: string, ownerId: string, status: 'loading' | 'ready') => void;
  activateTabById: (sessionId: string, options?: { connectReal?: boolean }) => Promise<boolean>;
  connectVisibleRealSession: (projectId: string, sessionId: string) => Promise<boolean>;
};

function deferred() {
  let resolve!: () => void;
  let reject!: (reason: Error) => void;
  const promise = new Promise<void>((onResolve, onReject) => {
    resolve = onResolve;
    reject = onReject;
  });
  return { promise, resolve, reject };
}

const scopes: ReturnType<typeof effectScope>[] = [];
afterEach(() => scopes.splice(0).forEach(scope => scope.stop()));

function createHarness() {
  const sessions: Session[] = [
    { id: 'session-1', projectId: 'project-1', revision: '1' },
    { id: 'session-2', projectId: 'project-1', revision: '1' },
    { id: 'other-session', projectId: 'project-2', revision: '1' },
  ];
  const currentSession = ref<Session | null>(sessions[0]!);
  const drafts = reactive(new Map([['project-1:session-1', { text: 'unsent draft' }]]));
  const snapshot = deferred();
  const webSessionStore = {
    getSessions: vi.fn((projectId: string) =>
      sessions.filter(item => item.projectId === projectId)
    ),
    setActiveSession: vi.fn((projectId: string, sessionId: string) => {
      currentSession.value =
        sessions.find(item => item.projectId === projectId && item.id === sessionId) ?? null;
    }),
    getHistoryMeta: () => ({ hydrationState: 'uninitialized' }),
    isSessionSnapshotCurrent: () => false,
    catchUpSession: vi.fn(() => snapshot.promise),
    getDraft: (projectId: string, ownerId: string) =>
      drafts.get(`${projectId}:${ownerId}`) ?? { text: '' },
    setDraftText: vi.fn((projectId: string, ownerId: string, text: string) => {
      drafts.set(`${projectId}:${ownerId}`, { text });
    }),
    trimInactiveSessionEvents: vi.fn(),
  };
  const dependencies = {
    computed,
    nextTick,
    watch,
    props: reactive({ projectId: 'project-1', isActive: true }),
    currentSession,
    composerTargetProjectId: ref('project-1'),
    composerTargetOwnerId: ref('session-1'),
    composerTargetStatus: ref('loading'),
    composerFocusRequested: ref(false),
    composerInputRef: ref({ focus: vi.fn() }),
    isProjectSessionInitializing: ref(true),
    pendingRouteActivationSessionId: ref(''),
    visibleSessionById: ref(new Map(sessions.map(session => [session.id, session]))),
    activeArchivedPreviewId: ref(''),
    draftSessions: ref([]),
    realSessionSnapshotLoadController: createWebSessionSnapshotLoadController(),
    replaceDraftSessionState: vi.fn(),
    rememberTabVisit: vi.fn(),
    acknowledgeVisibleSessionView: vi.fn().mockResolvedValue(undefined),
    isDraftSession,
    isArchivedPreviewSession,
    shouldLoadWebSessionSnapshotOnActivation,
    webSessionStore,
    showPiTreeDrawer: ref(false),
    showQuickInputPopover: ref(false),
    pendingUserInputTimelineAnchor: null,
    resetMobileComposerScrollState: vi.fn(),
    isMobile: ref(false),
    isMobileComposerSettingsExpanded: ref(false),
  };
  const initialize = new Function(
    ...Object.keys(dependencies),
    compiledSource + `\nreturn { ${[...selectedNames].join(', ')} };`
  );
  const scope = effectScope();
  scopes.push(scope);
  const bindings = scope.run(() => initialize(...Object.values(dependencies))) as Bindings;
  return { ...dependencies, ...bindings, snapshot };
}

describe('WebSessionPanel composer readiness', () => {
  it.each(['cancel', 'fail'] as const)(
    'restores editable drafts before timeline hydration and keeps them after hydration %s',
    async outcome => {
      const harness = createHarness();
      harness.composerFocusRequested.value = true;
      const activation = harness.activateTabById('session-1');

      expect(harness.isComposerTargetReady.value).toBe(true);
      expect(harness.composerText.value).toBe('unsent draft');
      harness.composerText.value = 'continued typing';
      expect(harness.webSessionStore.getDraft('project-1', 'session-1').text).toBe(
        'continued typing'
      );
      await nextTick();
      expect(harness.webSessionStore.catchUpSession).toHaveBeenCalledOnce();
      expect(harness.composerInputRef.value.focus).toHaveBeenCalledOnce();

      if (outcome === 'cancel') {
        harness.realSessionSnapshotLoadController.cancel();
        harness.snapshot.resolve();
        expect(await activation).toBe(false);
      } else {
        const failure = expect(activation).rejects.toThrow('timeline unavailable');
        harness.snapshot.reject(new Error('timeline unavailable'));
        await failure;
      }
      expect(harness.isComposerTargetReady.value).toBe(true);
      expect(harness.composerText.value).toBe('continued typing');
    }
  );

  it('unlocks an unchanged current session when project initialization finishes', async () => {
    const harness = createHarness();
    expect(harness.isComposerTargetReady.value).toBe(false);

    harness.isProjectSessionInitializing.value = false;
    await nextTick();

    expect(harness.isComposerTargetReady.value).toBe(true);
    expect(harness.composerText.value).toBe('unsent draft');
    expect(harness.resetMobileComposerScrollState).not.toHaveBeenCalled();
  });

  it('keeps an unvalidated route target locked when initialization finishes', async () => {
    const harness = createHarness();
    harness.setComposerTarget('project-1', 'route-session', 'loading');
    harness.pendingRouteActivationSessionId.value = 'route-session';
    harness.isProjectSessionInitializing.value = false;
    await nextTick();

    expect(harness.isComposerTargetReady.value).toBe(false);
    expect(harness.currentDraftSessionId.value).toBe('route-session');
    harness.composerText.value = 'must not overwrite another draft';
    expect(harness.webSessionStore.setDraftText).not.toHaveBeenCalled();
  });

  it('does not unlock a session left over from another project', async () => {
    const harness = createHarness();
    harness.props.projectId = 'project-2';
    harness.isProjectSessionInitializing.value = false;
    await nextTick();

    expect(harness.isComposerTargetReady.value).toBe(false);
    expect(harness.composerTargetProjectId.value).toBe('project-1');
  });

  it.each([
    ['project-1', 'missing-session'],
    ['project-1', 'other-session'],
    ['project-2', 'other-session'],
  ])('does not activate an unvalidated session %s/%s', async (projectId, sessionId) => {
    const harness = createHarness();
    expect(await harness.connectVisibleRealSession(projectId, sessionId)).toBe(false);
    expect(harness.webSessionStore.setActiveSession).not.toHaveBeenCalled();
    expect(harness.webSessionStore.catchUpSession).not.toHaveBeenCalled();
    expect(harness.isComposerTargetReady.value).toBe(false);
  });

  it('unlocks a validated session when its snapshot was already loaded', async () => {
    const harness = createHarness();
    expect(await harness.activateTabById('session-1', { connectReal: false })).toBe(true);
    expect(harness.isComposerTargetReady.value).toBe(true);
    expect(harness.webSessionStore.catchUpSession).not.toHaveBeenCalled();
  });

  it('does not retarget drafts when an old activation finishes after a project switch', async () => {
    const harness = createHarness();
    const activation = harness.activateTabById('session-1');
    await nextTick();

    harness.props.projectId = 'project-2';
    harness.webSessionStore.setActiveSession('project-2', 'other-session');
    harness.setComposerTarget('project-2', 'other-session', 'ready');
    harness.composerText.value = 'new project draft';
    harness.snapshot.resolve();
    await activation;

    expect(harness.isComposerTargetReady.value).toBe(true);
    expect(harness.currentDraftSessionId.value).toBe('other-session');
    expect(harness.composerText.value).toBe('new project draft');
    expect(harness.webSessionStore.getDraft('project-1', 'session-1').text).toBe('unsent draft');
  });
});
