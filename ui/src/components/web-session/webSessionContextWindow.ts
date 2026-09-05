export function contextWindowOptions(defaultLabel: string) {
  return [
    { label: defaultLabel, value: 0 },
    { label: '512K', value: 512000 },
    { label: '768K', value: 768000 },
    { label: '1M', value: 1000000 },
  ];
}

export function contextWindowPending(setting: number, applied: number | null | undefined) {
  return applied == null ? setting !== 0 : setting !== applied;
}
