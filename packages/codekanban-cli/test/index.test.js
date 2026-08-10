import test from 'node:test';
import assert from 'node:assert/strict';
import http from 'node:http';
import os from 'node:os';
import path from 'node:path';
import { mkdtemp, readFile } from 'node:fs/promises';

import {
  DEFAULT_BASE_URL,
  createAuthHeaders,
  getConfigDir,
  getSessionFilePath,
  readSavedSession,
  resolveBaseUrl,
  runCodeKanbanCli,
  writeSavedSession,
} from '../src/index.js';

function createJsonResponse(payload, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    headers: new Headers(),
    async text() {
      return JSON.stringify(payload);
    },
  };
}

function createMemoryStream() {
  const chunks = [];
  return {
    chunks,
    write(chunk) {
      chunks.push(String(chunk));
    },
    toString() {
      return chunks.join('');
    },
  };
}

function createStdin(text) {
  return {
    async *[Symbol.asyncIterator]() {
      yield text;
    },
  };
}

test('getConfigDir respects Windows and XDG conventions', () => {
  assert.equal(
    getConfigDir({ env: { APPDATA: 'C:/Users/test/AppData/Roaming' }, homeDir: 'C:/Users/test', platform: 'win32' }),
    path.join('C:/Users/test/AppData/Roaming', 'codekanban-cli'),
  );
  assert.equal(
    getConfigDir({ env: { XDG_CONFIG_HOME: '/tmp/config' }, homeDir: '/home/test', platform: 'linux' }),
    path.join('/tmp/config', 'codekanban-cli'),
  );
  assert.equal(
    getConfigDir({ env: {}, homeDir: '/home/test', platform: 'linux' }),
    path.join('/home/test', '.config', 'codekanban-cli'),
  );
});

test('saved session round-trips in the user config directory', async () => {
  const homeDir = await mkdtemp(path.join(os.tmpdir(), 'codekanban-cli-home-'));
  const filePath = await writeSavedSession(
    {
      base_url: 'http://127.0.0.1:3007',
      access_token: 'token-1',
      username: 'alice',
      saved_at: '2026-04-13T10:00:00.000Z',
    },
    { homeDir, platform: 'linux', env: {} },
  );
  assert.equal(filePath, getSessionFilePath({ homeDir, platform: 'linux', env: {} }));

  const session = await readSavedSession({ homeDir, platform: 'linux', env: {} });
  assert.deepEqual(session, {
    base_url: 'http://127.0.0.1:3007',
    access_token: 'token-1',
    username: 'alice',
    saved_at: '2026-04-13T10:00:00.000Z',
  });
});

test('resolveBaseUrl follows arg, env, saved session, default order', () => {
  assert.equal(resolveBaseUrl({ flags: { baseUrl: 'http://arg:3007' }, env: {}, savedSession: null }), 'http://arg:3007');
  assert.equal(resolveBaseUrl({ flags: {}, env: { CODEKANBAN_BASE_URL: 'http://env:3007' }, savedSession: null }), 'http://env:3007');
  assert.equal(resolveBaseUrl({ flags: {}, env: {}, savedSession: { base_url: 'http://saved:3007' } }), 'http://saved:3007');
  assert.equal(resolveBaseUrl({ flags: {}, env: {}, savedSession: null }), DEFAULT_BASE_URL);
});

test('createAuthHeaders emits both Authorization and Cookie headers', () => {
  assert.deepEqual(createAuthHeaders('abc123'), {
    Authorization: 'Bearer abc123',
    Cookie: 'codekanban_auth=abc123',
  });
  assert.deepEqual(createAuthHeaders(''), {});
});

test('runCodeKanbanCli prints help and version', async () => {
  const stdout = createMemoryStream();
  const stderr = createMemoryStream();
  const helpCode = await runCodeKanbanCli(['--help'], { stdout, stderr, version: '9.9.9' });
  assert.equal(helpCode, 0);
  assert.match(stdout.toString(), /codekanban-cli - installable Codex skill CLI/);
  assert.equal(stderr.toString(), '');

  const versionOut = createMemoryStream();
  const versionCode = await runCodeKanbanCli(['--version'], { stdout: versionOut, stderr: createMemoryStream(), version: '9.9.9' });
  assert.equal(versionCode, 0);
  assert.equal(versionOut.toString(), 'codekanban-cli 9.9.9\n');
});

test('runCodeKanbanCli rejects removed Kanban scopes before contacting the server', async () => {
  for (const scope of ['task', 'tasks', 'kanban', 'board']) {
    let fetchCalls = 0;
    let runnerCalls = 0;
    const stderr = createMemoryStream();
    const code = await runCodeKanbanCli([scope, 'list'], {
      stdout: createMemoryStream(),
      stderr,
      fetchImpl: async () => {
        fetchCalls += 1;
        throw new Error('unexpected request');
      },
      runner: async () => {
        runnerCalls += 1;
        return 0;
      },
    });

    assert.equal(code, 1);
    assert.match(stderr.toString(), /removed with the Kanban task API/);
    assert.equal(fetchCalls, 0);
    assert.equal(runnerCalls, 0);
  }
});

test('runCodeKanbanCli injects default base URL and saved token into the command runner', async () => {
  const homeDir = await mkdtemp(path.join(os.tmpdir(), 'codekanban-cli-home-'));
  await writeSavedSession(
    {
      base_url: 'http://127.0.0.1:3007',
      access_token: 'saved-token',
      username: '',
      saved_at: '2026-04-13T10:00:00.000Z',
    },
    { homeDir, platform: 'linux', env: {} },
  );

  let received = null;
  const code = await runCodeKanbanCli(['session', 'list', '--path', '/repo'], {
    homeDir,
    platform: 'linux',
    env: {},
    stdout: createMemoryStream(),
    stderr: createMemoryStream(),
    runner: async (argv, options) => {
      received = { argv, options };
      return 0;
    },
  });

  assert.equal(code, 0);
  assert.deepEqual(received.argv, ['session', 'list', '--path', '/repo']);
  assert.equal(received.options.defaultBaseURL, 'http://127.0.0.1:3007');
  assert.equal(received.options.clientOptions.headers.Authorization, 'Bearer saved-token');
  assert.equal(received.options.clientOptions.headers.Cookie, 'codekanban_auth=saved-token');
});

test('auth save-token validates a password, saves the session, and auth status can use it', async () => {
  const password = 'swordfish';
  const token = 'live-token-123';
  const homeDir = await mkdtemp(path.join(os.tmpdir(), 'codekanban-cli-home-'));
  const { pbkdf2Sync } = await import('node:crypto');
  const expectedHash = pbkdf2Sync(password, 'salt-1', 20000, 64, 'sha512').toString('hex');
  let loginCalls = 0;

  const server = http.createServer(async (req, res) => {
    if (req.url === '/api/v1/auth/status' && req.method === 'GET') {
      const auth = req.headers.authorization || '';
      const authenticated = auth === `Bearer ${token}`;
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({
        body: {
          item: {
            enabled: true,
            authenticated,
            frontendSalt: 'salt-1',
            frontendPBKDF2Rounds: 20000,
            sessionTtlSeconds: 60,
          },
        },
      }));
      return;
    }
    if (req.url === '/api/v1/auth/login' && req.method === 'POST') {
      loginCalls += 1;
      const chunks = [];
      for await (const chunk of req) {
        chunks.push(chunk);
      }
      const payload = JSON.parse(Buffer.concat(chunks).toString('utf8'));
      assert.equal(payload.clientHash, expectedHash);
      res.writeHead(200, {
        'Content-Type': 'application/json',
        'Set-Cookie': 'codekanban_auth=live-token-123; Path=/; HttpOnly; SameSite=Lax',
      });
      res.end(JSON.stringify({ body: { message: 'login successful' } }));
      return;
    }
    res.writeHead(404, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ detail: 'not found' }));
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  const address = server.address();
  const baseUrl = `http://127.0.0.1:${address.port}`;

  try {
    const stdout = createMemoryStream();
    const stderr = createMemoryStream();
    const code = await runCodeKanbanCli(['auth', 'save-token', '--base-url', baseUrl, '--password-stdin', '--username', 'alice'], {
      homeDir,
      platform: 'linux',
      env: {},
      stdin: createStdin(`${password}\n`),
      stdout,
      stderr,
      fetchImpl: globalThis.fetch,
      now: new Date('2026-04-13T12:00:00.000Z'),
    });
    assert.equal(code, 0);
    assert.equal(loginCalls, 1);
    assert.equal(stderr.toString(), '');
    assert.match(stdout.toString(), /token saved/);

    const session = JSON.parse(await readFile(getSessionFilePath({ homeDir, platform: 'linux', env: {} }), 'utf8'));
    assert.deepEqual(session, {
      base_url: baseUrl,
      access_token: token,
      username: 'alice',
      saved_at: '2026-04-13T12:00:00.000Z',
    });

    const statusOut = createMemoryStream();
    const statusCode = await runCodeKanbanCli(['auth', 'status', '--base-url', baseUrl], {
      homeDir,
      platform: 'linux',
      env: {},
      stdout: statusOut,
      stderr: createMemoryStream(),
      fetchImpl: globalThis.fetch,
    });
    assert.equal(statusCode, 0);
    assert.match(statusOut.toString(), /"authenticated": true/);
  } finally {
    server.close();
  }
});


test('project list returns server-side project names and ids', async () => {
  const stdout = createMemoryStream();
  const code = await runCodeKanbanCli(['project', 'list', '--base-url', 'http://remote.example:3007'], {
    stdout,
    stderr: createMemoryStream(),
    fetchImpl: async input => {
      const url = input instanceof URL ? input : new URL(String(input));
      assert.equal(url.pathname, '/api/v1/projects');
      return createJsonResponse({
        items: [
          { id: 'p1', name: 'codekanban', path: '/srv/codekanban' },
          { id: 'p2', name: 'demo', path: '/srv/demo' },
        ],
      });
    },
  });

  assert.equal(code, 0);
  const payload = JSON.parse(stdout.toString());
  assert.equal(payload.items.length, 2);
  assert.equal(payload.items[0].name, 'codekanban');
  assert.equal(payload.items[0].pathBaseName, 'codekanban');
});

test('project-name resolves to project-id before delegating to the command runner', async () => {
  let received = null;
  const code = await runCodeKanbanCli(['web-session', 'create', '--project-name', 'codekanban', '--agent', 'codex'], {
    stdout: createMemoryStream(),
    stderr: createMemoryStream(),
    fetchImpl: async input => {
      const url = input instanceof URL ? input : new URL(String(input));
      assert.equal(url.pathname, '/api/v1/projects');
      return createJsonResponse({
        items: [
          { id: 'p-codekanban', name: 'codekanban', path: '/srv/codekanban' },
        ],
      });
    },
    runner: async (argv, options) => {
      received = { argv, options };
      return 0;
    },
    env: { CODEKANBAN_BASE_URL: 'http://remote.example:3007' },
  });

  assert.equal(code, 0);
  assert.deepEqual(received.argv, ['web-session', 'create', '--agent', 'codex', '--project-id', 'p-codekanban']);
  assert.equal(received.options.defaultBaseURL, 'http://remote.example:3007');
});

test('project-name can pick an ambiguous project with --project-index', async () => {
  let received = null;
  const code = await runCodeKanbanCli(['web-session', 'create', '--project-name', 'codekanban', '--project-index', '2', '--agent', 'codex'], {
    stdout: createMemoryStream(),
    stderr: createMemoryStream(),
    fetchImpl: async () => createJsonResponse({
      items: [
        { id: 'p1', name: 'codekanban', path: '/srv/codekanban-a' },
        { id: 'p2', name: 'codekanban', path: '/srv/codekanban-b' },
      ],
    }),
    runner: async argv => {
      received = argv;
      return 0;
    },
    env: { CODEKANBAN_BASE_URL: 'http://remote.example:3007' },
  });

  assert.equal(code, 0);
  assert.deepEqual(received, ['web-session', 'create', '--agent', 'codex', '--project-id', 'p2']);
});

test('ambiguous project-name errors list candidates and the project-index hint', async () => {
  const stderr = createMemoryStream();
  const code = await runCodeKanbanCli(['web-session', 'create', '--project-name', 'codekanban', '--agent', 'codex'], {
    stdout: createMemoryStream(),
    stderr,
    fetchImpl: async () => createJsonResponse({
      items: [
        { id: 'p1', name: 'codekanban', path: '/srv/codekanban-a' },
        { id: 'p2', name: 'codekanban', path: '/srv/codekanban-b' },
      ],
    }),
    env: { CODEKANBAN_BASE_URL: 'http://remote.example:3007' },
  });

  assert.equal(code, 1);
  assert.match(stderr.toString(), /--project-index <n>/);
  assert.match(stderr.toString(), /codekanban \[p1\]/);
  assert.match(stderr.toString(), /codekanban \[p2\]/);
});

test('local project commands fall back to the current working directory when no target is given', async () => {
  let received = null;
  const code = await runCodeKanbanCli(['session', 'list'], {
    stdout: createMemoryStream(),
    stderr: createMemoryStream(),
    runner: async argv => {
      received = argv;
      return 0;
    },
    cwd: '/worktrees/codekanban',
    env: { CODEKANBAN_BASE_URL: 'http://127.0.0.1:3007' },
  });

  assert.equal(code, 0);
  assert.deepEqual(received, ['session', 'list', '--path', '/worktrees/codekanban']);
});

test('remote project commands require project-id, project-name, or a server-side path', async () => {
  const stderr = createMemoryStream();
  const code = await runCodeKanbanCli(['session', 'list', '--base-url', 'http://remote.example:3007'], {
    stdout: createMemoryStream(),
    stderr,
  });

  assert.equal(code, 1);
  assert.match(stderr.toString(), /remote CodeKanban commands require --project-id, --project-name, or a server-side --path/);
});
