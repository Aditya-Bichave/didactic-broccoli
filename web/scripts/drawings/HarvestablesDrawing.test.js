import {beforeEach, describe, expect, test, vi} from 'vitest';

vi.mock('../utils/SettingsSync.js', () => ({
    default: {
        getBool: vi.fn(() => false),
        getFloat: vi.fn((_key, defaultValue = 1) => defaultValue),
        getNumber: vi.fn((_key, defaultValue = 500) => defaultValue)
    }
}));

const {HarvestablesDrawing} = await import('./HarvestablesDrawing.js');

describe('HarvestablesDrawing', () => {
    beforeEach(() => {
        window.logger = {debug: vi.fn(), info: vi.fn(), warn: vi.fn(), error: vi.fn()};
    });

    test('renders living harvestables even when they spawn with size zero', () => {
        const drawing = new HarvestablesDrawing();
        const ctx = {};

        drawing.DrawCustomImage = vi.fn();
        drawing.transformPoint = vi.fn(() => ({x: 120, y: 80}));

        drawing.invalidate(ctx, [{
            id: 8403,
            type: 16,
            tier: 4,
            charges: 0,
            size: 0,
            stringType: 'Fiber',
            mobileTypeId: 529,
            hX: 12,
            hY: 8
        }]);

        expect(drawing.DrawCustomImage).toHaveBeenCalledWith(ctx, 120, 80, 'fiber_4_0', 'Resources', 40);
    });

    test('still skips static zero-size harvestables', () => {
        const drawing = new HarvestablesDrawing();
        const ctx = {};

        drawing.DrawCustomImage = vi.fn();
        drawing.transformPoint = vi.fn(() => ({x: 120, y: 80}));

        drawing.invalidate(ctx, [{
            id: 2246,
            type: 14,
            tier: 5,
            charges: 1,
            size: 0,
            stringType: 'Fiber',
            mobileTypeId: -1,
            hX: 12,
            hY: 8
        }]);

        expect(drawing.DrawCustomImage).not.toHaveBeenCalled();
    });
});
