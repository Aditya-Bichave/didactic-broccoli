// EventRouter.js - WebSocket event routing to handlers
// Extracted from Utils.js during Phase 1B refactor

import {EventCodes} from '../utils/EventCodes.js';
import {OperationCodes} from '../utils/OperationCodes.js';
import {CATEGORIES} from '../constants/LoggerConstants.js';

// Map change debouncing
const MAP_CHANGE_DEBOUNCE_MS = 4000;
let lastMapChangeTime = 0;

// Local player position (relative coords).
// lpX/lpY hold the LAST KNOWN move destination sniffed from Request_Move.
// The character hasn't arrived there yet — getLocalPlayerPosition() extrapolates
// the current position along the last move vector using an estimated speed.
let lpX = 0.0;
let lpY = 0.0;

let moveTargetX = 0.0;
let moveTargetY = 0.0;
let hasMoveTarget = false;
let hasAuthoritativeLocalMove = false;

// Movement interpolation state. When a new Move request fires, we snapshot the
// current interpolated position as the origin of the new leg, stash the
// destination, and start the clock. Subsequent reads advance along the leg.
let moveOriginX = 0.0;
let moveOriginY = 0.0;
let moveStartedAt = 0;
let moveMountedAtStart = false;

// Movement speed estimates in game units per second. Ground truth varies with
// mount type, buffs, terrain; these are conservative averages that keep the
// interpolated position slightly *behind* the true position rather than ahead
// (better to under-extrapolate than click past the destination).
const WALK_SPEED_GU_PER_SEC = 5.5;
const MOUNT_SPEED_GU_PER_SEC = 9.5;

// Expose globally for debug access
window.lpX = lpX;
window.lpY = lpY;

// Dependency references (set via init)
let handlers = null;
let map = null;
let radarRenderer = null;

// Helper: Update local player position (DRY pattern).
// Called on every Request_Move sniff — the (x,y) passed in is the click
// DESTINATION, not the character's current location. We snapshot the current
// interpolated position as the new leg's origin before overwriting the
// destination so interpolation stays continuous across chained move requests.
function publishLocalPlayerPosition(x, y) {
    lpX = x;
    lpY = y;
    window.lpX = lpX;
    window.lpY = lpY;
    handlers?.playersHandler?.updateLocalPlayerPosition(lpX, lpY);
    radarRenderer?.setLocalPlayerPosition?.(lpX, lpY);
    document.dispatchEvent(new CustomEvent('localPlayerPositionChanged', {
        detail: {x: lpX, y: lpY}
    }));
}

function rememberLocalPlayerMoveTarget(x, y) {
    const now = Date.now();
    const current = computeInterpolatedPosition(now);
    moveOriginX = current.x;
    moveOriginY = current.y;
    moveStartedAt = now;
    moveMountedAtStart = Boolean(handlers?.playersHandler?.localPlayer?.mounted);
    moveTargetX = x;
    moveTargetY = y;
    hasMoveTarget = true;
}

function updateLocalPlayerPosition(x, y, {authoritativeMove = false} = {}) {
    const now = Date.now();

    if (authoritativeMove) {
        hasAuthoritativeLocalMove = true;
    }

    if (hasMoveTarget) {
        const remainingDistance = Math.hypot(moveTargetX - x, moveTargetY - y);
        if (remainingDistance <= 0.35) {
            hasMoveTarget = false;
            moveStartedAt = 0;
        } else {
            moveOriginX = x;
            moveOriginY = y;
            moveStartedAt = now;
            moveMountedAtStart = Boolean(handlers?.playersHandler?.localPlayer?.mounted);
        }
    } else {
        moveOriginX = x;
        moveOriginY = y;
        moveStartedAt = 0;
    }

    publishLocalPlayerPosition(x, y);
}

function isLocalPlayerEntity(id) {
    const localId = Number(handlers?.playersHandler?.localPlayer?.id);
    return Number.isFinite(localId) && localId > 0 && Number(id) === localId;
}

// Estimate the character's *current* position along the most recent move leg.
// Without this, callers see the last-known destination, which is usually where
// the character is heading — not where it is — leading to bad distance
// estimates for gather click projection.
function computeInterpolatedPosition(now = Date.now()) {
    if (hasAuthoritativeLocalMove || !hasMoveTarget || moveStartedAt === 0) {
        return {x: lpX, y: lpY};
    }
    const dx = moveTargetX - moveOriginX;
    const dy = moveTargetY - moveOriginY;
    const total = Math.hypot(dx, dy);
    if (total < 0.01) {
        return {x: lpX, y: lpY};
    }
    const speed = moveMountedAtStart ? MOUNT_SPEED_GU_PER_SEC : WALK_SPEED_GU_PER_SEC;
    const traveled = Math.max(0, (now - moveStartedAt) / 1000) * speed;
    if (traveled >= total) {
        return {x: moveTargetX, y: moveTargetY};
    }
    const t = traveled / total;
    return {
        x: moveOriginX + dx * t,
        y: moveOriginY + dy * t
    };
}

// Helper function to get event name (for debugging)
function getEventName(eventCode) {
    const eventNames = {
        1: 'Leave',
        2: 'JoinFinished',
        3: 'Move',
        4: 'Teleport',
        5: 'ChangeEquipment',
        6: 'HealthUpdate',
        7: 'HealthUpdates',
        15: 'Damage',
        21: 'Request_Move',
        29: 'NewCharacter',
        35: 'ClusterChange',
        38: 'NewSimpleHarvestableObject',
        39: 'NewSimpleHarvestableObjectList',
        40: 'NewHarvestableObject',
        46: 'HarvestableChangeState',
        71: 'NewMob',
        72: 'MobChangeState',
        91: 'RegenerationHealthChanged',
        101: 'NewHarvestableObject',
        102: 'NewSimpleHarvestableObjectList',
        103: 'HarvestStart',
        104: 'HarvestCancel',
        105: 'HarvestFinished',
        137: 'GetCharacterStats',
        201: 'NewSimpleItem',
        202: 'NewEquipmentItem',
    };
    return eventNames[eventCode] || `Unknown_${eventCode}`;
}

export function init(deps) {
    handlers = deps.handlers;
    map = deps.map;
    radarRenderer = deps.radarRenderer;
}

export function setRadarRenderer(renderer) {
    radarRenderer = renderer;
}

export function setMap(mapRef) {
    map = mapRef;
}

export function getLocalPlayerPosition() {
    return computeInterpolatedPosition();
}

export function restoreMapFromSession() {
    if (!map) return;

    try {
        const savedMap = sessionStorage.getItem('lastMapDisplayed');
        window.logger?.debug(CATEGORIES.MAP, 'SessionRestoreAttempt', {
            hasData: !!savedMap
        });

        if (savedMap) {
            const data = JSON.parse(savedMap);

            if (data.mapId !== undefined && data.mapId !== null && data.mapId !== -1) {
                map.id = data.mapId;
                map.hX = data.hX || 0;
                map.hY = data.hY || 0;
                map.isBZ = data.isBZ || false;
                window.currentMapId = map.id;

                window.logger?.info(CATEGORIES.MAP, 'MapRestoredFromSession', {
                    mapId: map.id,
                    age: Date.now() - (data.timestamp || 0)
                });
            }
        }
    } catch (e) {
        window.logger?.warn(CATEGORIES.MAP, 'SessionRestoreFailed', {error: e?.message});
    }
}

export function onEvent(Parameters) {
    const id = parseInt(Parameters[0]);
    const eventCode = Parameters[252];

    // Raw packet logging
    window.logger?.debug(CATEGORIES.NETWORK, `Event_${eventCode}`, {
        id,
        eventCode,
        allParameters: Parameters
    });

    // Detailed event logging (skip verbose events)
    if (eventCode !== 91) {
        const paramDetails = {};
        for (let key in Parameters) {
            if (Parameters.hasOwnProperty(key) && key !== '252' && key !== '0') {
                paramDetails[`param[${key}]`] = Parameters[key];
            }
        }

        window.logger?.debug(CATEGORIES.NETWORK, `Event_${eventCode}_ID_${id}`, {
            id,
            eventCode,
            eventName: getEventName(eventCode),
            parameterCount: Object.keys(Parameters).length,
            parameters: paramDetails
        });
    }

    const {
        playersHandler, mobsHandler, harvestablesHandler, chestsHandler,
        dungeonsHandler, fishingHandler, wispCageHandler
    } = handlers;

    switch (eventCode) {
        case EventCodes.Leave:
            playersHandler.removePlayer(id);
            mobsHandler.removeMist(id);
            mobsHandler.removeMob(id);
            dungeonsHandler.removeDungeon(id);
            chestsHandler.removeChest(id);
            fishingHandler.removeFish(id);
            wispCageHandler.removeCage(id);
            break;

        case EventCodes.Move:
            const posX = Parameters[4];
            const posY = Parameters[5];
            playersHandler.updatePlayerPosition?.(id, posX, posY);
            if (isLocalPlayerEntity(id)) {
                updateLocalPlayerPosition(posX, posY, {authoritativeMove: true});
                window.logger?.debug(CATEGORIES.PLAYERS, 'LocalPlayerMoveEvent', {
                    id,
                    posX,
                    posY
                });
            }
            mobsHandler.updateMistPosition(id, posX, posY);
            mobsHandler.updateMobPosition(id, posX, posY);
            break;

        case EventCodes.NewCharacter:
            playersHandler.handleNewPlayerEvent(id, Parameters);
            break;

        case EventCodes.NewSimpleHarvestableObjectList:
        case EventCodes.NewSimpleHarvestableObject:
            harvestablesHandler.newSimpleHarvestableObject(Parameters);
            break;

        case EventCodes.NewHarvestableObject:
            harvestablesHandler.newHarvestableObject(id, Parameters);
            break;

        case EventCodes.HarvestableChangeState:
            harvestablesHandler.HarvestUpdateEvent(Parameters);
            break;

        case EventCodes.HarvestStart:
        case EventCodes.HarvestCancel:
            // Handled by HarvestablesHandler via database validation
            break;

        case EventCodes.HarvestFinished:
            harvestablesHandler.harvestFinished(Parameters);
            break;

        case EventCodes.InventoryPutItem:
        case EventCodes.InventoryDeleteItem:
        case EventCodes.InventoryState:
        case EventCodes.NewSimpleItem:
        case EventCodes.NewEquipmentItem:
        case EventCodes.NewJournalItem:
        case EventCodes.UpdateFame:
        case EventCodes.UpdateMoney:
            // Inventory/economy events - not currently used
            break;

        case EventCodes.MobChangeState:
            mobsHandler.updateEnchantEvent(Parameters);
            break;

        case EventCodes.RegenerationHealthChanged: {
            const mobInfo = mobsHandler.debugLogMobById(Parameters[0]);
            window.logger?.debug(CATEGORIES.MOBS, 'regen_health_changed', {
                eventCode: 91,
                id: Parameters[0],
                mobInfo,
                allParameters: Parameters
            });
        }
            playersHandler.UpdatePlayerHealth(Parameters);
            mobsHandler.updateMobHealthRegen(Parameters);
            break;

        case EventCodes.HealthUpdate: {
            const mobInfo = mobsHandler.debugLogMobById(Parameters[0]);
            window.logger?.debug(CATEGORIES.MOBS, 'health_update', {
                eventCode: 6,
                id: Parameters[0],
                mobInfo,
                allParameters: Parameters
            });
        }
            playersHandler.UpdatePlayerLooseHealth(Parameters);
            mobsHandler.updateMobHealth(Parameters);
            break;

        case EventCodes.HealthUpdates:
            window.logger?.debug(CATEGORIES.MOBS, 'bulk_hp_update', {
                eventCode: 7,
                allParameters: Parameters
            });
            mobsHandler.updateMobHealthBulk(Parameters);
            break;

        case EventCodes.CharacterEquipmentChanged:
            playersHandler.updateItems(id, Parameters);
            break;

        case EventCodes.NewMob:
            mobsHandler.NewMobEvent(Parameters);
            break;

        case EventCodes.Mounted:
            playersHandler.handleMountedPlayerEvent(id, Parameters);
            break;

        case EventCodes.NewRandomDungeonExit:
            dungeonsHandler.dungeonEvent(Parameters);
            break;

        case EventCodes.NewLootChest:
            chestsHandler.addChestEvent(Parameters);
            break;

        case EventCodes.NewCagedObject:
            wispCageHandler.newCageEvent(Parameters);
            break;

        case EventCodes.CagedObjectStateUpdated:
            wispCageHandler.cageOpenedEvent(Parameters);
            break;

        case EventCodes.NewFishingZoneObject:
            fishingHandler.newFishEvent(Parameters);
            break;

        case EventCodes.FishingFinished:
            fishingHandler.fishingEnd(Parameters);
            break;

        case EventCodes.ChangeFlaggingFinished:
            playersHandler.updatePlayerFaction(Parameters[0], Parameters[1]);
            break;

        // upstream 590 = UpdateEnemyWarBannerActive; local dispatch labels it "key_sync", semantics diverge.
        case 590:
            window.logger?.debug(CATEGORIES.NETWORK, 'key_sync', {Parameters});
            break;
    }
}

export function onRequest(Parameters) {
    // 22 = OperationCodes.Move.
    if (Parameters[253] == OperationCodes.Move) {
        if (Array.isArray(Parameters[1]) && Parameters[1].length === 2) {
            rememberLocalPlayerMoveTarget(Parameters[1][0], Parameters[1][1]);
            window.logger?.debug(CATEGORIES.PLAYERS, 'Operation21_LocalPlayerTarget', {
                targetX: Parameters[1][0],
                targetY: Parameters[1][1]
            });
        }
        // Legacy Buffer handling
        else if (Parameters[1] && Parameters[1].type === 'Buffer') {
            const uint8Array = new Uint8Array(Parameters[1].data);
            const dataView = new DataView(uint8Array.buffer);
            rememberLocalPlayerMoveTarget(dataView.getFloat32(0, true), dataView.getFloat32(4, true));
        } else {
            window.logger?.error(CATEGORIES.PLAYERS, 'OnRequest_Move_UnknownFormat', {
                param1: Parameters[1],
                param1Type: typeof Parameters[1]
            });
        }
    }
}

export function onResponse(Parameters, clearHandlersCallback) {
    if (Parameters[253] == OperationCodes.ChangeCluster) {
        const newMapId = Parameters[0];
        if (typeof newMapId === 'string' && newMapId.length > 0 && newMapId !== map.id) {
            const previousMapId = map.id;
            map.id = newMapId;
            window.currentMapId = map.id;
            lastMapChangeTime = Date.now();
            radarRenderer?.setMap?.(map);

            try {
                sessionStorage.setItem('lastMapDisplayed', JSON.stringify({
                    mapId: map.id,
                    hX: map.hX,
                    hY: map.hY,
                    isBZ: map.isBZ,
                    timestamp: Date.now()
                }));
            } catch (e) {
                window.logger?.warn(CATEGORIES.MAP, 'SessionStorageFailed', {error: e?.message});
            }

            window.logger?.info(CATEGORIES.MAP, 'ChangeClusterResponse', {
                previousMapId,
                newMapId: map.id
            });

            document.dispatchEvent(new CustomEvent('radarMapChanged', {
                detail: {
                    previousMapId,
                    mapId: map.id
                }
            }));

            clearHandlersCallback();
        }
        return;
    }

    // upstream 35 = InventoryStack; this branch treats it as a map-change response, semantics diverge.
    if (Parameters[253] == 35) {
        const newMapId = Parameters[0];
        const now = Date.now();
        const timeSinceLastChange = now - lastMapChangeTime;

        // Debounce: Ignore rapid map changes
        if (timeSinceLastChange < MAP_CHANGE_DEBOUNCE_MS && map.id !== -1) {
            window.logger?.debug(CATEGORIES.MAP, 'MapChangeDebounced', {
                currentMapId: map.id,
                newMapId,
                timeSinceLastChange
            });
            return;
        }

        // Skip if same map ID
        if (newMapId === map.id) {
            return;
        }

        const previousMapId = map.id;
        map.id = newMapId;
        lastMapChangeTime = now;
        window.currentMapId = map.id;

        window.logger?.info(CATEGORIES.MAP, 'MapChanged', {
            previousMapId,
            newMapId: map.id
        });

        document.dispatchEvent(new CustomEvent('radarMapChanged', {
            detail: {
                previousMapId,
                mapId: map.id
            }
        }));

        if (radarRenderer) {
            radarRenderer.setMap(map);
        }

        // Save to sessionStorage
        try {
            sessionStorage.setItem('lastMapDisplayed', JSON.stringify({
                mapId: map.id,
                hX: map.hX,
                hY: map.hY,
                isBZ: map.isBZ,
                timestamp: Date.now()
            }));
        } catch (e) {
            window.logger?.warn(CATEGORIES.MAP, 'SessionStorageFailed', {error: e?.message});
        }
    }
    // All data on the player joining the map (us)
    else if (Parameters[253] == OperationCodes.Join) {
        if (Number.isFinite(Number(Parameters[0]))) {
            handlers?.playersHandler?.setLocalPlayerId?.(Number(Parameters[0]));
        }

        // Decode position from Buffer or Array
        if (Parameters[9] && Parameters[9].type === 'Buffer') {
            const uint8Array = new Uint8Array(Parameters[9].data);
            const dataView = new DataView(uint8Array.buffer);
            updateLocalPlayerPosition(dataView.getFloat32(0, true), dataView.getFloat32(4, true));
            window.logger?.info(CATEGORIES.PLAYERS, 'OnResponse_JoinMap_BufferDecoded', {lpX, lpY});
        } else if (Array.isArray(Parameters[9])) {
            updateLocalPlayerPosition(Parameters[9][0], Parameters[9][1]);
            window.logger?.info(CATEGORIES.PLAYERS, 'OnResponse_JoinMap_Array', {lpX, lpY});
        } else {
            window.logger?.error(CATEGORIES.PLAYERS, 'OnResponse_JoinMap_UnknownFormat', {
                param9: Parameters[9],
                param9Type: typeof Parameters[9]
            });
        }

        if (typeof Parameters[8] === 'string' && Parameters[8].length > 0) {
            const previousMapId = map.id;
            map.id = Parameters[8];

            if (Parameters[103] && typeof Parameters[103] === 'object') {
                const values = Object.values(Parameters[103]);
                map.isBZ = values.includes(2);
            } else {
                map.isBZ = false;
            }

            window.currentMapId = map.id;
            lastMapChangeTime = Date.now();
            radarRenderer?.setMap?.(map);

            try {
                sessionStorage.setItem('lastMapDisplayed', JSON.stringify({
                    mapId: map.id,
                    hX: map.hX,
                    hY: map.hY,
                    isBZ: map.isBZ,
                    timestamp: Date.now()
                }));
            } catch (e) {
                window.logger?.warn(CATEGORIES.MAP, 'SessionStorageFailed', {error: e?.message});
            }

            window.logger?.info(CATEGORIES.MAP, 'MapChangedFromJoinMap', {
                previousMapId,
                newMapId: map.id
            });

            document.dispatchEvent(new CustomEvent('radarMapChanged', {
                detail: {
                    previousMapId,
                    mapId: map.id
                }
            }));
        }

        clearHandlersCallback();
    // upstream 137 = ChangeGuildTax; inline label says "character stats", branch appears dead.
    } else if (Parameters[253] == 137) {
        // Character stats response - not currently used
    }
}

export function reset() {
    lpX = 0.0;
    lpY = 0.0;
    moveTargetX = 0.0;
    moveTargetY = 0.0;
    hasMoveTarget = false;
    hasAuthoritativeLocalMove = false;
    window.lpX = 0;
    window.lpY = 0;
    moveOriginX = 0.0;
    moveOriginY = 0.0;
    moveStartedAt = 0;
    moveMountedAtStart = false;
    lastMapChangeTime = 0;

    // Clear references to prevent memory leaks
    handlers = null;
    map = null;
    radarRenderer = null;
}
