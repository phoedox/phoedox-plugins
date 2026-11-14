package main

import (
	"os"
	"strings"
	"testing"

	"github.com/phoedox/phoedox-sdk/go/pdk"
)

const config = `{"mqtt":{"enabled":true,"host":"mqtt.lan","password":"********"},
 "go2rtc":{"streams":{"garage":["rtsp://viewer:s3cret@192.168.30.12:554/h264Preview_01_main","ffmpeg:garage#audio=opus"]}},
 "cameras":{
  "front_door":{"name":"front_door","enabled":true,
   "ffmpeg":{"inputs":[{"path":"rtsp://*:*@192.168.30.11:554/cam/realmonitor?channel=1&subtype=0","roles":["detect","record"]}]},
   "detect":{"enabled":true,"width":1280,"height":720,"fps":5},"record":{"enabled":true,"retain":{"days":7,"mode":"motion"}},
   "snapshots":{"enabled":true},"objects":{"track":["person","car"]},"onvif":{"host":"","port":8000,"user":null,"password":null}},
  "garage":{"name":"garage","friendly_name":"Garage","enabled":true,
   "ffmpeg":{"inputs":[{"path":"rtsp://127.0.0.1:8554/garage","roles":["detect","record"]}]},
   "detect":{"enabled":false,"width":640,"height":360,"fps":5},"record":{"enabled":false},"objects":{"track":["person"]}},
  "backyard":{"name":"backyard","enabled":false,
   "ffmpeg":{"inputs":[{"path":"rtsp://*:*@cam-backyard.lan:554/stream1","roles":["detect"]}]},
   "detect":{"width":0,"height":0},"onvif":{"host":"192.168.30.13"}}}}`

const stats = `{"cameras":{
  "front_door":{"camera_fps":5.1,"process_fps":5.0,"skipped_fps":0.0,"detection_fps":0.4,"detection_enabled":true,"pid":101,"capture_pid":102,"ffmpeg_pid":103,"connection_quality":"excellent"},
  "garage":{"camera_fps":0.0,"process_fps":0.0,"skipped_fps":0.0,"detection_fps":0.0,"detection_enabled":false,"pid":104}},
 "detectors":{"coral":{"inference_speed":8.42,"detection_start":0.0,"pid":99}},
 "camera_fps":5.1,"process_fps":5.0,"skipped_fps":0.0,"detection_fps":0.4,
 "cpu_usages":{"frigate.full_system":{"cpu":"12.5","mem":"41.3"},"101":{"cpu":"3.0","mem":"1.1","cmdline":"frigate.process:front_door"}},
 "service":{"uptime":86400,"version":"0.16.2-4d58206","latest_version":"0.16.2",
  "storage":{"/media/frigate/recordings":{"total":953869.7,"used":512000.0,"free":441869.7,"mount_type":"ext4"},"/tmp/cache":{},"/dev/shm":{"total":512.0,"used":12.3,"free":499.7,"mount_type":"tmpfs","min_shm":79.5}},
  "last_updated":1731577200,"temperatures":{}}}`

func TestImport(t *testing.T) {
	c, err := parseConfig([]byte(config))
	if err != nil {
		t.Fatal(err)
	}
	s, err := parseStats([]byte(stats))
	if err != nil {
		t.Fatal(err)
	}
	ops := graphOps("Frigate NVR", "https://frigate.example.org:8971", c, s).Ops
	svc := ops[0]
	if svc.Kind != "service" || svc.ID != "frigate-frigate-nvr" || svc.Attrs["version"] != "0.16.2-4d58206" || svc.Attrs["port"] != 8971 || svc.Attrs["url"] != "https://frigate.example.org:8971" {
		t.Fatalf("service: %+v", svc)
	}
	// cameras sorted: backyard, front_door, garage; each followed by its link
	back, front, garage := ops[1], ops[3], ops[5]
	if back.Kind != "camera" || back.ID != "frigate-camera-backyard" || back.Attrs["state"] != "disabled" || back.Attrs["hostname"] != "cam-backyard.lan" {
		t.Fatalf("backyard: %+v", back)
	}
	if _, ok := back.Attrs["resolution"]; ok {
		t.Fatal("empty resolution set")
	}
	if front.Attrs["ip"] != "192.168.30.11" || front.Attrs["state"] != "connected" || front.Attrs["resolution"] != "1280x720" ||
		front.Attrs["recording"] != "on" || front.Attrs["tracked_objects"] != "person, car" || front.Attrs["object_detection"] != true {
		t.Fatalf("front: %+v", front)
	}
	// go2rtc restream: the address comes from the go2rtc stream, never the credentials.
	if garage.Name != "Garage" || garage.Attrs["ip"] != "192.168.30.12" || garage.Attrs["state"] != "disconnected" || garage.Attrs["recording"] != "off" {
		t.Fatalf("garage: %+v", garage)
	}
	for _, o := range ops {
		for _, v := range o.Attrs {
			if strings.Contains(strings.ToLower(toString(v)), "s3cret") {
				t.Fatalf("credentials leaked: %+v", o)
			}
		}
	}
	if ops[2].Op != "link" || ops[2].Rel != "related_to" || ops[2].From != back.Ref || ops[2].To != svc.Ref || len(ops) != 7 {
		t.Fatalf("links: %+v", ops)
	}
	v := statsView(s)
	if v.CPUPct != "12.5" || v.Detectors["coral"] != 8.42 || v.Cameras["front_door"].Quality != "excellent" ||
		v.Storage["/media/frigate/recordings"].TotalGiB != 931.5 || len(v.Storage) != 2 {
		t.Fatalf("stats: %+v", v)
	}
}

func toString(v any) string {
	s, _ := v.(string)
	return s
}

func TestEvents(t *testing.T) {
	ev, err := parseEvents([]byte(`[
	 {"id":"1731577200.123456-ab12cd","camera":"front_door","label":"person","sub_label":["Anna",0.91],"top_score":null,"false_positive":null,
	  "start_time":1731577200.5,"end_time":1731577215.0,"has_clip":true,"has_snapshot":true,"zones":["porch"],"retain_indefinitely":false,
	  "data":{"box":[0.1,0.2,0.3,0.4],"region":[0,0,1,1],"score":0.8,"top_score":0.874,"attributes":[],"type":"object"}},
	 {"id":"1731577100.0-xy","camera":"garage","label":"car","sub_label":null,"top_score":0.71,"start_time":1731577100,"end_time":null,
	  "has_clip":false,"has_snapshot":true,"zones":[],"data":{}}]`))
	if err != nil || len(ev) != 2 {
		t.Fatalf("%+v %v", ev, err)
	}
	if ev[0].SubLabel != "Anna" || *ev[0].Score != 87 || ev[0].Start != "2024-11-14T09:40:00Z" || ev[0].End != "2024-11-14T09:40:15Z" || !ev[0].HasClip {
		t.Fatalf("event 0: %+v", ev[0])
	}
	if *ev[1].Score != 71 || ev[1].End != "" || ev[1].HasClip {
		t.Fatalf("event 1: %+v", ev[1])
	}
	if q := eventsQuery(EventArgs{Camera: "front_door", Label: "person"}); q != "/api/events?cameras=front_door&include_thumbnails=0&labels=person&limit=50" {
		t.Fatal(q)
	}
	if q := eventsQuery(EventArgs{}); q != "/api/events?include_thumbnails=0&limit=50" {
		t.Fatal(q)
	}
}

func TestAuthAndErrors(t *testing.T) {
	if tok := cookieToken(`frigate_token=eyJhbGciOiJIUzI1NiJ9.e30.abc; HttpOnly; Max-Age=86400; Path=/; SameSite=lax; Secure`); tok != "eyJhbGciOiJIUzI1NiJ9.e30.abc" {
		t.Fatal(tok)
	}
	if cookieToken("") != "" {
		t.Fatal("empty cookie")
	}
	if b := loginBody("viewer", `p"w`); b != `{"password":"p\"w","user":"viewer"}` {
		t.Fatal(b)
	}
	if _, err := parseConfig([]byte(`{"detail":"Not Found"}`)); err == nil {
		t.Fatal("no cameras accepted")
	}
	if _, err := parseEvents([]byte(`{"message":"error"}`)); err == nil {
		t.Fatal("object accepted as event list")
	}
	if hostOf("ffmpeg:rtsp://u:p@10.0.0.5:554/x#video=copy") != "10.0.0.5" || hostOf("/dev/video0") != "" || !local("127.0.0.1") {
		t.Fatal("hostOf")
	}
}

func TestCameraSchema(t *testing.T) {
	a, _ := os.ReadFile("../schemas/camera.json")
	b, _ := os.ReadFile("../../unifiprotect/schemas/camera.json")
	if len(a) == 0 || string(a) != string(b) {
		t.Fatal("schemas/camera.json differs between frigate and unifiprotect")
	}
	data, _ := os.ReadFile("../phoedox-plugin.json")
	m, err := pdk.ParseManifest(data)
	if err != nil {
		t.Fatal(err)
	}
	other, _ := os.ReadFile("../../unifiprotect/phoedox-plugin.json")
	o, err := pdk.ParseManifest(other)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Contributes.Kinds) != 1 || m.Contributes.Kinds[0] != o.Contributes.Kinds[0] || len(m.Contributes.Relations) != len(o.Contributes.Relations) {
		t.Fatal("camera kind or relations differ from unifiprotect")
	}
	for i, r := range m.Contributes.Relations {
		q := o.Contributes.Relations[i]
		if r.ID != q.ID || strings.Join(r.From, ",") != strings.Join(q.From, ",") || strings.Join(r.To, ",") != strings.Join(q.To, ",") {
			t.Fatalf("relation %s differs", r.ID)
		}
	}
}
