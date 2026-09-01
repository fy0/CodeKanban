export const WEB_SESSION_HISTORY_AUTO_LOAD_LIMIT = 3;

export interface WebSessionHistoryAutoLoadBudget {
  tryConsume(): boolean;
  reset(): void;
  remaining(): number;
}

export function createWebSessionHistoryAutoLoadBudget(): WebSessionHistoryAutoLoadBudget {
  let consumed = 0;

  return {
    tryConsume() {
      if (consumed >= WEB_SESSION_HISTORY_AUTO_LOAD_LIMIT) {
        return false;
      }
      consumed += 1;
      return true;
    },
    reset() {
      consumed = 0;
    },
    remaining() {
      return Math.max(0, WEB_SESSION_HISTORY_AUTO_LOAD_LIMIT - consumed);
    },
  };
}
