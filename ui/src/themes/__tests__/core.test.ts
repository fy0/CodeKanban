import { describe, it } from 'vitest';
import builtin from '../builtin.json';
import { themeContractCases } from './contracts';

describe('appearance data contracts', () => {
  for (const test of themeContractCases(builtin)) it(test.name, test.run);
});
