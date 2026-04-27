// RadarWorker.js - Offloads rendering, WebSocket routing, and state handlers

import {CATEGORIES} from '../constants/LoggerConstants.js';
import * as DatabaseLoader from '../core/DatabaseLoader.js';
import * as EventRouter from '../core/EventRouter.js';
import {createRadarRenderer} from '../utils/RadarRenderer.js';
import {MapH} from '../utils/Map.js';
import {DrawingUtils} from '../utils/DrawingUtils.js';

import {PlayersDrawing} from '../drawings/PlayersDrawing.js';
import {HarvestablesDrawing} from '../drawings/HarvestablesDrawing.js';
import {MobsDrawing} from '../drawings/MobsDrawing.js';
import {ChestsDrawing} from '../drawings/ChestsDrawing.js';
import {DungeonsDrawing} from '../drawings/DungeonsDrawing.js';
import {MapDrawing} from '../drawings/MapsDrawing.js';
import {WispCageDrawing} from '../drawings/WispCageDrawing.js';
import {FishingDrawing} from '../drawings/FishingDrawing.js';

import {PlayersHandler} from '../handlers/PlayersHandler.js';
import {WispCageHandler} from '../handlers/WispCageHandler.js';
import {FishingHandler} from '../handlers/FishingHandler.js';
import {MobsHandler} from '../handlers/MobsHandler.js';
import {ChestsHandler} from '../handlers/ChestsHandler.js';
import {HarvestablesHandler} from '../handlers/HarvestablesHandler.js';
import {DungeonsHandler} from '../handlers/DungeonsHandler.js';
import {EventEmitter} from '../utils/EventEmitter.js';

// Polyfill window for worker context
if (typeof window === 'undefined') {
    self.window = self;
}

// Proxies to main thread
self.window.logger = {
    info: (cat, msg, meta) => postMessage({ type: 'log', level: 'info', cat, msg, meta }),
    warn: (cat, msg, meta) => postMessage({ type: 'log', level: 'warn', cat, msg, meta }),
    error: (cat, msg, meta) => postMessage({ type: 'log', level: 'error', cat, msg, meta }),
    debug: (cat, msg, meta) => postMessage({ type: 'log', level: 'debug', cat, msg, meta })
};

let workerSettings = {};

// We define an object to mock settingsSync that syncs with main thread
self.window.settingsSync = {
    getNumber: (key) => workerSettings[key],
    getBool: (key, def = false) => workerSettings[key] !== undefined ? workerSettings[key] : def,
    getFloat: (key, def = 0) => workerSettings[key] !== undefined ? workerSettings[key] : def,
    getString: (key, def = '') => workerSettings[key] !== undefined ? workerSettings[key] : def,
    getJSON: (key, def = {}) => workerSettings[key] !== undefined ? workerSettings[key] : def
};
// Add alias for settingsSync
// To intercept direct imports of settingsSync
self.window.__getSettingsSync = () => self.window.settingsSync;

let radarRenderer = null;
let emitter = null;

let handlers = {
    harvestables: null, mobs: null, players: null, chests: null,
    dungeons: null, wispCage: null, fishing: null
};

let drawings = {
    harvestables: null, mobs: null, players: null, chests: null,
    dungeons: null, wispCage: null, fishing: null, maps: null
};

let drawingUtils = null;
let map = null;

function clearHandlers() {
    handlers.chests?.chestsList && (handlers.chests.chestsList = []);
    handlers.dungeons?.dungeonList && (handlers.dungeons.dungeonList = []);
    handlers.fishing?.Clear?.();
    handlers.harvestables?.Clear?.();
    handlers.mobs?.Clear?.();
    handlers.players?.Clear?.();
    handlers.wispCage?.Clear?.();
}

async function initRadar(offscreenCanvases) {
    try {
        await DatabaseLoader.load();

        drawingUtils = new DrawingUtils();
        map = new MapH(-1);
        emitter = new EventEmitter();
        window.eventEmitter = emitter;

        handlers.dungeons = new DungeonsHandler();
        handlers.chests = new ChestsHandler();
        handlers.mobs = new MobsHandler();
        handlers.harvestables = new HarvestablesHandler(handlers.mobs);
        handlers.players = new PlayersHandler();
        handlers.wispCage = new WispCageHandler();
        handlers.fishing = new FishingHandler();

        drawings.maps = new MapDrawing();
        drawings.harvestables = new HarvestablesDrawing();
        drawings.mobs = new MobsDrawing();
        drawings.players = new PlayersDrawing();
        drawings.chests = new ChestsDrawing();
        drawings.dungeons = new DungeonsDrawing();
        drawings.wispCage = new WispCageDrawing();
        drawings.fishing = new FishingDrawing();

        window.harvestablesHandler = handlers.harvestables;
        window.mobsHandler = handlers.mobs;
        window.playersHandler = handlers.players;

        Object.values(handlers).forEach(handler => {
            if (handler && typeof handler.init === 'function') {
                handler.init(emitter);
            }
        });

        EventRouter.init({
            emitter,
            map,
            radarRenderer: null,
            getMountedStatusCallback: () => Boolean(handlers.players?.localPlayer?.mounted)
        });

        radarRenderer = createRadarRenderer({
            handlers: {
                harvestablesHandler: handlers.harvestables,
                mobsHandler: handlers.mobs,
                playersHandler: handlers.players,
                chestsHandler: handlers.chests,
                dungeonsHandler: handlers.dungeons,
                wispCageHandler: handlers.wispCage,
                fishingHandler: handlers.fishing
            },
            drawings: {
                mapsDrawing: drawings.maps,
                harvestablesDrawing: drawings.harvestables,
                mobsDrawing: drawings.mobs,
                playersDrawing: drawings.players,
                chestsDrawing: drawings.chests,
                dungeonsDrawing: drawings.dungeons,
                wispCageDrawing: drawings.wispCage,
                fishingDrawing: drawings.fishing
            },
            drawingUtils
        });

        radarRenderer.initialize(offscreenCanvases);
        radarRenderer.setMap(map);
        window.radarRenderer = radarRenderer;
        EventRouter.setRadarRenderer(radarRenderer);
        radarRenderer.start();

        postMessage({ type: 'initialized' });
    } catch (error) {
        window.logger.error(CATEGORIES.SYSTEM, 'WorkerInitFailed', {error: error.message});
        postMessage({ type: 'error', error: error.message });
    }
}

function handleMessage(event) {
    const data = event.data;

    if (data.type === 'init') {
        workerSettings = data.settings;
        initRadar(data.canvases);
    } else if (data.type === 'settingsUpdate') {
        workerSettings = data.settings;
    } else if (data.type === 'canvasSizeChanged' && radarRenderer) {
        const newSize = data.size;
        Object.values(radarRenderer.canvasManager.canvases).forEach(canvas => {
            if (canvas) {
                canvas.width = newSize;
                canvas.height = newSize;
            }
        });
        radarRenderer.canvasManager.setupOurPlayerCanvas();
    } else if (data.type === 'websocket_message') {
        const messageType = data.messageType;
        const params = data.params;
        switch (messageType) {
            case 'request':
                EventRouter.onRequest(params);
                break;
            case 'event':
                EventRouter.onEvent(params);
                break;
            case 'response':
                EventRouter.onResponse(params, () => clearHandlers());
                break;
        }
    } else if (data.type === 'clearHandlers') {
        clearHandlers();
    } else if (data.type === 'destroy') {
        if (radarRenderer) radarRenderer.stop();
        clearHandlers();
        close();
    }
}

addEventListener('message', handleMessage);
