import {beforeEach, describe, expect, test, vi} from 'vitest';

vi.mock('../utils/SettingsSync.js', () => ({
    default: {
        getBool: vi.fn(() => false),
        getFloat: vi.fn((_key, defaultValue = 1) => defaultValue),
        getNumber: vi.fn((_key, defaultValue = 500) => defaultValue)
    }
}));

const {MobsDrawing} = await import('./MobsDrawing.js');
const {EnemyType} = await import('../handlers/MobsHandler.js');

describe('MobsDrawing', () => {
    beforeEach(() => {
        window.logger = {debug: vi.fn(), info: vi.fn(), warn: vi.fn(), error: vi.fn()};
        window.mobsHandler = {
            shouldDisplayLivingResource: vi.fn(() => true)
        };
        delete window.handlers;
    });

    test('hides living resources when the resource filter disables them', () => {
        const drawing = new MobsDrawing();
        const ctx = {};

        drawing.DrawCustomImage = vi.fn();
        drawing.drawFilledCircle = vi.fn();
        drawing.transformPoint = vi.fn(() => ({x: 150, y: 90}));

        window.mobsHandler.shouldDisplayLivingResource.mockReturnValue(false);

        drawing.invalidate(ctx, [{
            id: 8403,
            type: EnemyType.LivingHarvestable,
            typeId: 529,
            tier: 4,
            enchantmentLevel: 2,
            name: 'Fiber',
            hX: 12,
            hY: 8,
            getCurrentHP: () => 100,
            maxHealth: 100
        }], []);

        expect(drawing.DrawCustomImage).not.toHaveBeenCalled();
        expect(drawing.drawFilledCircle).not.toHaveBeenCalled();
    });

    test('still draws living resources when the resource filter allows them', () => {
        const drawing = new MobsDrawing();
        const ctx = {};

        drawing.DrawCustomImage = vi.fn();
        drawing.drawFilledCircle = vi.fn();
        drawing.transformPoint = vi.fn(() => ({x: 150, y: 90}));

        drawing.invalidate(ctx, [{
            id: 8403,
            type: EnemyType.LivingHarvestable,
            typeId: 529,
            tier: 4,
            enchantmentLevel: 2,
            name: 'Fiber',
            hX: 12,
            hY: 8,
            getCurrentHP: () => 100,
            maxHealth: 100
        }], []);

        expect(window.mobsHandler.shouldDisplayLivingResource).toHaveBeenCalledWith('Fiber', 4, 2);
        expect(drawing.DrawCustomImage).toHaveBeenCalledWith(ctx, 150, 90, 'fiber_4_2', 'Resources', 40);
    });
});
