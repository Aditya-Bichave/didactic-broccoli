import {afterEach, beforeEach, describe, expect, test, vi} from 'vitest';

vi.mock('../utils/SettingsSync.js', () => ({
    default: {
        getJSON: vi.fn(() => null),
        setJSON: vi.fn()
    }
}));

vi.mock('./EventRouter.js', () => ({
    getLocalPlayerPosition: vi.fn(() => ({x: 0, y: 0}))
}));

const statusResponse = {
    supported: true,
    platform: 'test',
    active: false,
    state: 'idle',
    calibrated: false,
    focusedWindow: false,
    consecutiveErrors: 0
};

const {GatherController} = await import('./GatherController.js');

describe('GatherController', () => {
    beforeEach(() => {
        vi.useFakeTimers();
        vi.setSystemTime(new Date('2026-04-24T01:00:00Z'));
        document.body.innerHTML = `
            <div id="gather-panel"></div>
            <button id="gatherModeToggle"></button>
            <button id="gatherStartStop"></button>
            <button id="gatherCalibrate"></button>
            <input id="gatherHotkeyEnabled" type="checkbox">
            <input id="gatherAllowStatic" type="checkbox">
            <input id="gatherAllowLiving" type="checkbox">
            <input id="gatherCenterOffsetX" type="number">
            <input id="gatherCenterOffsetY" type="number">
            <input id="gatherSafeMarginX" type="number">
            <input id="gatherSafeMarginY" type="number">
            <input id="gatherMoveRadiusPx" type="number">
            <input id="gatherInteractRadiusPx" type="number">
            <input id="gatherInteractThresholdGu" type="number">
            <input id="gatherRetryLimit" type="number">
            <div id="gatherTypeFilters"></div>
            <div id="gatherTierFilters"></div>
            <div id="gatherEnchantFilters"></div>
            <div id="gatherModeBadge"></div>
            <div id="gatherCalibrationState"></div>
            <div id="gatherFocusedState"></div>
            <div id="gatherQueueCount"></div>
            <div id="gatherCurrentTarget"></div>
            <div id="gatherRunState"></div>
            <div id="gatherThreatState"></div>
            <div id="gatherStatusNote"></div>
            <div id="gatherQueueList"></div>
            <div id="gatherLogList"></div>
        `;

        window.logger = {
            info: vi.fn(),
            debug: vi.fn(),
            warn: vi.fn(),
            error: vi.fn()
        };
        window.harvestablesHandler = {
            getHarvestableList: vi.fn(() => [])
        };
        window.playersHandler = {
            getFilteredPlayers: vi.fn(() => []),
            localPlayer: { mounted: false }
        };
        globalThis.fetch = vi.fn(() => Promise.resolve({
            ok: true,
            json: () => Promise.resolve(statusResponse)
        }));
    });

    afterEach(() => {
        vi.useRealTimers();
    });

    test('Ctrl+Shift+G toggles gather mode badge', async () => {
        const controller = new GatherController();
        await controller.init();

        document.dispatchEvent(new KeyboardEvent('keydown', {
            key: 'g',
            ctrlKey: true,
            shiftKey: true
        }));

        expect(document.getElementById('gatherModeBadge').textContent).toBe('Mode Enabled');

        controller.destroy();
    });

    test('panel state reflects rebuilt queue and current target', async () => {
        window.harvestablesHandler.getHarvestableList.mockReturnValue([
            {id: 81, stringType: 'Ore', tier: 6, charges: 2, size: 2, posX: 12, posY: 0, mobileTypeId: -1}
        ]);

        const controller = new GatherController();
        await controller.init();
        controller.rebuildQueue();

        expect(document.getElementById('gatherQueueCount').textContent).toBe('1');
        expect(document.getElementById('gatherCurrentTarget').textContent).toContain('Ore T6.2');

        controller.destroy();
    });

    test('start run auto-calibrates before toggling gather mode', async () => {
        globalThis.fetch = vi.fn()
            .mockResolvedValueOnce({
                ok: true,
                json: () => Promise.resolve(statusResponse)
            })
            .mockResolvedValueOnce({
                ok: true,
                json: () => Promise.resolve({
                    status: {
                        ...statusResponse,
                        calibrated: true,
                        calibration: {
                            centerOffsetX: 0,
                            centerOffsetY: 0,
                            safeMarginX: 24,
                            safeMarginY: 24,
                            moveRadiusPx: 180,
                            interactRadiusPx: 80,
                            interactThresholdGu: 14,
                            windowRect: {left: 10, top: 10, right: 100, bottom: 100}
                        }
                    }
                })
            })
            .mockResolvedValueOnce({
                ok: true,
                json: () => Promise.resolve({
                    status: {
                        ...statusResponse,
                        active: true,
                        calibrated: true,
                        state: 'running',
                        focusedWindow: true
                    }
                })
            });

        const controller = new GatherController();
        await controller.init();
        await controller.startGather();

        expect(globalThis.fetch).toHaveBeenNthCalledWith(2, '/api/gather/calibrate', expect.any(Object));
        expect(globalThis.fetch).toHaveBeenNthCalledWith(3, '/api/gather/toggle', expect.any(Object));
        expect(document.getElementById('gatherRunState').textContent).toBe('running');

        controller.destroy();
    });

    test('successful movement ticks do not burn retry budget', async () => {
        window.harvestablesHandler.getHarvestableList.mockReturnValue([
            {id: 91, stringType: 'Ore', tier: 5, charges: 0, size: 2, posX: 40, posY: 0, mobileTypeId: -1}
        ]);

        globalThis.fetch = vi.fn((url) => {
            if (url === '/api/gather/status') {
                return Promise.resolve({
                    ok: true,
                    json: () => Promise.resolve(statusResponse)
                });
            }

            if (url === '/api/gather/target') {
                return Promise.resolve({
                    ok: true,
                    json: () => Promise.resolve({
                        status: {
                            ...statusResponse,
                            active: true,
                            calibrated: true,
                            state: 'running'
                        },
                        step: {
                            mode: 'moving'
                        }
                    })
                });
            }

            throw new Error(`Unexpected fetch ${url}`);
        });

        const controller = new GatherController();
        await controller.init();
        controller.settings.modeEnabled = true;
        controller.settings.retryLimit = 1;
        controller.backendStatus = {...statusResponse, active: true, calibrated: true, state: 'running'};

        await controller.tick();
        await controller.tick();

        expect(controller.statusMap.get(91)).toBe('moving');
        expect(controller.queue.find(node => node.id === 91)).toBeTruthy();

        controller.destroy();
    });

    test('stalled travel switches to a recovery target', async () => {
        window.harvestablesHandler.getHarvestableList.mockReturnValue([
            {id: 92, stringType: 'Ore', tier: 5, charges: 0, size: 2, posX: 40, posY: 0, mobileTypeId: -1}
        ]);

        const targetBodies = [];
        globalThis.fetch = vi.fn((url, options) => {
            if (url === '/api/gather/status') {
                return Promise.resolve({
                    ok: true,
                    json: () => Promise.resolve(statusResponse)
                });
            }

            if (url === '/api/gather/target') {
                targetBodies.push(JSON.parse(options.body));
                return Promise.resolve({
                    ok: true,
                    json: () => Promise.resolve({
                        status: {
                            ...statusResponse,
                            active: true,
                            calibrated: true,
                            state: 'running'
                        },
                        step: {
                            mode: 'moving'
                        }
                    })
                });
            }

            throw new Error(`Unexpected fetch ${url}`);
        });

        const controller = new GatherController();
        await controller.init();
        controller.intervals.forEach(clearInterval);
        controller.intervals = [];
        controller.settings.modeEnabled = true;
        controller.backendStatus = {...statusResponse, active: true, calibrated: true, state: 'running'};

        for (let i = 0; i < 10; i += 1) {
            await controller.tick();
            vi.advanceTimersByTime(400);
        }

        const recoveryBody = targetBodies.find(body =>
            body.target.position.x !== 40 || body.target.position.y !== 0
        );
        expect(recoveryBody).toBeTruthy();

        controller.destroy();
    });

    test('travel does not recover too early while time window has not elapsed', async () => {
        window.harvestablesHandler.getHarvestableList.mockReturnValue([
            {id: 93, stringType: 'Ore', tier: 5, charges: 0, size: 2, posX: 40, posY: 0, mobileTypeId: -1}
        ]);

        const targetBodies = [];
        globalThis.fetch = vi.fn((url, options) => {
            if (url === '/api/gather/status') {
                return Promise.resolve({
                    ok: true,
                    json: () => Promise.resolve(statusResponse)
                });
            }

            if (url === '/api/gather/target') {
                targetBodies.push(JSON.parse(options.body));
                return Promise.resolve({
                    ok: true,
                    json: () => Promise.resolve({
                        status: {
                            ...statusResponse,
                            active: true,
                            calibrated: true,
                            state: 'running'
                        },
                        step: {
                            mode: 'moving'
                        }
                    })
                });
            }

            throw new Error(`Unexpected fetch ${url}`);
        });

        const controller = new GatherController();
        await controller.init();
        controller.intervals.forEach(clearInterval);
        controller.intervals = [];
        controller.settings.modeEnabled = true;
        controller.backendStatus = {...statusResponse, active: true, calibrated: true, state: 'running'};

        for (let i = 0; i < 4; i += 1) {
            await controller.tick();
            vi.advanceTimersByTime(300);
        }

        expect(targetBodies.every(body => body.target.position.x === 40 && body.target.position.y === 0)).toBe(true);

        controller.destroy();
    });

    test('stale close target is skipped after repeated interact attempts', async () => {
        window.harvestablesHandler.getHarvestableList.mockReturnValue([
            {
                id: 94,
                stringType: 'Ore',
                tier: 5,
                charges: 0,
                size: 2,
                posX: 4,
                posY: 0,
                mobileTypeId: -1,
                lastUpdateTime: Date.now() - 6000
            },
            {
                id: 95,
                stringType: 'Ore',
                tier: 5,
                charges: 0,
                size: 2,
                posX: 12,
                posY: 0,
                mobileTypeId: -1,
                lastUpdateTime: Date.now()
            }
        ]);

        globalThis.fetch = vi.fn((url) => {
            if (url === '/api/gather/status') {
                return Promise.resolve({
                    ok: true,
                    json: () => Promise.resolve(statusResponse)
                });
            }

            if (url === '/api/gather/target') {
                return Promise.resolve({
                    ok: true,
                    json: () => Promise.resolve({
                        status: {
                            ...statusResponse,
                            active: true,
                            calibrated: true,
                            state: 'running'
                        },
                        step: {
                            mode: 'interacting'
                        }
                    })
                });
            }

            throw new Error(`Unexpected fetch ${url}`);
        });

        const controller = new GatherController();
        await controller.init();
        controller.intervals.forEach(clearInterval);
        controller.intervals = [];
        controller.settings.modeEnabled = true;
        controller.backendStatus = {...statusResponse, active: true, calibrated: true, state: 'running'};

        for (let i = 0; i < 6; i += 1) {
            await controller.tick();
            vi.advanceTimersByTime(300);
        }

        expect(controller.statusMap.get(94)).toBe('failed');
        expect(controller.queue.find(node => node.id === 94)).toBeFalsy();

        controller.destroy();
    });

    test('disappeared current target is dropped immediately and next resource is requested', async () => {
        const harvestables = [
            {id: 96, stringType: 'Ore', tier: 5, charges: 0, size: 2, posX: 4, posY: 0, mobileTypeId: -1},
            {id: 97, stringType: 'Ore', tier: 5, charges: 0, size: 2, posX: 12, posY: 0, mobileTypeId: -1}
        ];
        window.harvestablesHandler.getHarvestableList.mockImplementation(() => [...harvestables]);

        const requestedTargetIds = [];
        globalThis.fetch = vi.fn((url, options) => {
            if (url === '/api/gather/status') {
                return Promise.resolve({
                    ok: true,
                    json: () => Promise.resolve(statusResponse)
                });
            }

            if (url === '/api/gather/target') {
                requestedTargetIds.push(JSON.parse(options.body).target.id);
                return Promise.resolve({
                    ok: true,
                    json: () => Promise.resolve({
                        status: {
                            ...statusResponse,
                            active: true,
                            calibrated: true,
                            state: 'running'
                        },
                        step: {
                            mode: 'interacting'
                        }
                    })
                });
            }

            throw new Error(`Unexpected fetch ${url}`);
        });

        const controller = new GatherController();
        await controller.init();
        controller.intervals.forEach(clearInterval);
        controller.intervals = [];
        controller.settings.modeEnabled = true;
        controller.backendStatus = {...statusResponse, active: true, calibrated: true, state: 'running'};

        await controller.tick();
        harvestables.shift();
        await controller.tick();

        expect(requestedTargetIds).toEqual([96, 97]);
        expect(controller.statusMap.get(96)).toBe('done');
        expect(controller.currentTargetId).toBe(97);

        controller.destroy();
    });
});
