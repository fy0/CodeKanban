export function formatWebSessionTokenCount(value: number, exact = false) {
  const normalized = Math.max(0, Math.round(Number(value) || 0));
  if (exact) {
    return new Intl.NumberFormat().format(normalized);
  }
  if (normalized < 1000) {
    return String(normalized);
  }
  const units = [
    { threshold: 1_000_000_000, suffix: 'B' },
    { threshold: 1_000_000, suffix: 'M' },
    { threshold: 1_000, suffix: 'K' },
  ];
  const unit = units.find(candidate => normalized >= candidate.threshold);
  if (!unit) {
    return String(normalized);
  }
  return (normalized / unit.threshold).toFixed(1).replace(/\.0$/, '') + unit.suffix;
}
