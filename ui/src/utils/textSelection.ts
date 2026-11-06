export function preventClickAfterTextSelection(event: MouseEvent) {
  const target = event.currentTarget;
  if (event.detail === 0 || !(target instanceof Element)) return;

  const selection = target.ownerDocument.getSelection();
  if (!selection || selection.isCollapsed || !target.contains(selection.anchorNode)) return;

  // Dragging selectable option text can also generate a click. Keep copying
  // from changing the answer, while allowing ordinary and keyboard clicks.
  event.preventDefault();
  event.stopPropagation();
}
