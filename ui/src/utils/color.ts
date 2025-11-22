const HEX_PATTERN = /^#?[0-9a-f]{3}([0-9a-f]{3})?$/i;

function normalizeHex(hex: string): string | null {
  if (!HEX_PATTERN.test(hex)) {
    return null;
  }
  let normalized = hex.startsWith('#') ? hex.slice(1) : hex;
  if (normalized.length === 3) {
    normalized = normalized
      .split('')
      .map(char => char + char)
      .join('');
  }
  return normalized.toLowerCase();
}

function clamp(value: number) {
  return Math.max(0, Math.min(255, value));
}

function toHex(value: number) {
  return value.toString(16).padStart(2, '0');
}

function adjustColor(hex: string, ratio: number): string {
  const normalized = normalizeHex(hex);
  if (!normalized) {
    return hex;
  }
  const r = parseInt(normalized.slice(0, 2), 16);
  const g = parseInt(normalized.slice(2, 4), 16);
  const b = parseInt(normalized.slice(4, 6), 16);

  const amount = Math.round(255 * ratio);

  const nextR = clamp(r + amount);
  const nextG = clamp(g + amount);
  const nextB = clamp(b + amount);

  return `#${toHex(nextR)}${toHex(nextG)}${toHex(nextB)}`;
}

export function lightenColor(hex: string, ratio = 0.1): string {
  return adjustColor(hex, Math.abs(ratio));
}

export function darkenColor(hex: string, ratio = 0.1): string {
  return adjustColor(hex, -Math.abs(ratio));
}

// 保证返回标准的 #rrggbb 形式，不合法时返回 fallback
export function ensureHexWithHash(hex: string, fallback = '#000000'): string {
  const normalized = normalizeHex(hex);
  if (!normalized) {
    return fallback;
  }
  return `#${normalized}`;
}

/**
 * 使用感知亮度（Relative Luminance）判断颜色是否为暗色
 * 基于 sRGB 颜色空间的人眼感知权重
 * @param hex 十六进制颜色值
 * @returns 如果颜色较暗返回 true，否则返回 false
 */
export function isDarkHex(hex: string): boolean {
  const normalized = normalizeHex(hex);
  if (!normalized) {
    return false;
  }

  const r = parseInt(normalized.slice(0, 2), 16);
  const g = parseInt(normalized.slice(2, 4), 16);
  const b = parseInt(normalized.slice(4, 6), 16);

  // 使用 sRGB 感知亮度公式（人眼对绿色最敏感，红色次之，蓝色最弱）
  // 参考: https://www.w3.org/TR/WCAG20/#relativeluminancedef
  const luminance = (0.299 * r + 0.587 * g + 0.114 * b) / 255;

  // 亮度低于 0.5 判定为暗色
  return luminance < 0.5;
}

export function getReadableTextColor(hex: string): string {
  return isDarkHex(hex) ? '#FFFFFFD9' : '#000000E0';
}

export function hexToRgba(hex: string, alpha = 1): string {
  const normalized = normalizeHex(hex);
  if (!normalized) {
    // 与终端默认背景 (#0f111a) 保持一致的降级方案
    return `rgba(15, 17, 26, ${alpha})`;
  }
  const r = parseInt(normalized.slice(0, 2), 16);
  const g = parseInt(normalized.slice(2, 4), 16);
  const b = parseInt(normalized.slice(4, 6), 16);
  return `rgba(${r}, ${g}, ${b}, ${alpha})`;
}
