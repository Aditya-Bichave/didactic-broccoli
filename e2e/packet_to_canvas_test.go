package e2e

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nospy/albion-openradar/internal/logger"
	"github.com/nospy/albion-openradar/internal/photon"
	"github.com/nospy/albion-openradar/internal/server"
	"github.com/stretchr/testify/require"
)

func TestE2EPacketToCanvas(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	log := logger.New("./logs")
	defer log.Stop()

	wsHandler := server.NewWebSocketHandler(log)

	cwd, _ := os.Getwd()
	appDir := filepath.Join(cwd, "..")

	// We use a fixed port for the test, similar to dev
	port := 5001
	httpServer, err := server.NewHTTPServerDev(port, appDir, wsHandler, log, "test-version")
	require.NoError(t, err)

	go func() {
		httpServer.Start()
	}()

	// Cleanup
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		httpServer.Shutdown(ctx)
	}()

	// Give server time to start
	time.Sleep(1 * time.Second)

	script := fmt.Sprintf(`
const puppeteer = require('puppeteer');

(async () => {
	const browser = await puppeteer.launch({
		headless: 'new',
		args: ['--no-sandbox', '--disable-setuid-sandbox']
	});
	const page = await browser.newPage();

	page.on('console', msg => console.log('BROWSER:', msg.text()));

	await page.evaluateOnNewDocument(() => {
		// Mock matchMedia
		window.matchMedia = window.matchMedia || function() {
			return {
				matches: false,
				addListener: function() {},
				removeListener: function() {}
			};
		};

		const originalDrawImage = CanvasRenderingContext2D.prototype.drawImage;
		CanvasRenderingContext2D.prototype.drawImage = function(...args) {
			window.__TEST_DRAW_CALLS = (window.__TEST_DRAW_CALLS || 0) + 1;
			return originalDrawImage.apply(this, args);
		};
		const originalFillRect = CanvasRenderingContext2D.prototype.fillRect;
		CanvasRenderingContext2D.prototype.fillRect = function(...args) {
			window.__TEST_DRAW_CALLS = (window.__TEST_DRAW_CALLS || 0) + 1;
			return originalFillRect.apply(this, args);
		};
		const originalArc = CanvasRenderingContext2D.prototype.arc;
		CanvasRenderingContext2D.prototype.arc = function(...args) {
			window.__TEST_DRAW_CALLS = (window.__TEST_DRAW_CALLS || 0) + 1;
			return originalArc.apply(this, args);
		};

		// Reset count so we ONLY detect draw calls *after* the packet
		window.__TEST_DRAW_CALLS = 0;
	});

	await page.goto('http://localhost:%d');

	await new Promise(r => setTimeout(r, 2000));

	// We clear any draw calls that happened during initial load (e.g. background/grid rendering)
	await page.evaluate(() => { window.__TEST_DRAW_CALLS = 0; });

	console.log("READY_FOR_PACKET");

	try {
		await page.waitForFunction(() => window.__TEST_DRAW_CALLS > 0, { timeout: 10000 });
		console.log("SUCCESS");
	} catch(e) {
		console.error("FAIL: No draw calls detected");
		process.exit(1);
	}

	await browser.close();
})();
`, port)

	scriptPath := filepath.Join(cwd, "test_puppeteer.cjs")
	os.WriteFile(scriptPath, []byte(script), 0644)
	defer os.Remove(scriptPath)

	cmd := exec.Command("node", "test_puppeteer.cjs")

	stdoutPipe, err := cmd.StdoutPipe()
	require.NoError(t, err)
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start puppeteer: %v", err)
	}

	ready := make(chan bool)
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			text := scanner.Text()
			fmt.Println(text)
			if strings.Contains(text, "READY_FOR_PACKET") {
				ready <- true
			}
		}
	}()

	select {
	case <-ready:
		// Puppeteer is ready
	case <-time.After(15 * time.Second):
		t.Fatalf("Timed out waiting for Puppeteer to be ready")
	}

	ev := &photon.EventData{
		Code: 71, // NewMob
		Parameters: map[byte]interface{}{
			0:  int32(999),          // Mob ID
			1:  []int32{12, 13},     // Mob Type id (something)
			8:  []float32{100, 100}, // position
			14: []float32{100, 100}, // position ?
		},
	}
	wsHandler.BroadcastEvent(ev)

	err = cmd.Wait()
	require.NoError(t, err, "Puppeteer script failed (no draw calls or error)")
}
