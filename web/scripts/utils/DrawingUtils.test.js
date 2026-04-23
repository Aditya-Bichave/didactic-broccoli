import {describe, expect, test, vi} from 'vitest';

vi.mock('./SettingsSync.js', () => ({
    default: {
        getBool: vi.fn(() => false),
        getFloat: vi.fn((_key, defaultValue = 1) => defaultValue),
        getNumber: vi.fn((_key, defaultValue = 500) => defaultValue)
    }
}));

const {DrawingUtils} = await import('./DrawingUtils.js');

describe('DrawingUtils.detectClusters', () => {
    test('keeps living size-zero harvestables in cluster detection', () => {
        const utils = new DrawingUtils();

        const clusters = utils.detectClusters([
            {hX: 0, hY: 0, size: 0, mobileTypeId: 529, name: 'Fiber', tier: 4},
            {hX: 5, hY: 0, size: 0, mobileTypeId: 530, name: 'Fiber', tier: 4}
        ], 30, 2);

        expect(clusters).toHaveLength(1);
        expect(clusters[0].count).toBe(2);
    });

    test('continues skipping depleted static harvestables', () => {
        const utils = new DrawingUtils();

        const clusters = utils.detectClusters([
            {hX: 0, hY: 0, size: 0, mobileTypeId: -1, stringType: 'Fiber', tier: 4},
            {hX: 5, hY: 0, size: 0, mobileTypeId: -1, stringType: 'Fiber', tier: 4}
        ], 30, 2);

        expect(clusters).toHaveLength(0);
    });
});
