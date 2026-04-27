package photon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
)

type ParserRegressionOutput struct {
	Events    []map[string]interface{} `json:"events"`
	Requests  []map[string]interface{} `json:"requests"`
	Responses []map[string]interface{} `json:"responses"`
}

func sanitizeMap(in map[byte]interface{}) map[string]interface{} {
	out := make(map[string]interface{})
	for k, v := range in {
		out[fmt.Sprintf("%d", k)] = sanitizeValue(v)
	}
	return out
}

func sanitizeValue(v interface{}) interface{} {
	switch val := v.(type) {
	case map[interface{}]interface{}:
		out := make(map[string]interface{})
		for mk, mv := range val {
			out[fmt.Sprintf("%v", mk)] = sanitizeValue(mv)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(val))
		for i, arrV := range val {
			out[i] = sanitizeValue(arrV)
		}
		return out
	case map[byte]interface{}:
		return sanitizeMap(val)
	default:
		return v
	}
}

func TestParserRegression(t *testing.T) {
	testdataDir := filepath.Join("testdata")
	pcapFiles, err := filepath.Glob(filepath.Join(testdataDir, "*.pcap"))
	require.NoError(t, err)

	for _, pcapFile := range pcapFiles {
		t.Run(filepath.Base(pcapFile), func(t *testing.T) {
			output := ParserRegressionOutput{
				Events:    []map[string]interface{}{},
				Requests:  []map[string]interface{}{},
				Responses: []map[string]interface{}{},
			}

			parser := NewPhotonParser(
				func(ev *EventData) {
					output.Events = append(output.Events, map[string]interface{}{
						"Code":       ev.Code,
						"Parameters": sanitizeMap(ev.Parameters),
					})
				},
				func(req *OperationRequest) {
					output.Requests = append(output.Requests, map[string]interface{}{
						"OperationCode": req.OperationCode,
						"Parameters":    sanitizeMap(req.Parameters),
					})
				},
				func(resp *OperationResponse) {
					output.Responses = append(output.Responses, map[string]interface{}{
						"OperationCode": resp.OperationCode,
						"ReturnCode":    resp.ReturnCode,
						"DebugMessage":  resp.DebugMessage,
						"Parameters":    sanitizeMap(resp.Parameters),
					})
				},
			)

			f, err := os.Open(pcapFile)
			require.NoError(t, err)
			defer f.Close()

			reader, err := pcapgo.NewReader(f)
			require.NoError(t, err)

			for {
				data, _, err := reader.ReadPacketData()
				if err != nil {
					break // EOF or error
				}
				packet := gopacket.NewPacket(data, layers.LayerTypeEthernet, gopacket.Default)
				udpLayer := packet.Layer(layers.LayerTypeUDP)
				if udpLayer != nil {
					udp := udpLayer.(*layers.UDP)
					parser.ReceivePacket(udp.Payload)
				}
			}

			goldenFile := pcapFile + ".golden.json"
			outputBytes, err := json.MarshalIndent(output, "", "  ")
			require.NoError(t, err)

			if os.Getenv("UPDATE_GOLDEN") == "1" {
				err = os.WriteFile(goldenFile, outputBytes, 0644)
				require.NoError(t, err)
			} else {
				goldenBytes, err := os.ReadFile(goldenFile)
				if os.IsNotExist(err) {
					t.Skipf("Golden file %s not found. Run with UPDATE_GOLDEN=1 to create it", goldenFile)
				}
				require.NoError(t, err)

				require.JSONEq(t, string(goldenBytes), string(outputBytes), "Parser output differs from golden file for %s", pcapFile)
			}
		})
	}
}
