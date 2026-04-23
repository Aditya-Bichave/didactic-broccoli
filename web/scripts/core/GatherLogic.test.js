import {describe, expect, test} from 'vitest';

import {
    buildGatherQueue,
    createDefaultGatherSettings,
    normalizeHarvestable,
    reconcileStatuses
} from './GatherLogic.js';

describe('GatherLogic', () => {
    test('filters live harvestables through gather whitelist', () => {
        const settings = createDefaultGatherSettings();
        settings.whitelist.types.Fiber = false;
        settings.whitelist.living = false;

        const queue = buildGatherQueue([
            {id: 1, stringType: 'Fiber', tier: 4, charges: 0, size: 3, posX: 10, posY: 10, mobileTypeId: -1},
            {id: 2, stringType: 'Ore', tier: 5, charges: 2, size: 2, posX: 15, posY: 10, mobileTypeId: -1},
            {id: 3, stringType: 'Ore', tier: 5, charges: 2, size: 2, posX: 20, posY: 10, mobileTypeId: 531}
        ], {x: 0, y: 0}, settings);

        expect(queue).toHaveLength(1);
        expect(queue[0].id).toBe(2);
    });

    test('orders queue nearest-first', () => {
        const settings = createDefaultGatherSettings();

        const queue = buildGatherQueue([
            {id: 11, stringType: 'Rock', tier: 4, charges: 0, size: 1, posX: 50, posY: 0, mobileTypeId: -1},
            {id: 12, stringType: 'Rock', tier: 4, charges: 0, size: 1, posX: 20, posY: 0, mobileTypeId: -1},
            {id: 13, stringType: 'Rock', tier: 4, charges: 0, size: 1, posX: 30, posY: 0, mobileTypeId: -1}
        ], {x: 0, y: 0}, settings);

        expect(queue.map(node => node.id)).toEqual([12, 13, 11]);
    });

    test('queue refresh drops depleted nodes and keeps new ones', () => {
        const settings = createDefaultGatherSettings();
        const statusMap = new Map();
        const firstQueue = buildGatherQueue([
            {id: 21, stringType: 'Hide', tier: 4, charges: 1, size: 2, posX: 10, posY: 0, mobileTypeId: -1}
        ], {x: 0, y: 0}, settings, statusMap);

        const secondQueue = buildGatherQueue([
            {id: 22, stringType: 'Hide', tier: 4, charges: 1, size: 2, posX: 12, posY: 0, mobileTypeId: -1},
            {id: 21, stringType: 'Hide', tier: 4, charges: 1, size: 0, posX: 10, posY: 0, mobileTypeId: -1}
        ], {x: 0, y: 0}, settings, statusMap);

        expect(firstQueue.map(node => node.id)).toEqual([21]);
        expect(secondQueue.map(node => node.id)).toEqual([22]);
    });

    test('keeps living harvestables with zero initial size in the queue', () => {
        const settings = createDefaultGatherSettings();

        const queue = buildGatherQueue([
            {id: 31, stringType: 'Fiber', tier: 4, charges: 0, size: 0, posX: 8, posY: 0, mobileTypeId: 529}
        ], {x: 0, y: 0}, settings);

        expect(queue).toHaveLength(1);
        expect(queue[0].id).toBe(31);
        expect(queue[0].isLiving).toBe(true);
    });

    test('status reconciliation marks vanished current target as done', () => {
        const statusMap = new Map();
        const previousQueue = [{id: 44, status: 'interacting'}];
        const nextQueue = [{id: 45, status: 'candidate'}];

        reconcileStatuses(previousQueue, nextQueue, statusMap, 44);

        expect(statusMap.get(44)).toBe('done');
        expect(statusMap.get(45)).toBe('candidate');
    });

    test('normalizeHarvestable preserves last update timestamp when present', () => {
        const node = normalizeHarvestable({
            id: 77,
            stringType: 'Ore',
            tier: 6,
            charges: 1,
            size: 3,
            posX: 10,
            posY: 20,
            mobileTypeId: -1,
            lastUpdateTime: 123456
        });

        expect(node.lastSeenAt).toBe(123456);
    });
});
