// Logic of the Frigate plugin, without build tags so it is testable natively.
package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/phoedox/phoedox-plugins/internal/integ"
	"github.com/phoedox/phoedox-sdk/go/pdk"
)

// Config is the part of GET /api/config Phoedox uses. Frigate removes
// credentials from the camera input paths in this answer; the plugin only
// reads host names from them anyway.
type Config struct {
	Cameras map[string]Camera `json:"cameras"`
	Go2RTC  struct {
		Streams map[string]any `json:"streams"`
	} `json:"go2rtc"`
}

// Camera is one camera of the Frigate configuration.
type Camera struct {
	Name         string `json:"name"`
	FriendlyName string `json:"friendly_name"`
	Enabled      *bool  `json:"enabled"`
	FFmpeg       struct {
		Inputs []struct {
			Path  string   `json:"path"`
			Roles []string `json:"roles"`
		} `json:"inputs"`
	} `json:"ffmpeg"`
	Detect struct {
		Enabled *bool `json:"enabled"`
		Width   int   `json:"width"`
		Height  int   `json:"height"`
		FPS     int   `json:"fps"`
	} `json:"detect"`
	Record struct {
		Enabled *bool `json:"enabled"`
	} `json:"record"`
	Snapshots struct {
		Enabled *bool `json:"enabled"`
	} `json:"snapshots"`
	Objects struct {
		Track []string `json:"track"`
	} `json:"objects"`
	Onvif struct {
		Host string `json:"host"`
	} `json:"onvif"`
}

func parseConfig(body []byte) (Config, error) {
	var c Config
	if err := json.Unmarshal(body, &c); err != nil {
		return c, fmt.Errorf("unexpected configuration from Frigate: %w", err)
	}
	if c.Cameras == nil {
		return c, fmt.Errorf("the address does not answer like Frigate (no cameras in /api/config)")
	}
	return c, nil
}

// Stats is the part of GET /api/stats Phoedox uses.
type Stats struct {
	Cameras map[string]struct {
		CameraFPS         float64 `json:"camera_fps"`
		ProcessFPS        float64 `json:"process_fps"`
		SkippedFPS        float64 `json:"skipped_fps"`
		DetectionFPS      float64 `json:"detection_fps"`
		ConnectionQuality string  `json:"connection_quality"`
	} `json:"cameras"`
	Detectors map[string]struct {
		InferenceSpeed float64 `json:"inference_speed"`
	} `json:"detectors"`
	DetectionFPS float64                   `json:"detection_fps"`
	CPU          map[string]map[string]any `json:"cpu_usages"`
	Service      struct {
		Uptime        int    `json:"uptime"`
		Version       string `json:"version"`
		LatestVersion string `json:"latest_version"`
		Storage       map[string]struct {
			Total *float64 `json:"total"` // MiB
			Used  *float64 `json:"used"`
			Free  *float64 `json:"free"`
			Mount string   `json:"mount_type"`
		} `json:"storage"`
	} `json:"service"`
}

func parseStats(body []byte) (Stats, error) {
	var s Stats
	if err := json.Unmarshal(body, &s); err != nil {
		return s, fmt.Errorf("unexpected statistics from Frigate: %w", err)
	}
	return s, nil
}

// cameraState: "disabled" when switched off in Frigate, otherwise
// connected while frames arrive.
func cameraState(c Camera, s Stats, name string) string {
	if c.Enabled != nil && !*c.Enabled {
		return "disabled"
	}
	cs, ok := s.Cameras[name]
	switch {
	case !ok:
		return ""
	case cs.CameraFPS > 0:
		return "connected"
	}
	return "disconnected"
}

// cameraHost finds the camera's address: the first input that is not the
// local go2rtc restream, then the go2rtc stream of the same name, then the
// ONVIF host.
func cameraHost(c Config, name string) string {
	cam := c.Cameras[name]
	var restreams []string
	for _, in := range cam.FFmpeg.Inputs {
		h := hostOf(in.Path)
		if h != "" && !local(h) {
			return h
		}
		if h != "" {
			restreams = append(restreams, lastSegment(in.Path))
		}
	}
	for _, s := range append(restreams, name) {
		for _, src := range streamSources(c.Go2RTC.Streams[s]) {
			if h := hostOf(src); h != "" && !local(h) {
				return h
			}
		}
	}
	if h := strings.TrimSpace(cam.Onvif.Host); h != "" && !local(h) {
		return h
	}
	return ""
}

// streamSources reads a go2rtc stream (a string or a list).
func streamSources(v any) []string {
	switch x := v.(type) {
	case string:
		return []string{x}
	case []any:
		var out []string
		for _, i := range x {
			if s, ok := i.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// hostOf returns the host of rtsp://…, ffmpeg:rtsp://…#video=copy and the like.
func hostOf(src string) string {
	src = strings.TrimSpace(src)
	if i := strings.Index(src, "://"); i > 0 {
		if j := strings.LastIndexAny(src[:i], ":"); j >= 0 {
			src = src[j+1:] // drop "ffmpeg:" and similar prefixes
		}
	}
	if i := strings.IndexByte(src, '#'); i >= 0 {
		src = src[:i]
	}
	u, err := url.Parse(src)
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

func local(h string) bool {
	if h == "localhost" {
		return true
	}
	a, err := netip.ParseAddr(h)
	return err == nil && a.IsLoopback()
}

func lastSegment(p string) string {
	u, err := url.Parse(p)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	return parts[len(parts)-1]
}

// graphOps: Frigate as service, its cameras as camera "related to" it.
func graphOps(name, base string, c Config, s Stats) pdk.GraphOps {
	var ops pdk.GraphOps
	attrs := map[string]any{"url": base}
	if s.Service.Version != "" {
		attrs["version"] = s.Service.Version
	}
	if u, err := url.Parse(base); err == nil && u.Port() != "" {
		if p, err := strconv.Atoi(u.Port()); err == nil {
			attrs["port"] = p
		}
	}
	svc := ops.UpsertNamed("service", "frigate-"+integ.Slugify(name), name, attrs)
	names := make([]string, 0, len(c.Cameras))
	for n := range c.Cameras {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, n := range names {
		cam := c.Cameras[n]
		a := map[string]any{}
		if h := cameraHost(c, n); h != "" {
			if _, err := netip.ParseAddr(h); err == nil {
				a["ip"] = h
			} else {
				a["hostname"] = h
			}
		}
		if st := cameraState(cam, s, n); st != "" {
			a["state"] = st
		}
		if cam.Detect.Width > 0 && cam.Detect.Height > 0 {
			a["resolution"] = fmt.Sprintf("%dx%d", cam.Detect.Width, cam.Detect.Height)
		}
		if cam.Record.Enabled != nil {
			a["recording"] = map[bool]string{true: "on", false: "off"}[*cam.Record.Enabled]
		}
		if cam.Detect.Enabled != nil {
			a["object_detection"] = *cam.Detect.Enabled
		}
		if len(cam.Objects.Track) > 0 {
			a["tracked_objects"] = strings.Join(cam.Objects.Track, ", ")
		}
		label := cam.FriendlyName
		if label == "" {
			label = n
		}
		ref := ops.UpsertNamed("camera", "frigate-camera-"+integ.Slugify(n), label, a)
		ops.Link(ref, "related_to", svc, nil)
	}
	return ops
}

// EventArgs are the optional filters of frigate.events.
type EventArgs struct {
	Camera string `json:"camera"`
	Label  string `json:"label"`
}

// eventsQuery builds the query of /api/events (50 newest, no thumbnails).
func eventsQuery(a EventArgs) string {
	q := url.Values{"limit": {"50"}, "include_thumbnails": {"0"}}
	if c := strings.TrimSpace(a.Camera); c != "" {
		q.Set("cameras", c)
	}
	if l := strings.TrimSpace(a.Label); l != "" {
		q.Set("labels", l)
	}
	return "/api/events?" + q.Encode()
}

// Event is what frigate.events returns per event.
type Event struct {
	Label     string   `json:"label"`
	SubLabel  string   `json:"sub_label,omitempty"`
	Camera    string   `json:"camera"`
	Score     *int     `json:"score_pct,omitempty"`
	Start     string   `json:"start"`
	End       string   `json:"end,omitempty"`
	HasClip   bool     `json:"has_clip"`
	HasSnap   bool     `json:"has_snapshot"`
	Zones     []string `json:"zones,omitempty"`
	FalsePosi *bool    `json:"false_positive,omitempty"`
}

func parseEvents(body []byte) ([]Event, error) {
	var raw []struct {
		Camera        string   `json:"camera"`
		Label         string   `json:"label"`
		SubLabel      any      `json:"sub_label"`
		TopScore      *float64 `json:"top_score"`
		FalsePositive *bool    `json:"false_positive"`
		StartTime     float64  `json:"start_time"`
		EndTime       *float64 `json:"end_time"`
		HasClip       bool     `json:"has_clip"`
		HasSnapshot   bool     `json:"has_snapshot"`
		Zones         []string `json:"zones"`
		Data          struct {
			TopScore *float64 `json:"top_score"`
			Score    *float64 `json:"score"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("unexpected event list from Frigate: %w", err)
	}
	out := make([]Event, 0, len(raw))
	for _, r := range raw {
		e := Event{Label: r.Label, Camera: r.Camera, Start: unix(r.StartTime), HasClip: r.HasClip, HasSnap: r.HasSnapshot, Zones: r.Zones, FalsePosi: r.FalsePositive}
		if r.EndTime != nil {
			e.End = unix(*r.EndTime)
		}
		switch sl := r.SubLabel.(type) {
		case string:
			e.SubLabel = sl
		case []any: // [name, score]
			if len(sl) > 0 {
				e.SubLabel, _ = sl[0].(string)
			}
		}
		for _, sc := range []*float64{r.Data.TopScore, r.TopScore, r.Data.Score} {
			if sc != nil {
				pct := int(math.Round(*sc * 100))
				e.Score = &pct
				break
			}
		}
		out = append(out, e)
	}
	return out, nil
}

func unix(f float64) string {
	if f <= 0 {
		return ""
	}
	sec, frac := math.Modf(f)
	return time.Unix(int64(sec), int64(frac*1e9)).UTC().Format(time.RFC3339)
}

// StatsView is what frigate.stats returns.
type StatsView struct {
	Version       string                 `json:"version,omitempty"`
	LatestVersion string                 `json:"latest_version,omitempty"`
	UptimeS       int                    `json:"uptime_s"`
	DetectionFPS  float64                `json:"detection_fps"`
	CPUPct        string                 `json:"cpu_pct,omitempty"`
	MemPct        string                 `json:"memory_pct,omitempty"`
	Detectors     map[string]float64     `json:"detector_inference_ms,omitempty"`
	Cameras       map[string]CameraStats `json:"cameras"`
	Storage       map[string]StorageView `json:"storage,omitempty"`
}

// CameraStats are the rates of one camera.
type CameraStats struct {
	CameraFPS    float64 `json:"camera_fps"`
	ProcessFPS   float64 `json:"process_fps"`
	SkippedFPS   float64 `json:"skipped_fps"`
	DetectionFPS float64 `json:"detection_fps"`
	Quality      string  `json:"connection_quality,omitempty"`
}

// StorageView is the usage of one storage path in GiB.
type StorageView struct {
	TotalGiB float64 `json:"total_gib"`
	UsedGiB  float64 `json:"used_gib"`
	FreeGiB  float64 `json:"free_gib"`
	Mount    string  `json:"mount_type,omitempty"`
}

func statsView(s Stats) StatsView {
	v := StatsView{Version: s.Service.Version, LatestVersion: s.Service.LatestVersion, UptimeS: s.Service.Uptime, DetectionFPS: s.DetectionFPS,
		Detectors: map[string]float64{}, Cameras: map[string]CameraStats{}, Storage: map[string]StorageView{}}
	if sys, ok := s.CPU["frigate.full_system"]; ok {
		v.CPUPct, v.MemPct = fmt.Sprint(sys["cpu"]), fmt.Sprint(sys["mem"])
	}
	for n, d := range s.Detectors {
		v.Detectors[n] = d.InferenceSpeed
	}
	for n, c := range s.Cameras {
		v.Cameras[n] = CameraStats{CameraFPS: c.CameraFPS, ProcessFPS: c.ProcessFPS, SkippedFPS: c.SkippedFPS, DetectionFPS: c.DetectionFPS, Quality: c.ConnectionQuality}
	}
	mib := func(p *float64) float64 {
		if p == nil {
			return 0
		}
		return integ.GiB(*p * (1 << 20))
	}
	for p, st := range s.Service.Storage {
		if st.Total == nil {
			continue
		}
		v.Storage[p] = StorageView{TotalGiB: mib(st.Total), UsedGiB: mib(st.Used), FreeGiB: mib(st.Free), Mount: st.Mount}
	}
	return v
}

// cookieToken reads the JWT from the Set-Cookie header of POST /api/login
// ("frigate_token=<jwt>; Path=/; HttpOnly; …"; the cookie name is
// configurable in Frigate).
func cookieToken(setCookie string) string {
	first, _, _ := strings.Cut(setCookie, ";")
	_, v, ok := strings.Cut(first, "=")
	if !ok {
		return ""
	}
	return strings.Trim(strings.TrimSpace(v), `"`)
}

// loginBody is the JSON of POST /api/login.
func loginBody(user, pass string) string {
	b, _ := json.Marshal(map[string]string{"user": user, "password": pass})
	return string(b)
}
