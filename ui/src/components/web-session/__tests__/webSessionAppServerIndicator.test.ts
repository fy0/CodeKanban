import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const panelPath = fileURLToPath(new URL('../WebSessionPanel.vue', import.meta.url));
const composerStylePath = fileURLToPath(
  new URL('../styles/webSessionPanelComposer.css', import.meta.url)
);

describe('webSession App Server indicator', () => {
  it('is compact, backend-specific, and exposes termination only for actionable states', () => {
    const source = readFileSync(panelPath, 'utf8');
    const styleSource = readFileSync(composerStylePath, 'utf8');

    expect(source).toContain('v-if="isCurrentCodexAppServerSession"');
    expect(source).toContain("session.backend === 'codex_app_server'");
    expect(source).toContain(
      "runtimeState.state === 'active' || runtimeState.state === 'draining'"
    );
    expect(source).toContain('v-if="canForceTerminateCodexAppServer"');
    expect(source).toContain('error instanceof ApiError && error.status === 409');
    expect(source).toContain("sameRun && currentRuntime.state !== 'inactive'");
    expect(source).not.toContain('class="composer-settings-danger-zone"');
    expect(styleSource).toMatch(
      /\.composer-app-server-trigger\s*\{[^}]*width:\s*24px;[^}]*height:\s*24px;/s
    );
  });
});
