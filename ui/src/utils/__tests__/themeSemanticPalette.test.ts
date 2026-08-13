import { describe, expect, it } from 'vitest';

import { getPresetById } from '@/constants/themes';
import type { ThemeSettings } from '@/stores/settings';
import { createThemeSemanticPalette } from '@/utils/themeSemanticPalette';

function presetTheme(id: string): ThemeSettings {
  const theme = getPresetById(id)?.colors;
  if (!theme) {
    throw new Error(`Missing theme preset: ${id}`);
  }
  return theme;
}

describe('createThemeSemanticPalette', () => {
  it('keeps the v0.44.1 light workflow and status colors', () => {
    const palette = createThemeSemanticPalette(presetTheme('light'), '#333333');

    expect(palette.colorScheme).toBe('light');
    expect(palette.plan).toBe('#6366f1');
    expect(palette.link).toBe('#255a8f');
    expect(palette.linkHover).toBe('#1f4c7f');
    expect(palette.working).toBe('#8b5cf6');
    expect(palette.completion).toBe('#10b981');
    expect(palette.approval).toBe('#f79009');
    expect(palette.planApproval).toBe('#0891b2');
    expect(palette.redirect).toBe('#2563eb');
    expect(palette.queue).toBe('#4f46e5');
    expect(palette.success).toBe('#52c41a');
    expect(palette.warning).toBe('#fa8c16');
    expect(palette.error).toBe('#f5222d');
    expect(palette.projectTerminal).toBe('#18a058');
    expect(palette.projectTerminalSoft).toBe('#eaf8e3');
    expect(palette.projectWebSession).toBe('#2080f0');
    expect(palette.projectWebSessionSoft).toBe('#e7edf5');
    expect(palette.sidebarFooter).toBe('#FFFFFF');
    expect(palette.changeAddition).toBe('#15803d');
    expect(palette.changeDeletion).toBe('#dc2626');
    expect(palette.changeWarning).toBe('#b45309');
    expect(palette.shadowSubtle).toBe('rgba(15, 23, 42, 0.06)');
    expect(palette.workspaceBackground).toBe(
      'linear-gradient(180deg, rgba(246, 241, 232, 0.78), rgba(255, 255, 255, 0.94))'
    );
  });

  it('preserves the high-contrast dark status colors', () => {
    const palette = createThemeSemanticPalette(presetTheme('dark'), '#D4D4D4');

    expect(palette.colorScheme).toBe('dark');
    expect(palette.success).toBe('#89d185');
    expect(palette.warning).toBe('#cca700');
    expect(palette.error).toBe('#f48771');
    expect(palette.info).toBe('#75beff');
    expect(palette.planApproval).toBe('#4ec9b0');
    expect(palette.projectTerminal).toBe('#89d185');
    expect(palette.projectWebSession).toBe('#75beff');
    expect(palette.sidebarFooter).not.toBe(palette.surface);
  });

  it('applies independent custom workflow and workspace roles', () => {
    const palette = createThemeSemanticPalette(
      {
        ...presetTheme('light'),
        workingColor: '#111111',
        completionColor: '#222222',
        approvalColor: '#333333',
        changeAdditionColor: '#444444',
        workspaceTopColor: '#555555',
        workspaceBottomColor: '#666666',
      },
      '#333333'
    );

    expect(palette.working).toBe('#111111');
    expect(palette.completion).toBe('#222222');
    expect(palette.approval).toBe('#333333');
    expect(palette.changeAddition).toBe('#444444');
    expect(palette.workspaceBackground).toBe(
      'linear-gradient(180deg, rgba(85, 85, 85, 0.78), rgba(102, 102, 102, 0.94))'
    );
  });
});
