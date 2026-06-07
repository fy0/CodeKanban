import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const filePreviewSurfacePath = fileURLToPath(new URL('../FilePreviewSurface.vue', import.meta.url));
const filePreviewSurfaceSource = readFileSync(filePreviewSurfacePath, 'utf8');

describe('FilePreviewSurface styles', () => {
  it('keeps long diff lines horizontally scrollable instead of clipping them', () => {
    expect(filePreviewSurfaceSource).toMatch(
      /\.file-preview-content\s*\{[^}]*min-width:\s*0;[^}]*overflow:\s*auto;/s
    );
    expect(filePreviewSurfaceSource).toMatch(
      /\.file-preview-diff\s+:deep\(pre\.markdown-code-block\)\s*\{[^}]*max-width:\s*100%;[^}]*overflow-x:\s*auto;/s
    );
    expect(filePreviewSurfaceSource).toMatch(
      /\.file-preview-diff\s+:deep\(pre\.markdown-code-block code\.hljs\)\s*\{[^}]*width:\s*max-content;[^}]*min-width:\s*100%;[^}]*white-space:\s*pre;/s
    );
  });
});
