export function createWebSessionCatchUpScheduler(run: (reason: string) => void, delayMs = 150) {
  let timer: ReturnType<typeof setTimeout> | null = null;

  return {
    schedule(reason: string) {
      if (timer != null) {
        clearTimeout(timer);
      }
      timer = setTimeout(
        () => {
          timer = null;
          run(reason);
        },
        Math.max(0, delayMs)
      );
    },

    cancel() {
      if (timer != null) {
        clearTimeout(timer);
        timer = null;
      }
    },
  };
}
