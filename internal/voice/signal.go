package voice

import (
	"bytes"
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

// HideAddresses keeps a device's network addresses out of a voice_signal, so
// joining a call teaches nobody where the people in it are. Current apps
// gather only relay candidates; this also holds for older ones, which gather
// everything. A candidate that is not a relay's, or cannot be read as one,
// stops the whole signal (false). A relay candidate keeps its relay's address
// and loses the related address, which can be the device's own. A description
// loses direct candidates and its default addresses. A payload that says
// nothing of candidates is passed on as it came.
func HideAddresses(raw json.RawMessage) (json.RawMessage, bool) {
	// an escaped key can spell one of the names too
	if !bytes.Contains(raw, []byte("candidate")) && !bytes.Contains(raw, []byte("sdp")) && !bytes.Contains(raw, []byte(`\u`)) {
		return raw, true
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil || m == nil {
		return raw, true
	}
	changed := false
	if c, ok := m["candidate"]; ok && !bytes.Equal(bytes.TrimSpace(c), []byte("null")) {
		out, keep, moved := hideInObject(c, "candidate", func(line string) (string, bool) {
			if line == "" { // the end of the candidates
				return line, true
			}
			return relayCandidate(line)
		})
		if !keep {
			return nil, false
		}
		if moved {
			m["candidate"] = out
			changed = true
		}
	}
	if d, ok := m["description"]; ok && !bytes.Equal(bytes.TrimSpace(d), []byte("null")) {
		out, keep, moved := hideInObject(d, "sdp", func(sdp string) (string, bool) { return hideInSDP(sdp), true })
		if !keep {
			return nil, false
		}
		if moved {
			m["description"] = out
			changed = true
		}
	}
	if !changed {
		return raw, true
	}
	out, err := json.Marshal(m)
	if err != nil {
		return nil, false
	}
	return out, true
}

// hideInObject runs clean over one string field of a JSON object. keep is
// false for anything that is not such an object; moved says the text changed.
func hideInObject(raw json.RawMessage, field string, clean func(string) (string, bool)) (out json.RawMessage, keep, moved bool) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil || obj == nil {
		return nil, false, false
	}
	v, ok := obj[field]
	if !ok {
		return raw, true, false
	}
	var text string
	if err := json.Unmarshal(v, &text); err != nil || bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
		return nil, false, false
	}
	cleaned, keep := clean(text)
	if !keep {
		return nil, false, false
	}
	if cleaned == text {
		return raw, true, false
	}
	encoded, err := json.Marshal(cleaned)
	if err != nil {
		return nil, false, false
	}
	obj[field] = encoded
	if out, err = json.Marshal(obj); err != nil {
		return nil, false, false
	}
	return out, true, true
}

// relayCandidate returns an ICE candidate line with its related address
// removed, and false when the line is not a relay's candidate.
func relayCandidate(line string) (string, bool) {
	f := strings.Fields(line)
	if len(f) < 8 || f[6] != "typ" || f[7] != "relay" {
		return "", false
	}
	moved := false
	for i := 8; i+1 < len(f); i += 2 {
		blank := map[string]string{"raddr": "0.0.0.0", "rport": "0"}[f[i]]
		if blank != "" && f[i+1] != blank {
			f[i+1] = blank
			moved = true
		}
	}
	if !moved {
		return line, true
	}
	return strings.Join(f, " "), true
}

// hideInSDP drops direct candidates from a session description and blanks its
// default addresses. Line endings stay as they came.
func hideInSDP(sdp string) string {
	lines := strings.Split(sdp, "\n")
	out := make([]string, 0, len(lines))
	for _, raw := range lines {
		line, cr := strings.CutSuffix(raw, "\r")
		switch {
		case strings.HasPrefix(line, "a=candidate:"):
			kept, ok := relayCandidate(line)
			if !ok {
				continue
			}
			line = kept
		case strings.HasPrefix(line, "c=IN IP4 "):
			line = "c=IN IP4 0.0.0.0"
		case strings.HasPrefix(line, "c=IN IP6 "):
			line = "c=IN IP6 ::"
		case strings.HasPrefix(line, "a=rtcp:"):
			if f := strings.Fields(line); len(f) == 4 && f[1] == "IN" {
				f[3] = map[string]string{"IP4": "0.0.0.0", "IP6": "::"}[f[2]]
				if f[3] != "" {
					line = strings.Join(f, " ")
				}
			}
		}
		if cr {
			line += "\r"
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// signalNumbers are the numbers a voice_signal can carry from one participant
// to another, with the largest each may be: a listener's loss report through
// the SFU (web/src/voice/sfu.ts), the share of a voice lost and concealed and
// how long it is held for playback, in milliseconds, over a stretch of time.
// The server passes a signal on without reading it otherwise; these are held
// to their range on the way, so no client is handed an absurd number.
var signalNumbers = map[string]float64{
	"voice_loss":    1,
	"voice_conceal": 1,
	"voice_delay":   60_000,
	"voice_span":    60_000,
}

// CleanSignal returns a voice_signal payload fit to relay: each number above
// held between 0 and its most, and dropped when it is not a number at all.
// Anything else in it, and a payload that is not a JSON object, is passed on
// as it came.
func CleanSignal(raw json.RawMessage) json.RawMessage {
	// an escaped key can spell one of the names too, so either sends it through the check
	if !bytes.Contains(raw, []byte("voice_")) && !bytes.Contains(raw, []byte(`\u`)) {
		return raw
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil || m == nil {
		return raw
	}
	changed := false
	for name, most := range signalNumbers {
		v, ok := m[name]
		if !ok {
			continue
		}
		var f float64
		if bytes.Equal(bytes.TrimSpace(v), []byte("null")) || json.Unmarshal(v, &f) != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			delete(m, name)
			changed = true
			continue
		}
		if held := min(most, max(0, f)); held != f {
			m[name] = json.RawMessage(strconv.FormatFloat(held, 'f', -1, 64))
			changed = true
		}
	}
	if !changed {
		return raw
	}
	out, err := json.Marshal(m)
	if err != nil {
		return raw
	}
	return out
}
