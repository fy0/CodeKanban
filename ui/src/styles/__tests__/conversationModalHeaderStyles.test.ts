import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const aiSessionHistoryDialogPath = fileURLToPath(
  new URL('../../components/terminal/AISessionHistoryDialog.vue', import.meta.url)
);
const webSessionImportDialogPath = fileURLToPath(
  new URL('../../components/web-session/WebSessionImportDialog.vue', import.meta.url)
);

const aiSessionHistoryDialog = readFileSync(aiSessionHistoryDialogPath, 'utf8');
const webSessionImportDialog = readFileSync(webSessionImportDialogPath, 'utf8');

describe('conversation modal header styles', () => {
  it('uses dedicated header slots for truncatable titles', () => {
    expect(aiSessionHistoryDialog).toContain('<template #header>');
    expect(webSessionImportDialog).toContain('<template #header>');
  });

  it('allows the card header main area to shrink so ellipsis can apply', () => {
    const components = [aiSessionHistoryDialog, webSessionImportDialog];
    for (const content of components) {
      expect(content).toMatch(/:deep\(\.n-card-header__main\)\s*\{[^}]*min-width:\s*0;[^}]*\}/s);
    }
  });

  it('truncates long conversation titles onto a single line', () => {
    expect(aiSessionHistoryDialog).toMatch(
      /\.conversation-modal-title\s*\{[^}]*overflow:\s*hidden;[^}]*text-overflow:\s*ellipsis;[^}]*white-space:\s*nowrap;[^}]*\}/s
    );
    expect(webSessionImportDialog).toMatch(
      /\.web-session-import__preview-title\s*\{[^}]*overflow:\s*hidden;[^}]*text-overflow:\s*ellipsis;[^}]*white-space:\s*nowrap;[^}]*\}/s
    );
  });
});
