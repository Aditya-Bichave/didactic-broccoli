export const RESOURCE_TYPES = ['Fiber', 'Hide', 'Log', 'Ore', 'Rock'];
export const TIER_VALUES = [1, 2, 3, 4, 5, 6, 7, 8];
export const ENCHANT_VALUES = [0, 1, 2, 3, 4];

export function createDefaultGatherSettings() {
    return {
        modeEnabled: false,
        hotkeyEnabled: true,
        whitelist: {
            static: true,
            living: true,
            types: Object.fromEntries(RESOURCE_TYPES.map(type => [type, true])),
            tiers: Object.fromEntries(TIER_VALUES.map(tier => [tier, true])),
            enchants: Object.fromEntries(ENCHANT_VALUES.map(enchant => [enchant, true])),
        },
        calibration: {
            centerOffsetX: 0,
            centerOffsetY: 0,
            safeMarginX: 24,
            safeMarginY: 24,
            moveRadiusPx: 220,
            interactRadiusPx: 80,
            interactThresholdGu: 14,
            desiredMount: true,
            mountKey: 'A',
            mountDelayMs: 3500,
            dismountDelayMs: 1500,
            clickRefreshMs: 1400,
            clickDriftPx: 35,
        },
        retryLimit: 6
    };
}

export function normalizeGatherSettings(input = {}) {
    input = input || {};
    const defaults = createDefaultGatherSettings();
    return {
        ...defaults,
        ...input,
        whitelist: {
            ...defaults.whitelist,
            ...(input.whitelist || {}),
            types: {
                ...defaults.whitelist.types,
                ...((input.whitelist && input.whitelist.types) || {})
            },
            tiers: {
                ...defaults.whitelist.tiers,
                ...((input.whitelist && input.whitelist.tiers) || {})
            },
            enchants: {
                ...defaults.whitelist.enchants,
                ...((input.whitelist && input.whitelist.enchants) || {})
            }
        },
        calibration: {
            ...defaults.calibration,
            ...(input.calibration || {})
        }
    };
}

export function normalizeHarvestable(harvestable, now = Date.now()) {
    const type = harvestable.stringType || harvestable.type || '';
    const lastSeenAt = Number(harvestable.lastUpdateTime || now);
    const isLiving = harvestable.mobileTypeId !== null &&
        harvestable.mobileTypeId !== undefined &&
        harvestable.mobileTypeId !== 65535 &&
        harvestable.mobileTypeId !== -1;
    return {
        id: Number(harvestable.id),
        type,
        tier: Number(harvestable.tier || 0),
        enchant: Number(harvestable.charges || 0),
        size: Number(harvestable.size ?? 0),
        isLiving,
        position: {
            x: Number(harvestable.posX || 0),
            y: Number(harvestable.posY || 0)
        },
        distance: 0,
        status: 'candidate',
        lastSeenAt,
    };
}

export function calculateDistance(from, to) {
    const dx = Number(to.x || 0) - Number(from.x || 0);
    const dy = Number(to.y || 0) - Number(from.y || 0);
    return Math.hypot(dx, dy);
}

export function matchesWhitelist(node, whitelist) {
    if (!node || !node.type) return false;
    if (!node.isLiving && node.size <= 0) return false;
    if (node.isLiving && node.size < 0) return false;
    if (node.isLiving && whitelist.living !== true) return false;
    if (!node.isLiving && whitelist.static !== true) return false;
    if (whitelist.types?.[node.type] !== true) return false;
    if (whitelist.tiers?.[node.tier] !== true) return false;
    if (whitelist.enchants?.[node.enchant] !== true) return false;
    return true;
}

export function buildGatherQueue(harvestables, playerPosition, settings, statusMap = new Map(), now = Date.now()) {
    const normalizedSettings = normalizeGatherSettings(settings);
    const queue = [];

    for (const harvestable of harvestables || []) {
        const node = normalizeHarvestable(harvestable, now);
        if (!matchesWhitelist(node, normalizedSettings.whitelist)) continue;

        node.distance = calculateDistance(playerPosition, node.position);
        node.status = statusMap.get(node.id) || 'candidate';
        queue.push(node);
    }

    queue.sort((left, right) => {
        if (left.distance !== right.distance) return left.distance - right.distance;
        if (left.tier !== right.tier) return right.tier - left.tier;
        return left.id - right.id;
    });

    return queue;
}

export function reconcileStatuses(previousQueue, nextQueue, statusMap, previousCurrentTargetId = null) {
    const nextIds = new Set(nextQueue.map(node => node.id));

    for (const previous of previousQueue || []) {
        if (!nextIds.has(previous.id) && previous.id === previousCurrentTargetId) {
            statusMap.set(previous.id, 'done');
        }
    }

    for (const node of nextQueue) {
        if (!statusMap.has(node.id) || statusMap.get(node.id) === 'done') {
            statusMap.set(node.id, 'candidate');
        }
    }

    return statusMap;
}

export function formatTargetLabel(target) {
    if (!target) return 'No target';
    const enchant = typeof target.enchant === 'number' ? `.${target.enchant}` : '';
    const tier = target.tier ? `T${target.tier}` : 'T?';
    return `${target.type || 'Resource'} ${tier}${enchant}`;
}
