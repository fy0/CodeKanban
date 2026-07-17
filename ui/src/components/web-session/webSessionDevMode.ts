export function isWebSessionDevMode(query: Readonly<Record<string, unknown>>): boolean {
  return Object.prototype.hasOwnProperty.call(query, 'DEV');
}

export function shouldShowCyberPolicyWarning(
  state: Readonly<{
    sessionFlagged: boolean | null | undefined;
    sessionDismissed: boolean;
    devMode: boolean;
    simulatedWarning: boolean;
  }>
): boolean {
  return (
    (state.sessionFlagged === true && !state.sessionDismissed) ||
    (state.devMode && state.simulatedWarning)
  );
}
