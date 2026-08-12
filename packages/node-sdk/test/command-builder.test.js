import test from 'node:test';
import assert from 'node:assert/strict';

import { AGENTS, buildAgentLaunchSpec, composeWorkflowPrompt } from '../src/command-builder.js';

test('buildAgentLaunchSpec builds codex plan profile with defaults', () => {
  const result = buildAgentLaunchSpec({
    agent: 'codex',
    profile: 'plan',
    prompt: 'Inspect the repository',
  });

  assert.equal(result.command, 'codex -s workspace-write -a on-request');
  assert.match(result.prompt, /planning mode/i);
  assert.match(result.prompt, /Inspect the repository/);
});

test('buildAgentLaunchSpec appends add-dir and extra args', () => {
  const result = buildAgentLaunchSpec({
    agent: 'codex',
    profile: 'standard',
    prompt: 'Do work',
    permissions: {
      addDirs: ['D:/shared docs'],
      approvalPolicy: 'never',
    },
    extraArgs: ['--search'],
  });

  assert.match(result.command, /codex -s workspace-write -a never --add-dir "D:\/shared docs" --search/);
});

test('buildAgentLaunchSpec rejects yolo with structured permissions', () => {
  assert.throws(
    () =>
      buildAgentLaunchSpec({
        agent: 'codex',
        profile: 'yolo',
        prompt: 'Do work',
        permissions: { addDirs: ['D:/shared'] },
      }),
    /yolo does not accept structured sandbox, approval, or addDirs overrides/i,
  );
});

test('buildAgentLaunchSpec rejects structured permission conflicts inside extraArgs', () => {
  assert.throws(
    () =>
      buildAgentLaunchSpec({
        agent: 'codex',
        profile: 'plan',
        prompt: 'Do work',
        permissions: { sandbox: 'workspace-write' },
        extraArgs: ['--sandbox', 'read-only'],
      }),
    /conflicts with structured permissions/i,
  );
});

test('composeWorkflowPrompt keeps standard profile prompt unchanged', () => {
  assert.equal(composeWorkflowPrompt({ profile: 'standard', prompt: 'Hello' }), 'Hello');
});

test('claude only supports standard profile', () => {
  assert.throws(
    () =>
      buildAgentLaunchSpec({
        agent: 'claude',
        profile: 'plan',
        prompt: 'Hello',
      }),
    /claude only supports the standard profile/i,
  );
});

test('buildAgentLaunchSpec builds Claude CCR terminal command', () => {
  const result = buildAgentLaunchSpec({
    agent: 'claude',
    claudeRuntime: 'ccr',
    prompt: 'Hello',
    extraArgs: ['--model', 'sonnet'],
  });

  assert.equal(result.command, 'ccr code --model sonnet');
  assert.equal(result.claudeRuntime, 'ccr');
});

test('buildAgentLaunchSpec builds Pi plan profile without Codex permission flags', () => {
  assert.deepEqual(AGENTS, ['codex', 'claude', 'pi']);
  const result = buildAgentLaunchSpec({
    agent: 'pi',
    profile: 'plan',
    prompt: 'Inspect the repository',
    extraArgs: ['--model', 'openai/gpt-5'],
  });

  assert.equal(result.command, 'pi --model openai/gpt-5');
  assert.match(result.prompt, /planning mode/i);
  assert.doesNotMatch(result.command, /sandbox|approval|dangerously/i);
});

test('buildAgentLaunchSpec rejects structured permissions for Pi', () => {
  assert.throws(
    () =>
      buildAgentLaunchSpec({
        agent: 'pi',
        prompt: 'Hello',
        permissions: { sandbox: 'workspace-write' },
      }),
    /structured permissions are not supported for pi/i,
  );
});

test('buildAgentLaunchSpec rejects invalid Claude runtime', () => {
  assert.throws(
    () =>
      buildAgentLaunchSpec({
        agent: 'claude',
        claudeRuntime: 'bad',
        prompt: 'Hello',
      }),
    /claudeRuntime must be one of/i,
  );
});
