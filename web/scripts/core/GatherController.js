import {CATEGORIES} from '../constants/LoggerConstants.js';
import settingsSync from '../utils/SettingsSync.js';
import {getLocalPlayerPosition} from './EventRouter.js';
import {
    ENCHANT_VALUES,
    RESOURCE_TYPES,
    TIER_VALUES,
    buildGatherQueue,
    createDefaultGatherSettings,
    formatTargetLabel,
    normalizeGatherSettings,
    reconcileStatuses
} from './GatherLogic.js';

const SETTINGS_KEY = 'gatherSettings';
const HOTKEY = { key: 'G', ctrlKey: true, shiftKey: true };
const FAST_TICK_INTERVAL_MS = 300;
const STUCK_DISTANCE_EPSILON = 0.75;
const STUCK_RECOVERY_DELAY_MS = 2600;
const INTERACT_RETRY_LIMIT = 6;
const INTERACT_STALE_AFTER_MS = 4000;
const RECOVERY_ANGLE_SEQUENCE = [
    Math.PI / 4,
    -Math.PI / 4,
    Math.PI / 2,
    -Math.PI / 2,
    (3 * Math.PI) / 4,
    -(3 * Math.PI) / 4
];

export class GatherController {
    constructor() {
        this.settings = normalizeGatherSettings(settingsSync.getJSON(SETTINGS_KEY, createDefaultGatherSettings()));
        this.queue = [];
        this.previousQueue = [];
        this.statusMap = new Map();
        this.targetStateMap = new Map();
        this.logs = [];
        this.backendStatus = null;
        this.currentTargetId = null;
        this.boundHandlers = [];
        this.intervals = [];
        this.initialized = false;
        this.panel = null;
        this.tickInFlight = false;
    }

    async init() {
        if (this.initialized) return;

        this.panel = document.getElementById('gather-panel');
        if (!this.panel) return;

        this.bindStaticElements();
        this.renderWhitelistControls();
        this.renderCalibrationFields();
        this.render();

        await this.refreshStatus();
        this.rebuildQueue();

        this.intervals.push(setInterval(() => this.refreshStatus(), 2000));
        this.intervals.push(setInterval(() => this.rebuildQueue(), 750));
        this.intervals.push(setInterval(() => this.tick(), FAST_TICK_INTERVAL_MS));

        this.initialized = true;
    }

    destroy() {
        this.boundHandlers.forEach(({ target, event, handler }) => target.removeEventListener(event, handler));
        this.boundHandlers = [];
        this.intervals.forEach(clearInterval);
        this.intervals = [];
        this.initialized = false;
        this.queue = [];
        this.previousQueue = [];
        this.statusMap.clear();
        this.targetStateMap.clear();
        this.currentTargetId = null;
        this.tickInFlight = false;
    }

    bind(target, event, handler) {
        if (!target) return;
        target.addEventListener(event, handler);
        this.boundHandlers.push({ target, event, handler });
    }

    bindStaticElements() {
        const modeToggle = document.getElementById('gatherModeToggle');
        const startStop = document.getElementById('gatherStartStop');
        const calibrate = document.getElementById('gatherCalibrate');
        const hotkeyToggle = document.getElementById('gatherHotkeyEnabled');
        const mountToggle = document.getElementById('gatherUseMount');
        const testN = document.getElementById('gatherTestClickN');
        const testE = document.getElementById('gatherTestClickE');
        const testS = document.getElementById('gatherTestClickS');
        const testW = document.getElementById('gatherTestClickW');

        this.bind(modeToggle, 'click', () => {
            this.settings.modeEnabled = !this.settings.modeEnabled;
            if (!this.settings.modeEnabled) {
                this.stopGather('gather mode disabled');
            }
            this.persistSettings();
            this.render();
        });

        this.bind(startStop, 'click', async () => {
            if (this.backendStatus?.active) {
                await this.stopGather('manual stop requested');
            } else {
                await this.startGather();
            }
        });

        this.bind(calibrate, 'click', async () => {
            await this.calibrate();
        });

        this.bind(hotkeyToggle, 'change', (event) => {
            this.settings.hotkeyEnabled = event.target.checked;
            this.persistSettings();
            this.render();
        });

        this.bind(document, 'keydown', async (event) => {
            if (!this.settings.hotkeyEnabled) return;
            if (event.ctrlKey !== HOTKEY.ctrlKey || event.shiftKey !== HOTKEY.shiftKey) return;
            if (event.key.toUpperCase() !== HOTKEY.key) return;

            event.preventDefault();
            this.settings.modeEnabled = !this.settings.modeEnabled;
            if (!this.settings.modeEnabled) {
                await this.stopGather('gather mode disabled from hotkey');
            }
            this.persistSettings();
            this.render();
        });

        this.bind(document, 'radarMapChanged', async () => {
            if (this.backendStatus?.active) {
                await this.stopGather('map change detected, gather mode stopped');
            }
        });

        if (mountToggle) {
            mountToggle.checked = this.settings.calibration.desiredMount !== false;
            this.bind(mountToggle, 'change', () => {
                this.settings.calibration.desiredMount = mountToggle.checked;
                this.persistSettings();
                // Push the new value to the backend immediately so the next tick
                // sees it without the user having to re-hit Calibrate.
                this.calibrate({ silentSuccess: true }).catch(() => {});
            });
        }

        [
            [testN, 'N'],
            [testE, 'E'],
            [testS, 'S'],
            [testW, 'W'],
        ].forEach(([element, direction]) => {
            if (!element) return;
            this.bind(element, 'click', async () => {
                await this.sendTestClick(direction);
            });
        });
    }

    renderWhitelistControls() {
        const types = document.getElementById('gatherTypeFilters');
        const tiers = document.getElementById('gatherTierFilters');
        const enchants = document.getElementById('gatherEnchantFilters');

        if (types) {
            types.innerHTML = RESOURCE_TYPES.map(type => `
                <label class="flex items-center gap-2 text-sm cursor-pointer">
                    <input type="checkbox" data-gather-type="${type}" class="checkbox checkbox-primary checkbox-xs">
                    <span>${type}</span>
                </label>
            `).join('');

            types.querySelectorAll('[data-gather-type]').forEach(element => {
                const type = element.dataset.gatherType;
                element.checked = this.settings.whitelist.types[type];
                this.bind(element, 'change', () => {
                    this.settings.whitelist.types[type] = element.checked;
                    this.persistSettings();
                    this.rebuildQueue();
                });
            });
        }

        if (tiers) {
            tiers.innerHTML = TIER_VALUES.map(tier => `
                <label class="flex items-center gap-2 text-sm cursor-pointer">
                    <input type="checkbox" data-gather-tier="${tier}" class="checkbox checkbox-primary checkbox-xs">
                    <span>T${tier}</span>
                </label>
            `).join('');

            tiers.querySelectorAll('[data-gather-tier]').forEach(element => {
                const tier = Number(element.dataset.gatherTier);
                element.checked = this.settings.whitelist.tiers[tier];
                this.bind(element, 'change', () => {
                    this.settings.whitelist.tiers[tier] = element.checked;
                    this.persistSettings();
                    this.rebuildQueue();
                });
            });
        }

        if (enchants) {
            enchants.innerHTML = ENCHANT_VALUES.map(enchant => `
                <label class="flex items-center gap-2 text-sm cursor-pointer">
                    <input type="checkbox" data-gather-enchant="${enchant}" class="checkbox checkbox-primary checkbox-xs">
                    <span>.${enchant}</span>
                </label>
            `).join('');

            enchants.querySelectorAll('[data-gather-enchant]').forEach(element => {
                const enchant = Number(element.dataset.gatherEnchant);
                element.checked = this.settings.whitelist.enchants[enchant];
                this.bind(element, 'change', () => {
                    this.settings.whitelist.enchants[enchant] = element.checked;
                    this.persistSettings();
                    this.rebuildQueue();
                });
            });
        }

        ['Static', 'Living'].forEach(kind => {
            const id = kind === 'Static' ? 'gatherAllowStatic' : 'gatherAllowLiving';
            const element = document.getElementById(id);
            if (!element) return;
            element.checked = kind === 'Static' ? this.settings.whitelist.static : this.settings.whitelist.living;
            this.bind(element, 'change', () => {
                if (kind === 'Static') this.settings.whitelist.static = element.checked;
                if (kind === 'Living') this.settings.whitelist.living = element.checked;
                this.persistSettings();
                this.rebuildQueue();
            });
        });
    }

    renderCalibrationFields() {
        const bindings = [
            ['gatherCenterOffsetX', 'centerOffsetX'],
            ['gatherCenterOffsetY', 'centerOffsetY'],
            ['gatherSafeMarginX', 'safeMarginX'],
            ['gatherSafeMarginY', 'safeMarginY'],
            ['gatherMoveRadiusPx', 'moveRadiusPx'],
            ['gatherInteractRadiusPx', 'interactRadiusPx'],
            ['gatherInteractThresholdGu', 'interactThresholdGu'],
            ['gatherRetryLimit', 'retryLimit']
        ];

        bindings.forEach(([id, key]) => {
            const element = document.getElementById(id);
            if (!element) return;

            if (key === 'retryLimit') {
                element.value = this.settings.retryLimit;
            } else {
                element.value = this.settings.calibration[key];
            }

            this.bind(element, 'input', () => {
                const value = Number(element.value);
                if (key === 'retryLimit') {
                    this.settings.retryLimit = value;
                } else {
                    this.settings.calibration[key] = value;
                }
                this.persistSettings();
            });
        });
    }

    persistSettings() {
        settingsSync.setJSON(SETTINGS_KEY, this.settings);
    }

    getHarvestables() {
        return window.harvestablesHandler?.getHarvestableList?.() || [];
    }

    getThreatCount() {
        const players = window.playersHandler?.getFilteredPlayers?.() || [];
        return players.filter(player => typeof player.isHostile === 'function' && player.isHostile()).length;
    }

    rebuildQueue() {
        const now = Date.now();
        const playerPosition = getLocalPlayerPosition();
        const nextQueue = buildGatherQueue(this.getHarvestables(), playerPosition, this.settings, this.statusMap, now);

        reconcileStatuses(this.previousQueue, nextQueue, this.statusMap, this.currentTargetId);
        nextQueue.forEach(node => {
            node.status = this.statusMap.get(node.id) || 'candidate';
        });

        this.previousQueue = this.queue;
        this.queue = nextQueue;
        this.trimTargetState(nextQueue);
        this.render();
    }

    trimTargetState(queue = this.queue) {
        const activeIds = new Set((queue || []).map(node => node.id));
        for (const id of this.targetStateMap.keys()) {
            if (!activeIds.has(id)) {
                this.targetStateMap.delete(id);
            }
        }
    }

    getTargetState(targetId) {
        if (!this.targetStateMap.has(targetId)) {
            this.targetStateMap.set(targetId, {
                lastDistance: Number.NaN,
                lastProgressAt: Date.now(),
                recoveryAttempts: 0,
                requestErrors: 0,
                interactAttempts: 0,
                interactStartedAt: Number.NaN
            });
        }
        return this.targetStateMap.get(targetId);
    }

    markTargetFailed(target, reason) {
        this.statusMap.set(target.id, 'failed');
        this.targetStateMap.delete(target.id);
        this.queue = this.queue.filter(node => node.id !== target.id);
        if (this.currentTargetId === target.id) {
            this.currentTargetId = null;
        }
        this.log(reason);
        this.render();
    }

    createRecoveryTarget(target, playerPosition, targetState) {
        const dx = Number(target.position?.x ?? 0) - Number(playerPosition?.x ?? 0);
        const dy = Number(target.position?.y ?? 0) - Number(playerPosition?.y ?? 0);
        if (dx === 0 && dy === 0) {
            return { target, label: 'straight retry' };
        }

        const angleIndex = Math.max(targetState.recoveryAttempts - 1, 0) % RECOVERY_ANGLE_SEQUENCE.length;
        const angle = RECOVERY_ANGLE_SEQUENCE[angleIndex];
        const cos = Math.cos(angle);
        const sin = Math.sin(angle);
        const rotatedDX = (dx * cos) - (dy * sin);
        const rotatedDY = (dx * sin) + (dy * cos);

        return {
            target: {
                ...target,
                position: {
                    x: Number(playerPosition?.x ?? 0) + rotatedDX,
                    y: Number(playerPosition?.y ?? 0) + rotatedDY
                },
                distance: Math.hypot(rotatedDX, rotatedDY)
            },
            label: `${Math.round((angle * 180) / Math.PI)}deg recovery`
        };
    }

    async refreshStatus() {
        try {
            const response = await fetch('/api/gather/status');
            this.backendStatus = await response.json();
            this.render();
        } catch (error) {
            this.log(`Status refresh failed: ${error.message}`);
        }
    }

    async calibrate({ silentSuccess = false } = {}) {
        try {
            const response = await fetch('/api/gather/calibrate', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    calibration: {
                        ...this.settings.calibration
                    }
                })
            });
            const payload = await response.json();
            this.backendStatus = payload.status;
            if (!response.ok) {
                this.log(payload.status?.lastError || 'Calibration failed');
                this.render();
                return false;
            }
            if (payload.status?.calibration) {
                this.settings.calibration = {
                    ...this.settings.calibration,
                    ...payload.status.calibration
                };
                this.persistSettings();
                this.renderCalibrationFields();
            }
            if (!silentSuccess) {
                this.log('Gather calibration saved from Albion window');
            }
            this.render();
            return true;
        } catch (error) {
            this.log(`Calibration failed: ${error.message}`);
            return false;
        }
    }

    async sendTestClick(direction) {
        try {
            // Ensure backend has the latest calibration values from the UI.
            if (!this.backendStatus?.calibrated) {
                const ok = await this.calibrate({ silentSuccess: true });
                if (!ok) return;
            }

            const response = await fetch('/api/gather/test-click', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ testDirection: direction })
            });
            const payload = await response.json();
            this.backendStatus = payload.status;

            if (!response.ok || !payload.step) {
                this.log(payload.status?.lastError || `Test click ${direction} failed`);
                this.render();
                return;
            }

            this.log(`Test click ${direction} → (${payload.step.clickX}, ${payload.step.clickY})`);
            this.render();
        } catch (error) {
            this.log(`Test click failed: ${error.message}`);
        }
    }

    async startGather() {
        if (!this.settings.modeEnabled) {
            this.settings.modeEnabled = true;
            this.persistSettings();
        }

        if (!this.backendStatus?.calibrated) {
            this.log('Calibration missing, preparing Albion window first');
            const calibrated = await this.calibrate({ silentSuccess: true });
            if (!calibrated) {
                this.render();
                return;
            }
        }

        const response = await fetch('/api/gather/toggle', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ active: true })
        });
        const payload = await response.json();
        this.backendStatus = payload.status;

        if (!response.ok) {
            this.log(payload.status?.lastError || 'Unable to start gather mode');
            this.render();
            return;
        }

        this.targetStateMap.clear();
        this.tickInFlight = false;
        this.log('Gather mode started');
        this.render();
    }

    async stopGather(reason = 'gather mode stopped') {
        try {
            const response = await fetch('/api/gather/stop', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ reason })
            });
            const payload = await response.json();
            this.backendStatus = payload.status;
            this.currentTargetId = null;
            this.targetStateMap.clear();
            this.tickInFlight = false;
            this.log(reason);
            this.render();
        } catch (error) {
            this.log(`Stop failed: ${error.message}`);
        }
    }

    async tick() {
        if (this.tickInFlight) return;
        if (!this.settings.modeEnabled) return;
        if (!this.backendStatus?.active) return;

        this.tickInFlight = true;
        let nextTarget = null;
        let targetState = null;

        try {
            this.rebuildQueue();
            if (this.currentTargetId !== null && !this.queue.some(node => node.id === this.currentTargetId)) {
                this.statusMap.set(this.currentTargetId, 'done');
                this.targetStateMap.delete(this.currentTargetId);
                this.log(`Target ${this.currentTargetId} disappeared, switching to next resource`);
                this.currentTargetId = null;
            }

            nextTarget = this.queue[0];
            if (!nextTarget) {
                await this.stopGather('queue empty, gather mode stopped');
                return;
            }

            const now = Date.now();
            const playerPosition = getLocalPlayerPosition();
            const interactThreshold = Number(this.settings.calibration.interactThresholdGu) || 14;
            const canRecover = nextTarget.distance > interactThreshold * 1.5;

            targetState = this.getTargetState(nextTarget.id);
            const distanceDelta = Number.isFinite(targetState.lastDistance)
                ? targetState.lastDistance - nextTarget.distance
                : Number.POSITIVE_INFINITY;
            const madeProgress = !Number.isFinite(targetState.lastDistance) || distanceDelta > STUCK_DISTANCE_EPSILON;

            if (madeProgress) {
                targetState.lastProgressAt = now;
                targetState.recoveryAttempts = 0;
                targetState.requestErrors = 0;
                targetState.interactAttempts = 0;
                targetState.interactStartedAt = Number.NaN;
            } else if (!Number.isFinite(targetState.lastProgressAt)) {
                targetState.lastProgressAt = now;
            }
            targetState.lastDistance = nextTarget.distance;

            let requestTarget = nextTarget;
            let recoveryLabel = '';
            if (canRecover && (now - targetState.lastProgressAt) >= STUCK_RECOVERY_DELAY_MS) {
                targetState.recoveryAttempts += 1;
                targetState.lastProgressAt = now;

                if (targetState.recoveryAttempts > this.settings.retryLimit) {
                    this.markTargetFailed(nextTarget, `Recovery limit reached for ${formatTargetLabel(nextTarget)}`);
                    return;
                }

                const recovery = this.createRecoveryTarget(nextTarget, playerPosition, targetState);
                requestTarget = recovery.target;
                recoveryLabel = ` ${recovery.label}`;
            }

            this.currentTargetId = nextTarget.id;
            this.statusMap.set(nextTarget.id, 'targeting');

            const response = await fetch('/api/gather/target', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    player: playerPosition,
                    target: requestTarget,
                    currentMapId: window.currentMapId || '',
                    isMounted: Boolean(window.playersHandler?.localPlayer?.mounted)
                })
            });
            const payload = await response.json();
            this.backendStatus = payload.status;

            if (!response.ok || !payload.step) {
                targetState.requestErrors += 1;
                if (targetState.requestErrors > this.settings.retryLimit) {
                    this.markTargetFailed(nextTarget, `Request limit reached for ${formatTargetLabel(nextTarget)}`);
                    return;
                }

                this.log(payload.status?.lastError || 'Gather step failed');
                this.render();
                return;
            }

            targetState.requestErrors = 0;
            if (payload.step.mode === 'interacting') {
                if (!Number.isFinite(targetState.interactStartedAt)) {
                    targetState.interactStartedAt = now;
                }
                targetState.interactAttempts += 1;

                const targetAgeMs = Math.max(0, now - Number(nextTarget.lastSeenAt || now));
                if (targetAgeMs >= INTERACT_STALE_AFTER_MS && targetState.interactAttempts >= INTERACT_RETRY_LIMIT) {
                    this.markTargetFailed(nextTarget, `Skipping stale target ${formatTargetLabel(nextTarget)}`);
                    return;
                }
            } else {
                targetState.interactAttempts = 0;
                targetState.interactStartedAt = Number.NaN;
            }

            this.statusMap.set(nextTarget.id, payload.step.mode);
            this.log(`${payload.step.mode}: ${formatTargetLabel(nextTarget)} @ ${Math.round(nextTarget.distance)}u${recoveryLabel}`);
            this.render();
        } catch (error) {
            if (nextTarget) {
                targetState = targetState || this.getTargetState(nextTarget.id);
                targetState.requestErrors += 1;
                if (targetState.requestErrors > this.settings.retryLimit) {
                    this.markTargetFailed(nextTarget, `Request limit reached for ${formatTargetLabel(nextTarget)}`);
                    return;
                }
            }

            this.log(`Gather tick failed: ${error.message}`);
            this.render();
        } finally {
            this.tickInFlight = false;
        }
    }

    log(message) {
        this.logs.unshift({
            message,
            at: new Date().toLocaleTimeString()
        });
        this.logs = this.logs.slice(0, 10);
        window.logger?.info(CATEGORIES.GATHER, 'GatherLog', { message });
    }

    render() {
        if (!this.panel) return;

        this.panel.classList.toggle('opacity-70', !this.settings.modeEnabled);

        const modeButton = document.getElementById('gatherModeToggle');
        const modeBadge = document.getElementById('gatherModeBadge');
        const hotkeyToggle = document.getElementById('gatherHotkeyEnabled');
        const startStop = document.getElementById('gatherStartStop');
        const calibrationState = document.getElementById('gatherCalibrationState');
        const focusedState = document.getElementById('gatherFocusedState');
        const queueCount = document.getElementById('gatherQueueCount');
        const currentTarget = document.getElementById('gatherCurrentTarget');
        const runState = document.getElementById('gatherRunState');
        const threatState = document.getElementById('gatherThreatState');
        const statusNote = document.getElementById('gatherStatusNote');
        const list = document.getElementById('gatherQueueList');
        const logs = document.getElementById('gatherLogList');

        if (modeButton) modeButton.textContent = this.settings.modeEnabled ? 'Disable Mode' : 'Enable Mode';
        if (modeBadge) {
            modeBadge.textContent = this.settings.modeEnabled ? 'Mode Enabled' : 'Mode Disabled';
            modeBadge.className = this.settings.modeEnabled
                ? 'badge badge-primary badge-outline'
                : 'badge badge-ghost';
        }
        if (hotkeyToggle) hotkeyToggle.checked = this.settings.hotkeyEnabled;
        if (startStop) startStop.textContent = this.backendStatus?.active ? 'Stop Run' : 'Start Run';
        if (calibrationState) calibrationState.textContent = this.backendStatus?.calibrated ? 'Ready' : 'Required';
        if (focusedState) focusedState.textContent = this.backendStatus?.focusedWindow ? 'Focused' : 'Not Focused';
        if (queueCount) queueCount.textContent = `${this.queue.length}`;
        if (currentTarget) currentTarget.textContent = formatTargetLabel(this.queue[0] || this.backendStatus?.currentTarget);
        if (runState) runState.textContent = this.backendStatus?.state || 'idle';
        if (threatState) threatState.textContent = `${this.getThreatCount()} hostile`;
        if (statusNote) {
            statusNote.textContent = this.backendStatus?.lastError
                || this.backendStatus?.lastAction
                || 'Start Run will focus Albion automatically and use your saved calibration values.';
            statusNote.className = this.backendStatus?.lastError
                ? 'rounded-xl border border-error/30 bg-error/5 px-3 py-2 text-xs text-error'
                : 'rounded-xl border border-base-content/10 px-3 py-2 text-xs text-base-content/60';
        }

        if (list) {
            list.innerHTML = this.queue.slice(0, 6).map(node => `
                <div class="flex items-center justify-between rounded-lg bg-base-300/40 px-3 py-2 text-sm">
                    <div>
                        <div class="font-medium">${formatTargetLabel(node)}</div>
                        <div class="text-xs text-base-content/50">${Math.round(node.distance)} units</div>
                    </div>
                    <span class="badge badge-ghost">${node.status}</span>
                </div>
            `).join('') || '<p class="text-sm text-base-content/50">No matching live nodes.</p>';
        }

        if (logs) {
            logs.innerHTML = this.logs.map(entry => `
                <div class="rounded-lg border border-base-content/5 bg-base-300/20 px-3 py-2 text-xs">
                    <div class="font-mono text-base-content/40">${entry.at}</div>
                    <div>${entry.message}</div>
                </div>
            `).join('') || '<p class="text-sm text-base-content/50">No gather events yet.</p>';
        }
    }
}

export function createGatherController() {
    return new GatherController();
}
