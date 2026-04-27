import {beforeEach, describe, expect, test, vi} from 'vitest';

const {WebSocketEventQueue} = await import('./WebSocketEventQueue.js');

describe('WebSocketEventQueue scheduling', () => {
    beforeEach(() => {
        vi.clearAllMocks();
        window.logger = {debug: vi.fn(), info: vi.fn(), warn: vi.fn(), error: vi.fn()};
        window.settingsSync = {
            getBool: vi.fn((_key, defaultValue = true) => defaultValue)
        };
        window.pipManager = {isActive: false};
        Object.defineProperty(document, 'hidden', {
            configurable: true,
            value: false
        });
    });

    test('uses timeout scheduling when PiP is active and the page is hidden', () => {
        const queue = new WebSocketEventQueue();
        const timeoutSpy = vi.spyOn(globalThis, 'setTimeout').mockImplementation(() => 321);
        const rafSpy = vi.spyOn(globalThis, 'requestAnimationFrame').mockImplementation(() => 654);

        Object.defineProperty(document, 'hidden', {
            configurable: true,
            value: true
        });
        window.pipManager = {isActive: true};

        queue.scheduleFlush();

        expect(timeoutSpy).toHaveBeenCalled();
        expect(rafSpy).not.toHaveBeenCalled();
        expect(queue.timeoutId).toBe(321);

        timeoutSpy.mockRestore();
        rafSpy.mockRestore();
        queue.destroy();
    });

    test('uses requestAnimationFrame when the page is visible', () => {
        const queue = new WebSocketEventQueue();
        const timeoutSpy = vi.spyOn(globalThis, 'setTimeout').mockImplementation(() => 321);
        const rafSpy = vi.spyOn(globalThis, 'requestAnimationFrame').mockImplementation(() => 654);

        queue.scheduleFlush();

        expect(rafSpy).toHaveBeenCalled();
        expect(timeoutSpy).not.toHaveBeenCalled();
        expect(queue.rafId).toBe(654);

        timeoutSpy.mockRestore();
        rafSpy.mockRestore();
        queue.destroy();
    });
});
