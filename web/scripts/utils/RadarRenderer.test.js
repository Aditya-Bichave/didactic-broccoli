import {beforeEach, describe, expect, test, vi} from 'vitest';

vi.mock('./SettingsSync.js', () => ({
    default: {
        getBool: vi.fn((key) => key === 'settingResourceClusters'),
        getFloat: vi.fn((_key, defaultValue = 1) => defaultValue),
        getNumber: vi.fn((_key, defaultValue = 500) => defaultValue)
    }
}));

const {RadarRenderer} = await import('./RadarRenderer.js');

describe('RadarRenderer cluster filtering', () => {
    beforeEach(() => {
        vi.clearAllMocks();
        window.logger = {debug: vi.fn(), info: vi.fn(), warn: vi.fn(), error: vi.fn()};
        window.EnemyType = {
            LivingHarvestable: 0,
            LivingSkinnable: 1
        };
        window.pipManager = {onRadarRendered: vi.fn()};
        Object.defineProperty(document, 'hidden', {
            configurable: true,
            value: false
        });
    });

    test('filters disabled resources out before cluster detection', () => {
        const detectClusters = vi.fn(() => []);
        const renderer = new RadarRenderer({
            handlers: {
                harvestablesHandler: {
                    harvestableList: [
                        {id: 1, stringType: 'Fiber', tier: 4, charges: 1, mobileTypeId: -1}
                    ],
                    shouldDisplayHarvestable: vi.fn(() => false)
                },
                mobsHandler: {
                    mobsList: [
                        {id: 2, type: 0, name: 'Fiber', tier: 4, enchantmentLevel: 2}
                    ],
                    shouldDisplayLivingResource: vi.fn(() => false)
                }
            },
            drawings: {},
            drawingUtils: {
                detectClusters,
                drawClusterRingsFromCluster: vi.fn(),
                drawClusterInfoBox: vi.fn()
            }
        });

        renderer.contexts = {
            mapCanvas: {},
            drawCanvas: {},
            uiCanvas: {}
        };
        renderer.canvasManager = {clearDynamicLayers: vi.fn()};
        renderer.map = {};
        renderer.renderUI = vi.fn();

        renderer.render();

        expect(detectClusters).toHaveBeenCalledWith([], expect.any(Number), expect.any(Number));
    });

    test('uses timeout scheduling when PiP is active and the page is hidden', () => {
        const renderer = new RadarRenderer({
            handlers: {},
            drawings: {},
            drawingUtils: {}
        });

        const timeoutSpy = vi.spyOn(globalThis, 'setTimeout').mockImplementation(() => 123);
        const rafSpy = vi.spyOn(globalThis, 'requestAnimationFrame').mockImplementation(() => 456);

        Object.defineProperty(document, 'hidden', {
            configurable: true,
            value: true
        });
        window.pipManager = {isActive: true};

        renderer.scheduleNextFrame();

        expect(timeoutSpy).toHaveBeenCalled();
        expect(rafSpy).not.toHaveBeenCalled();
        expect(renderer.timeoutId).toBe(123);

        timeoutSpy.mockRestore();
        rafSpy.mockRestore();
    });
});
