import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const webSessionImportDialogPath = fileURLToPath(
  new URL('../WebSessionImportDialog.vue', import.meta.url)
);
const webSessionImportDialogSource = readFileSync(webSessionImportDialogPath, 'utf8');

describe('webSession import dialog layout', () => {
  it('renders card actions in a full-width bottom row after the copy block', () => {
    expect(webSessionImportDialogSource).toMatch(
      /<div class="web-session-import__copy">[\s\S]*<\/div>\s*<div class="web-session-import__actions">/
    );
    expect(webSessionImportDialogSource).toMatch(
      /\.web-session-import__actions\s*\{[^}]*width:\s*100%;[^}]*justify-content:\s*flex-end;/s
    );
  });

  it('uses a single-column card layout so the content keeps the main width', () => {
    expect(webSessionImportDialogSource).toMatch(
      /\.web-session-import__item\s*\{[^}]*display:\s*flex;[^}]*flex-direction:\s*column;[^}]*align-items:\s*stretch;/s
    );
  });

  it('allows the title row to wrap instead of reserving a fixed action column', () => {
    expect(webSessionImportDialogSource).toMatch(
      /\.web-session-import__title-row\s*\{[^}]*align-items:\s*flex-start;[^}]*flex-wrap:\s*wrap;/s
    );
    expect(webSessionImportDialogSource).toMatch(
      /\.web-session-import__title\s*\{[^}]*flex:\s*1 1 280px;[^}]*min-width:\s*0;/s
    );
  });
});
