/** Translate a drag in the filtered/virtual strip to the persisted project order. */
export function resolveTerminalTabMove(
  allTabs: readonly { id: string }[],
  visibleTabs: readonly { id: string }[],
  oldIndex: number,
  newIndex: number
): { fromIndex: number; toIndex: number } | null {
  if (oldIndex === newIndex) return null;
  const from = visibleTabs[oldIndex];
  const to = visibleTabs[newIndex];
  if (!from || !to) return null;
  const fromIndex = allTabs.findIndex(tab => tab.id === from.id);
  const toIndex = allTabs.findIndex(tab => tab.id === to.id);
  // Empty guide tabs do not participate in the server's terminal ordering.
  return fromIndex >= 0 && toIndex >= 0 ? { fromIndex, toIndex } : null;
}
