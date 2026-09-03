export type RuntimeConfigLoadOptions = {
  force?: boolean;
};

type RuntimeConfigFlight<T> = {
  force: boolean;
  promise: Promise<T>;
};

export function createRuntimeConfigRequestLoader<T>(
  ttlMs: number,
  now: () => number = () => Date.now(),
  shouldCache: (value: T) => boolean = () => true
) {
  let cached: { value: T; expiresAt: number } | null = null;
  let flight: RuntimeConfigFlight<T> | null = null;

  function load(
    fetchConfig: (force: boolean) => Promise<T>,
    options: RuntimeConfigLoadOptions = {}
  ): Promise<T> {
    const force = options.force === true;
    if (!force && cached && now() < cached.expiresAt) {
      return Promise.resolve(cached.value);
    }
    if (flight) {
      if (!force || flight.force) {
        return flight.promise;
      }
      return flight.promise.then(
        () => load(fetchConfig, { force: true }),
        () => load(fetchConfig, { force: true })
      );
    }

    const request = Promise.resolve()
      .then(() => fetchConfig(force))
      .then(value => {
        cached = shouldCache(value)
          ? {
              value,
              expiresAt: now() + Math.max(0, ttlMs),
            }
          : null;
        return value;
      })
      .finally(() => {
        if (flight?.promise === request) {
          flight = null;
        }
      });
    flight = { force, promise: request };
    return request;
  }

  return { load };
}
