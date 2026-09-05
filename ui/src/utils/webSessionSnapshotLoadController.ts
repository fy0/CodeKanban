export type WebSessionSnapshotLoadHandle = {
  token: number;
  controller: AbortController;
  signal: AbortSignal;
};

export function createWebSessionSnapshotLoadController() {
  let currentToken = 0;
  let currentController: AbortController | null = null;
  let currentFlight: { key: string; promise: Promise<boolean> } | null = null;

  function abortCurrent() {
    currentFlight = null;
    if (!currentController) {
      return;
    }
    currentController.abort();
    currentController = null;
  }

  function begin(): WebSessionSnapshotLoadHandle {
    currentToken += 1;
    abortCurrent();
    const controller = new AbortController();
    currentController = controller;
    return {
      token: currentToken,
      controller,
      signal: controller.signal,
    };
  }

  function isCurrent(handle: WebSessionSnapshotLoadHandle) {
    return currentToken === handle.token && currentController === handle.controller;
  }

  function release(handle: WebSessionSnapshotLoadHandle) {
    if (currentController === handle.controller) {
      currentController = null;
      currentFlight = null;
    }
  }

  return {
    begin,

    run(
      key: string,
      load: (handle: WebSessionSnapshotLoadHandle) => Promise<void>
    ): Promise<boolean> {
      if (currentFlight?.key === key) {
        return currentFlight.promise;
      }
      const handle = begin();
      const promise = Promise.resolve()
        .then(async () => {
          if (!isCurrent(handle)) {
            return false;
          }
          await load(handle);
          return isCurrent(handle);
        })
        .catch(error => {
          if (!isCurrent(handle)) {
            return false;
          }
          throw error;
        })
        .finally(() => release(handle));
      currentFlight = { key, promise };
      return promise;
    },

    cancel() {
      currentToken += 1;
      abortCurrent();
    },

    isCurrent,
    release,
  };
}
