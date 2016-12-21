package prolinksink

import (
	"encoding/base64"
	"fmt"

	"go.evanpurkhiser.com/prolink"
	"go.evanpurkhiser.com/prolink/trackstatus"
)

type track struct {
	Title   string `json:"title"`
	Artist  string `json:"artist"`
	Album   string `json:"album"`
	Label   string `json:"label"`
	Release string `json:"release"`
	Artwork string `json:"artwork"`
	Length  int    `json:"length"`
}

type trackStatus struct {
	Event    string `json:"event"`
	Metadata *track `json:"metadata"`
}

type playerStatus struct {
	PlayState      string  `json:"play_state"`
	BPM            float32 `json:"bpm"`
	Pitch          float32 `json:"pitch"`
	EffectivePitch float32 `json:"effective_pitch"`
	OnAir          bool    `json:"on_air"`
	Synced         bool    `json:"synced"`
}

type message struct {
	EventType string      `json:"event_type"`
	PlayerID  int         `json:"player_id"`
	Object    interface{} `json:"object"`
}

func mapTrack(t2 *prolink.Track) *track {
	t := track{
		Title:   t2.Title,
		Artist:  t2.Artist,
		Album:   t2.Album,
		Label:   t2.Label,
		Release: t2.Comment,
		Length:  int(t2.Length.Seconds()),
	}

	if len(t2.Artwork) > 0 {
		artworkBase64 := base64.StdEncoding.EncodeToString(t2.Artwork)
		t.Artwork = fmt.Sprintf("data:%s;base64,%s", "image/jpeg", artworkBase64)
	}

	return &t
}

func mapStatus(s2 *prolink.CDJStatus) *playerStatus {
	s := playerStatus{
		PlayState:      s2.PlayState.String(),
		OnAir:          s2.IsOnAir,
		Synced:         s2.IsSync,
		BPM:            s2.TrackBPM,
		Pitch:          s2.SliderPitch,
		EffectivePitch: s2.EffectivePitch,
	}

	return &s
}

// StatusMapper constructs status messages upon player and track status
// updates of CDJs on the pro link network.
type StatusMapper struct {
	Network           *prolink.Network
	TrackStatusConfig trackstatus.Config
	MessageHandler    func(interface{})

	prevStatus map[prolink.DeviceID]*prolink.CDJStatus
	started    bool
}

// Start begins listening for status on the network and will delgate messages
// to the MessageHandler provided.
func (m *StatusMapper) Start() {
	if m.started {
		return
	}

	sm := m.Network.CDJStatusMonitor()

	sm.OnStatusUpdate(prolink.StatusHandlerFunc(m.playerStatus))
	sm.OnStatusUpdate(trackstatus.NewHandler(m.TrackStatusConfig, m.trackStatus))
}

func (m *StatusMapper) trackStatus(event trackstatus.Event, status *prolink.CDJStatus) {
	rdb := m.Network.RemoteDB()

	object := trackStatus{Event: string(event)}

	nowPlaying := event == trackstatus.NowPlaying
	trackQuery := status.TrackQuery()

	// Only include metadata for now playing tracks
	if track, err := rdb.GetTrack(trackQuery); nowPlaying && err == nil {
		object.Metadata = mapTrack(track)
	}

	m.MessageHandler(message{
		EventType: "track_status",
		PlayerID:  int(status.PlayerID),
		Object:    object,
	})
}

func (m *StatusMapper) playerStatus(status *prolink.CDJStatus) {
	if m.prevStatus == nil {
		m.prevStatus = map[prolink.DeviceID]*prolink.CDJStatus{}
	}

	opStatus := m.prevStatus[status.PlayerID]

	if opStatus == nil {
		opStatus = &prolink.CDJStatus{}
	}

	sameStatus := status.TrackBPM == opStatus.TrackBPM &&
		status.EffectivePitch == opStatus.EffectivePitch &&
		status.SliderPitch == opStatus.SliderPitch &&
		status.PlayState == opStatus.PlayState &&
		status.IsOnAir == opStatus.IsOnAir &&
		status.IsSync == opStatus.IsSync

	if sameStatus {
		return
	}

	m.prevStatus[status.PlayerID] = status

	m.MessageHandler(message{
		EventType: "player_status",
		PlayerID:  int(status.PlayerID),
		Object:    mapStatus(status),
	})
}
