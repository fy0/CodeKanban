import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const panelPath = fileURLToPath(new URL('../WebSessionPanel.vue', import.meta.url));
const dialogPath = fileURLToPath(
  new URL('../../project/PiProjectTrustDialog.vue', import.meta.url)
);
const panelSource = readFileSync(panelPath, 'utf8');
const dialogSource = readFileSync(dialogPath, 'utf8');

function sourceBetween(source: string, start: string, end: string) {
  const startIndex = source.indexOf(start);
  const endIndex = source.indexOf(end, startIndex + start.length);
  expect(startIndex).toBeGreaterThanOrEqual(0);
  expect(endIndex).toBeGreaterThan(startIndex);
  return source.slice(startIndex, endIndex);
}

describe('Pi project trust UI', () => {
  it('checks server trust before selecting Pi or submitting a Pi message', () => {
    const selectionSource = sourceBetween(
      panelSource,
      'async function handleAgentDropdownSelect',
      'async function ensurePiProjectTrust'
    );
    const submitSource = sourceBetween(
      panelSource,
      'async function handleSubmit()',
      'async function handleConfirmScheduledSend()'
    );
    const scheduledSource = sourceBetween(
      panelSource,
      'async function handleConfirmScheduledSend()',
      'async function handleConfirmScheduledInputUpdate()'
    );
    const importSource = sourceBetween(
      panelSource,
      'async function handleImportCodexSession',
      'async function handleCreateSession'
    );

    expect(selectionSource).toContain("next === 'pi' && !(await ensurePiProjectTrust(true))");
    expect(selectionSource.indexOf('await ensurePiProjectTrust')).toBeLessThan(
      selectionSource.indexOf('selectedAgent.value = next')
    );
    expect(submitSource).toContain("submitAgent === 'pi' && !(await ensurePiProjectTrust())");
    expect(scheduledSource).toContain("submitAgent === 'pi' && !(await ensurePiProjectTrust())");
    expect(importSource).toContain("agent === 'pi' && !(await ensurePiProjectTrust())");
    expect(importSource.indexOf('await ensurePiProjectTrust')).toBeLessThan(
      importSource.indexOf('webSessionStore.importSession')
    );
  });

  it('authorizes only through the server-owned project endpoint', () => {
    expect(dialogSource).toContain('await projectApi.trustForPi(props.projectId)');
    expect(dialogSource).toContain("emit('trusted', status)");
    expect(dialogSource).not.toContain('trusted: true');
    expect(dialogSource).not.toMatch(/trustForPi\([^)]*,/);
  });
});
