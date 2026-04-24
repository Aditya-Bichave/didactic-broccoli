# Deep Repository Audit Report

## 1. Repository Understanding

- **Project type:** Local network sniffer and visual radar for the game Albion Online, bundled with active botting/automation tools.
- **Tech stack:** Go (1.26 backend for packet capture, HTTP, WebSocket, gathering automation) and JavaScript/HTML/CSS (Frontend utilizing HTMX, TailwindCSS v4, DaisyUI, and native DOM APIs). Node.js and Python are present for tooling and experimental AI bots.
- **Main purpose:** To parse Albion Online's custom Photon protocol over UDP, extract game entity information (players, mobs, resources), and display them on a web-based UI radar to give the user a tactical advantage without modifying the game client memory.
- **Main entry points:** `cmd/radar/main.go` for the Go backend, `web/scripts/core/PageController.js` and `EventRouter.js` for the frontend.
- **Important folders:**
  - `cmd/radar/`: Entry point.
  - `internal/`: Core logic (`photon/` for decoding, `server/` for HTTP/WS, `capture/` for pcap).
  - `internal/gather/`: Win32-based active botting logic.
  - `web/`: Frontend static assets, UI logic, rendering, and handlers.
  - `external/`: Python-based experimental YOLO bots.
- **Testing setup:** Unit tests via standard `go test` and Vitest (`npm run test`). E2E coverage is missing.
- **Build/deployment setup:** GNU Make, Docker for cross-compilation, static assets compiled into the Go binary via `go:embed`.
- **Key dependencies:** `gopacket` (packet sniffing), `gorilla/websocket`, Tailwind CSS, HTMX, DaisyUI.
- **Assumptions made:** The repository aims to be a purely passive radar, but it actively contradicts this by shipping Windows-specific active input automation (`internal/gather`). The application runs locally on `localhost:5001`.

## 2. Top 5 Bugs or Defects

1. **Title:** Local Player Position Interpolation Blocking
   - **Severity:** High
   - **Confidence:** High
   - **Location:** `web/scripts/core/EventRouter.js` (lines around `updateLocalPlayerPosition` and `computeInterpolatedPosition`).
   - **Evidence:** The local player's authoritative position is flagged via `hasAuthoritativeLocalMove = true;`. However, inside `computeInterpolatedPosition`, if `hasAuthoritativeLocalMove` is true, it immediately returns `{x: lpX, y: lpY}` and stops interpolating future movements. Later, `hasAuthoritativeLocalMove = false;` is only reset in the `EventCodes.Leave` handler or scattered locations, resulting in frozen local player radar updates.
   - **Why this is a bug:** It breaks the core radar functionality. Once an authoritative move is registered, interpolation freezes.
   - **How it can fail in real usage:** The player's dot on the map stops moving dynamically.
   - **Suggested fix:** Reset `hasAuthoritativeLocalMove` appropriately after the tick, or adjust the `computeInterpolatedPosition` logic to recalculate based on the newest authoritative coordinates rather than halting interpolation entirely.
   - **Suggested test case:** Unit test `EventRouter.js` to trigger an authoritative move, step time forward, and assert that interpolation continues or resumes smoothly on the next move intent.
   - **Estimated effort:** Medium

2. **Title:** Resources Missing from Map Due to Incorrect Parameter Parsing
   - **Severity:** High
   - **Confidence:** High
   - **Location:** `web/scripts/handlers/HarvestablesHandler.js` in `newHarvestableObject` and `newSimpleHarvestableObject`.
   - **Evidence:** In `newHarvestableObject` (Event 40), `type` is hardcoded as `Parameters[5]`. In `newSimpleHarvestableObject` (Event 38), it accesses `a1[i]` for type ID. However, living resource objects and static objects diverge in their event signatures.
   - **Why this is a bug:** The radar is entirely missing certain resources because the parser attempts to map undefined or incorrectly indexed parameters (like `Parameters[5]`) to the database.
   - **How it can fail in real usage:** Resources fail to render on the radar.
   - **Suggested fix:** Re-align the indices for Event 38 and Event 40 by capturing a fresh packet dump and verifying the parameter map (e.g., `Parameters[5]` vs `Parameters[6]`). Ensure `mobileTypeId` vs `type` are parsed correctly based on the specific packet structure.
   - **Suggested test case:** Send mock Event 38 and Event 40 packets with known offsets and verify the `Harvestable` object is instantiated with the correct type.
   - **Estimated effort:** Small

3. **Title:** Legacy `OperationCodes.Move` Handling with Magic Numbers
   - **Severity:** Medium
   - **Confidence:** High
   - **Location:** `web/scripts/core/EventRouter.js:400`
   - **Evidence:** `if (Parameters[253] == 21 || Parameters[253] == OperationCodes.Move)`. As documented in the code comment, `21` is legacy and upstream `21` is now `GetShopTilesForCategory`.
   - **Why this is a bug:** If the user triggers `GetShopTilesForCategory`, the router will interpret it as a local player move request and parse parameters incorrectly, causing a crash or erratic behavior.
   - **How it can fail in real usage:** The radar throws errors or jumps the local player position when the player opens the in-game shop.
   - **Suggested fix:** Remove the legacy `21` check. Rely strictly on `OperationCodes.Move`.
   - **Suggested test case:** Inject an `OperationCodes.GetShopTilesForCategory` (21) request and verify the `EventRouter` ignores it rather than processing it as movement.
   - **Estimated effort:** Small

4. **Title:** Memory Leak in Stale Entities (Dungeons and Chests)
   - **Severity:** Medium
   - **Confidence:** High
   - **Location:** `web/scripts/handlers/DungeonsHandler.js` and `ChestsHandler.js`.
   - **Evidence:** `TODO.md` confirms that `DungeonsHandler.js` and `ChestsHandler.js` lack a stale entity cleanup mechanism (unlike `PlayersHandler` and `MobsHandler`).
   - **Why this is a bug:** Long gaming sessions will continuously accumulate dungeons and chests in the Javascript memory arrays, degrading UI rendering performance and leaking memory.
   - **How it can fail in real usage:** App memory grows boundlessly and frames drop after hours of mapping.
   - **Suggested fix:** Implement a `removeNotInRange` or `garbageCollect` loop for these handlers that prunes entities exceeding a maximum distance threshold from the local player.
   - **Suggested test case:** Spawn 10,000 mock chests out of range, run the GC tick, and assert the chest array length is bounded.
   - **Estimated effort:** Medium

5. **Title:** Missing API Body Close in Go Backend
   - **Severity:** Low
   - **Confidence:** High
   - **Location:** `internal/server/gather_api.go` (`decodeBody`)
   - **Evidence:** The standard library `json.NewDecoder(r.Body).Decode(&req)` is called, but `r.Body.Close()` is never invoked after.
   - **Why this is a bug:** While the Go HTTP server often cleans up the body for simple requests, it's a best practice and requirement for avoiding connection/resource leaks under heavy API stress.
   - **How it can fail in real usage:** Potential file descriptor/socket leak if the Gather API is spammed or kept alive.
   - **Suggested fix:** Add `defer r.Body.Close()` at the top of the request handlers or inside `decodeBody`.
   - **Suggested test case:** Use `golangci-lint` with `bodyclose` enabled on the `server` package to catch this.
   - **Estimated effort:** Small

## 3. Top 5 Enhancements

1. **Title:** Extract the Automation (Gathering Bot) into a Separate Tool
   - **Impact:** High
   - **Location or affected area:** `internal/gather/` and `external/`
   - **Current behavior:** The "passive" radar repo includes active Windows API key/mouse injection botting.
   - **Recommended improvement:** Decouple the Win32 automation into a completely separate repository or opt-in binary.
   - **Why it improves the repo:** Aligns the repository with its README claims ("Passive network capture", "Zero injection"). Mixing a passive radar with active anti-cheat detectable macro bots endangers the pure radar users.
   - **Implementation idea:** Move `internal/gather` to a standalone Go module that listens to the radar's WebSocket for coordinates.
   - **Estimated effort:** Large

2. **Title:** Humanize Gathering Automation Timings
   - **Impact:** High
   - **Location or affected area:** `internal/gather/service.go` and `internal/gather/platform_windows.go`
   - **Current behavior:** Movement, mounting, and interactions are sent to Win32 APIs with exact, static delays (`time.Sleep(time.Duration(calibration.MountDelayMs) * time.Millisecond)`).
   - **Recommended improvement:** Introduce jitter/randomization to the timings.
   - **Why it improves the repo:** Static, identical millisecond sleep intervals are easily flagged by simple anti-cheat heuristic checks.
   - **Implementation idea:** Add a configurable `jitterMs` setting, using `rand.Intn` to modify sleep durations and click durations.
   - **Estimated effort:** Small

3. **Title:** Map Scaling and Decryption Heuristics
   - **Impact:** Medium
   - **Location or affected area:** Map rendering (`web/scripts/drawings/`)
   - **Current behavior:** Player movement coordinates are encrypted by the game servers, rendering other players as static dots or at 0,0.
   - **Recommended improvement:** Implement an estimated dead-reckoning heuristic.
   - **Why it improves the repo:** Players are currently static. If the sniffer reads their initial spawn, it could predict paths based on zone chokepoints or average speeds.
   - **Implementation idea:** While true decryption is blocked, calculate rough zone regions based on sound/event distances if available.
   - **Estimated effort:** Large

4. **Title:** Full Mists Handler Integration
   - **Impact:** Medium
   - **Location or affected area:** `web/scripts/handlers/MistsHandler.js` (missing/broken)
   - **Current behavior:** Mists events (513-531) are undefined and ignored.
   - **Recommended improvement:** Implement the event handlers and UI elements for the Mists.
   - **Why it improves the repo:** The Mists are highly competitive. Radar users have a blind spot here.
   - **Implementation idea:** Build `MistsHandler.js` and map the incoming Wisp and Mist portal coordinates to the canvas.
   - **Estimated effort:** Medium

5. **Title:** Configurable Port and IP via Environment Variables
   - **Impact:** Low
   - **Location or affected area:** `cmd/radar/main.go`
   - **Current behavior:** Port `5001` is hardcoded.
   - **Recommended improvement:** Support `PORT` and `HOST` env vars, falling back to 5001.
   - **Why it improves the repo:** Allows users with conflicting local services (e.g., Flask/React default ports) to run the radar effortlessly.
   - **Implementation idea:** Use `os.Getenv("PORT")` during HTTP server initialization.
   - **Estimated effort:** Small

## 4. Top 5 Architectural Improvements

1. **Title:** Hard Boundary Between Radar and Input Injection
   - **Current architectural limitation:** `internal/gather` tightly couples the radar's state machine with operating system input.
   - **Evidence from repo structure or code:** `service.go` imports `logger` and runs alongside the `server` inside `main.go`.
   - **Recommended architecture change:** Move the input injection into an RPC-based or WebSocket-client microservice.
   - **Benefits:** Keeps the main sniffer pure, safe from automated bans that look for `user32.dll` hooks.
   - **Tradeoffs:** Adds deployment complexity (two binaries).
   - **Migration approach:** Create a standard REST/WS API on the radar, and build a Python/Go client that reads the API to perform the clicks.
   - **Risk level:** Low
   - **Estimated effort:** Medium

2. **Title:** True State Container for Frontend
   - **Current architectural limitation:** State is scattered globally (`lpX`, `lpY`, `window.wsConnectionStatus`) and mutated across disparate modules.
   - **Evidence from repo structure or code:** `EventRouter.js` exports floating variables and mutates global state freely.
   - **Recommended architecture change:** Adopt a lightweight centralized store (like a basic Redux pattern or Zustand-equivalent for vanilla JS/HTMX).
   - **Benefits:** Predictable state, easier to debug, prevents bugs like the current `hasAuthoritativeLocalMove` freeze.
   - **Tradeoffs:** Requires refactoring the handlers.
   - **Migration approach:** Introduce a `Store.js` class with a pub/sub model for radar coordinates.
   - **Risk level:** Medium
   - **Estimated effort:** Large

3. **Title:** Dedicated Type Definitions/Interfaces for Packets
   - **Current architectural limitation:** Hardcoded index lookups for packet parameters (`Parameters[253]`, `Parameters[9]`) are fragile.
   - **Evidence from repo structure or code:** `EventRouter.js` is riddled with magic numbers and `Array.isArray` checks.
   - **Recommended architecture change:** Create an abstraction layer that maps raw parameter indices to typed objects (e.g., `interface MovePacket { x: number, y: number }`).
   - **Benefits:** Strong typing protects against Albion protocol updates. If indices change, only the mapper needs updating.
   - **Tradeoffs:** Slight memory overhead for object creation.
   - **Migration approach:** Create packet mappers incrementally per `EventCode`.
   - **Risk level:** Low
   - **Estimated effort:** Medium

4. **Title:** Worker Threads for the Canvas Renderer
   - **Current architectural limitation:** The main JS thread parses WebSocket data and renders the canvas.
   - **Evidence from repo structure or code:** `radarRenderer` is invoked directly from the WebSocket `EventRouter.js`.
   - **Recommended architecture change:** Offload WebSocket parsing and state management to a Web Worker, sending only `OffscreenCanvas` render commands to the main thread.
   - **Benefits:** Completely eliminates UI stutter during massive Black Zone cluster battles.
   - **Tradeoffs:** Browser compatibility (mostly fine in modern browsers), complexity of message passing.
   - **Migration approach:** Move `EventRouter.js` and Handlers to a Worker, use `postMessage` for UI updates.
   - **Risk level:** High
   - **Estimated effort:** Large

5. **Title:** Extensible Plugin System for Data Layers
   - **Current architectural limitation:** Adding a new category (e.g., Fishing, Mists) requires modifying the core `EventRouter.js`.
   - **Evidence from repo structure or code:** The switch statement in `EventRouter.js` grows endlessly.
   - **Recommended architecture change:** A Pub/Sub or Plugin architecture where `FishingHandler` subscribes to `EventCodes.NewFishingZoneObject`.
   - **Benefits:** Modular codebase. Easy to toggle entire features on/off.
   - **Tradeoffs:** Minor performance cost in event routing.
   - **Migration approach:** Implement an `EventEmitter` and have handlers register themselves.
   - **Risk level:** Low
   - **Estimated effort:** Medium

## 5. Top 5 New Feature Ideas

1. **Feature name:** Webhook / Discord Integration for Hostile Alerts
   - **Why this feature would make the repo better:** Users afk gathering want notifications if an enemy enters the zone.
   - **User or developer value:** High user convenience and peace of mind when partially AFK.
   - **Where it would fit in the current repo:** `PlayersHandler.js` logic and settings page.
   - **Suggested implementation approach:** A simple configuration tab to input a Discord webhook URL. When `PlayerHandler` flags a hostile, issue an HTTP POST to Discord.
   - **Dependencies or prerequisites:** Needs HTTP request capabilities (fetch) configured locally without CORS issues.
   - **MVP scope:** Send basic "Hostile player in zone: <MapName>" text payload.
   - **Future expansion idea:** Include a mini-screenshot or exact coordinates of the ping.

2. **Feature name:** Historical Heatmap Tracking
   - **Why this feature would make the repo better:** Finding optimal gathering routes requires knowing where resources frequently spawn.
   - **User or developer value:** Massive economic advantage for gatherers plotting routes.
   - **Where it would fit in the current repo:** `HarvestablesHandler.js` and a new heatmap canvas layer.
   - **Suggested implementation approach:** Persist resource spawn coordinates locally (e.g., SQLite via Go, or IndexedDB in the browser).
   - **Dependencies or prerequisites:** An on-disk persistence layer.
   - **MVP scope:** Log the `x,y` of T7+ resources and paint red hotspots on the radar map.
   - **Future expansion idea:** Share heatmaps via a centralized server across users.

3. **Feature name:** Squad / Party Sharing
   - **Why this feature would make the repo better:** Guilds want to share their radar data for larger vision.
   - **User or developer value:** Tactical advantage for small-scale PvP.
   - **Where it would fit in the current repo:** `internal/server` and `WebSocketHandler`.
   - **Suggested implementation approach:** Connect multiple OpenRadar clients via a centralized relay server to merge datasets and rebroadcast.
   - **Dependencies or prerequisites:** A remote, cloud-hosted instance of OpenRadar or a stripped down relay server.
   - **MVP scope:** A secure token exchange to send `EventData` between two IP addresses.
   - **Future expansion idea:** Build an alliance-wide mega-radar view.

4. **Feature name:** Resource Value Estimator
   - **Why this feature would make the repo better:** Inventory fills up quickly; users want to prioritize the highest silver/hour.
   - **User or developer value:** Increased gathering efficiency and profit.
   - **Where it would fit in the current repo:** UI tooltips in the `radarRenderer`.
   - **Suggested implementation approach:** Fetch pricing data from the Albion Data Project API and overlay estimated silver values on the map icons.
   - **Dependencies or prerequisites:** Internet access on the machine running the radar to hit the external API.
   - **MVP scope:** Pull the 24h average price for resources and overlay a `$` or `$$$` icon based on value.
   - **Future expansion idea:** Calculate the full silver value of a dungeon chest before opening it.

5. **Feature name:** Audio Alert Customization
   - **Why this feature would make the repo better:** The default flash/sound might be annoying or ignored.
   - **User or developer value:** Better UX and accessibility.
   - **Where it would fit in the current repo:** Settings page and `sounds/` directory.
   - **Suggested implementation approach:** Let users upload local `.mp3` files or select different alert tones based on the threat level (Boss vs Hostile Player).
   - **Dependencies or prerequisites:** File upload capability in the Go backend.
   - **MVP scope:** Add a dropdown menu with 5 pre-packaged alert sounds.
   - **Future expansion idea:** Text-to-speech alerts ("Hostile player approached from North").

## 6. Testing and Quality Gaps

- **Missing E2E Tests:** There are unit tests for handlers, but no Puppeteer/Playwright E2E tests validating that a packet arriving at the Go server correctly results in a canvas draw on the frontend.
- **Untested Critical Paths:** The `internal/gather` platform-specific Win32 logic is excluded from standard tests (mocked out in `gather_api_test.go`).
- **Missing Integration Packets:** The repo includes a `tools/photon-dump` tool, but there is no automated regression test suite that feeds historical PCAP dumps through the parser to ensure updates don't break existing parsing.

## 7. Security and Dependency Risks

- **CSWSH / CSRF Vulnerability:** In `internal/server/websocket.go`, the `CheckOrigin` function unconditionally returns `true`. If a user is running OpenRadar on `localhost:5001`, a malicious external website can open a WebSocket connection to `ws://localhost:5001/ws` or hit the Gather API, hijacking the user's radar data or triggering automated gathering inputs (which executes native Win32 clicks).
- **Admin/Root Requirement:** Using `pcap` inherently requires elevated privileges. A vulnerability in the HTTP server (like the CSWSH above) while running as root/Admin is highly dangerous.
- **Dependencies:** Ensure `gopacket` and frontend tools are pinned and regularly audited via `govulncheck` and `npm audit`.

## 8. Performance and Scalability Risks

- **Memory Leaks:** Unbounded arrays for Chests and Dungeons (as noted in Bugs) will eventually crash the tab.
- **Garbage Collection Pressure:** The Go backend's `WebSocketHandler` broadcasts pointers to `interface{}`. High packet volume will churn the GC. While batched every 16ms, thousands of entities will still strain CPU serialization.
- **Frontend Canvas Bottleneck:** Rendering thousands of entities on a single 2D Canvas every frame is not scalable. Switching to WebGL or offscreen culling is necessary for massive ZvZ battles.

## 9. Final Priority Roadmap

### Immediate Fixes
1. Secure the WebSocket and HTTP endpoints by enforcing strict `Origin` checks (fix `CheckOrigin`).
2. Fix the `hasAuthoritativeLocalMove` freeze bug in `EventRouter.js` to restore player movement.
3. Fix the array indexing bugs for Event 38 and Event 40 in `HarvestablesHandler.js` to restore resource rendering.

### Short-Term Improvements
1. Implement stale entity cleanup for Dungeons and Chests.
2. Remove legacy logic like `OperationCodes.Move == 21`.
3. Add jitter to the Win32 automation logic to avoid trivial bans.

### Medium-Term Architecture Work
1. Transition the `internal/gather` logic out of the main OpenRadar sniffer binary to protect the "passive" nature of the tool.
2. Refactor `EventRouter.js` to use an Event/Pub-Sub model instead of a massive switch block.

### Feature Roadmap
1. Humanized Jitter/Randomization for gathering.
2. Discord Webhook integration.
3. Historical resource heatmap.
4. Mists full implementation.
5. Resource silver-value estimator overlay.

---

## Commands Run
- `ls -la internal/photon/`, `cat web/scripts/core/EventRouter.js` (tracked local movement variables).
- `grep -rn "TODO"` (revealed legacy and missing features).
- `make test` and `npm test` (verified unit tests pass).
- Examined `internal/server/websocket.go` (found CSWSH).
- Examined `internal/gather/` (found Win32 API usage).
- *Result:* Mapped the core architectural domains and found runtime defects.

## Files Reviewed
- `cmd/radar/main.go`
- `internal/server/websocket.go`, `internal/server/http.go`, `internal/server/gather_api.go`
- `internal/gather/service.go`, `internal/gather/platform_windows.go`
- `web/scripts/core/EventRouter.js`, `web/scripts/core/GatherController.js`
- `web/scripts/handlers/HarvestablesHandler.js`
- `README.md`, `TODO.md`, `Makefile`

## Confidence Summary
- Bugs: High
- Enhancements: High
- Architecture: High
- Features: Medium
- Security: High
- Testing: High
