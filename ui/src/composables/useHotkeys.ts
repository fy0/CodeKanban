import { onMounted, onUnmounted } from 'vue';

export interface Hotkey {
  key: string;
  ctrl?: boolean;
  alt?: boolean;
  shift?: boolean;
  handler: () => void;
  allowInInputs?: boolean;
}

export function useHotkeys(hotkeys: Hotkey[]) {
  const handleKeyDown = (event: KeyboardEvent) => {
    const target = event.target as HTMLElement | null;
    const isFormElement =
      target &&
      ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName) &&
      !(target as HTMLInputElement).readOnly;

    for (const hotkey of hotkeys) {
      if (
        event.key.toLowerCase() === hotkey.key.toLowerCase() &&
        (!!event.ctrlKey === !!hotkey.ctrl) &&
        (!!event.altKey === !!hotkey.alt) &&
        (!!event.shiftKey === !!hotkey.shift) &&
        (!isFormElement || hotkey.allowInInputs)
      ) {
        event.preventDefault();
        hotkey.handler();
        break;
      }
    }
  };

  onMounted(() => {
    window.addEventListener('keydown', handleKeyDown);
  });

  onUnmounted(() => {
    window.removeEventListener('keydown', handleKeyDown);
  });
}
