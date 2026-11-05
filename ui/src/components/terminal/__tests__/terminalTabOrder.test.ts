import { describe, expect, it } from 'vitest';
import { resolveTerminalTabMove } from '../terminalTabOrder';

describe('terminal tab reorder', () => {
  const allTabs = Array.from({ length: 500 }, (_, i) => ({ id: `t-${i}` }));
  const filtered = allTabs.filter((_, index) => index % 3 === 0);

  it('uses project-wide indexes when dragging across virtual windows in a branch filter', () => {
    expect(resolveTerminalTabMove(allTabs, filtered, 20, 100)).toEqual({
      fromIndex: 60,
      toIndex: 300,
    });
    expect(resolveTerminalTabMove(allTabs, filtered, 100, 20)).toEqual({
      fromIndex: 300,
      toIndex: 60,
    });
  });

  it('does not send empty guide tabs or invalid drops to the server', () => {
    const visible = [...filtered, { id: 'empty-1' }];
    expect(resolveTerminalTabMove(allTabs, visible, 0, visible.length - 1)).toBeNull();
    expect(resolveTerminalTabMove(allTabs, visible, visible.length - 1, 0)).toBeNull();
    expect(resolveTerminalTabMove(allTabs, visible, -1, 10)).toBeNull();
    expect(resolveTerminalTabMove(allTabs, visible, 0, 500)).toBeNull();
    expect(resolveTerminalTabMove(allTabs, visible, 10, 10)).toBeNull();
  });
});
