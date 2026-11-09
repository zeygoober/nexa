package voice

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func decode(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("relayed payload is not JSON: %v (%s)", err, raw)
	}
	return m
}

func TestCleanSignalPassesOrdinarySignalsUntouched(t *testing.T) {
	for _, in := range []string{
		`{"description":{"type":"offer","sdp":"v=0\r\n"},"streams":{"audio":"a","video":"b"}}`,
		`{"candidate":null}`,
		`{"voice_loss":0.05,"voice_conceal":0.02,"voice_delay":180,"voice_span":6000}`,
		`{"guest_keys":"x"}`,
		`[1,2,3]`,
		`null`,
		`"text"`,
	} {
		if got := CleanSignal(json.RawMessage(in)); string(got) != in {
			t.Errorf("CleanSignal(%s) = %s, want it unchanged", in, got)
		}
	}
}

func TestCleanSignalHoldsNumbersToTheirRange(t *testing.T) {
	got := decode(t, CleanSignal(json.RawMessage(`{"voice_loss":1e300,"voice_conceal":-4,"voice_delay":1e308,"voice_span":123456789,"description":{"sdp":"v=0"}}`)))
	want := map[string]float64{"voice_loss": 1, "voice_conceal": 0, "voice_delay": 60_000, "voice_span": 60_000}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v, want %v", k, got[k], v)
		}
	}
	if d, ok := got["description"].(map[string]any); !ok || d["sdp"] != "v=0" {
		t.Errorf("the rest of the signal was not kept: %v", got)
	}
}

func TestCleanSignalDropsWhatIsNotANumber(t *testing.T) {
	got := decode(t, CleanSignal(json.RawMessage(`{"voice_loss":"0.5","voice_conceal":null,"voice_delay":{"a":1},"voice_span":1e999,"other":2}`)))
	for _, k := range []string{"voice_loss", "voice_conceal", "voice_delay", "voice_span"} {
		if _, ok := got[k]; ok {
			t.Errorf("%s was relayed: %v", k, got[k])
		}
	}
	if got["other"] != float64(2) {
		t.Errorf("other = %v, want 2", got["other"])
	}
}

func TestCleanSignalSeesThroughEscapedNames(t *testing.T) {
	got := decode(t, CleanSignal(json.RawMessage(`{"voice_loss":5e10}`)))
	if got["voice_loss"] != float64(1) {
		t.Errorf("voice_loss = %v, want 1", got["voice_loss"])
	}
}

func TestHideAddressesKeepsOnlyRelayCandidates(t *testing.T) {
	for _, in := range []string{
		`{"candidate":{"candidate":"candidate:1 1 udp 2122 192.168.1.5 5000 typ host generation 0","sdpMid":"0"}}`,
		`{"candidate":{"candidate":"candidate:2 1 udp 1686 203.0.113.9 5000 typ srflx raddr 192.168.1.5 rport 5000","sdpMid":"0"}}`,
		`{"candidate":{"candidate":"candidate:3 1 udp 1686 203.0.113.9 5000 typ prflx","sdpMid":"0"}}`,
		`{"candidate":{"candidate":"203.0.113.9"}}`,
		`{"candidate":"candidate:1 1 udp 2122 192.168.1.5 5000 typ host"}`,
		`{"candidate":{"candidate":"candidate:1 1 udp 2122 192.168.1.5 5000 typ host"}}`,
	} {
		if out, ok := HideAddresses(json.RawMessage(in)); ok {
			t.Errorf("HideAddresses(%s) passed %s", in, out)
		}
	}
	for _, in := range []string{
		`{"candidate":null}`,
		`{"candidate":{"candidate":""}}`,
		`{"voice_loss":0.05}`,
		`{"guest_keys":"x"}`,
	} {
		if out, ok := HideAddresses(json.RawMessage(in)); !ok || string(out) != in {
			t.Errorf("HideAddresses(%s) = %s, %v, want it unchanged", in, out, ok)
		}
	}
}

// An older app's relay candidate names the device's own public address as its
// related address, which is the same leak in a smaller field.
func TestHideAddressesClearsARelaysRelatedAddress(t *testing.T) {
	in := `{"candidate":{"candidate":"candidate:4 1 udp 41 198.51.100.7 6000 typ relay raddr 203.0.113.9 rport 5000 generation 0","sdpMid":"0"}}`
	out, ok := HideAddresses(json.RawMessage(in))
	if !ok {
		t.Fatal("a relay candidate was stopped")
	}
	if bytes.Contains(out, []byte("203.0.113.9")) || !bytes.Contains(out, []byte("198.51.100.7 6000 typ relay raddr 0.0.0.0 rport 0")) {
		t.Fatalf("got %s", out)
	}
}

func TestHideAddressesCleansDescriptions(t *testing.T) {
	sdp := "v=0\r\nm=audio 5000 UDP/TLS/RTP/SAVPF 111\r\nc=IN IP4 203.0.113.9\r\na=rtcp:5000 IN IP4 203.0.113.9\r\n" +
		"a=candidate:1 1 udp 2122 192.168.1.5 5000 typ host\r\na=candidate:2 1 udp 1686 203.0.113.9 5000 typ srflx raddr 192.168.1.5 rport 5000\r\n" +
		"a=candidate:4 1 udp 41 198.51.100.7 6000 typ relay raddr 203.0.113.9 rport 5000\r\nc=IN IP6 2001:db8::1\r\n"
	body, _ := json.Marshal(map[string]any{"description": map[string]string{"type": "offer", "sdp": sdp}, "streams": map[string]string{"audio": "a"}})
	out, ok := HideAddresses(body)
	if !ok {
		t.Fatal("a description was stopped")
	}
	got := decode(t, out)["description"].(map[string]any)["sdp"].(string)
	for _, gone := range []string{"203.0.113.9", "192.168.1.5", "2001:db8::1", "typ host", "typ srflx"} {
		if strings.Contains(got, gone) {
			t.Errorf("%q survived in\n%s", gone, got)
		}
	}
	if !strings.Contains(got, "198.51.100.7 6000 typ relay") || !strings.HasSuffix(got, "\r\n") {
		t.Errorf("the relay candidate or line endings went missing:\n%s", got)
	}
}
