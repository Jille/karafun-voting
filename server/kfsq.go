package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Jille/contextcond"
	"github.com/Jille/genericz"
	"github.com/Jille/genericz/mapz"
	"github.com/gorilla/websocket"
)

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	dialer = websocket.Dialer{}

	sessions mapz.SyncMap[string, *karaokeSession]
)

func main() {
	http.HandleFunc("/ws", handleWS)
	log.Fatal(http.ListenAndServe(":8066", nil))
}

type karaokeSession struct {
	channel string

	mtx                sync.Mutex
	cond               *contextcond.Cond
	Status             Status
	Queue              []QueueSong
	Permissions        Permissions
	StatusVersion      int
	QueueVersion       int
	PermissionsVersion int
	SessionError       string

	queueSyncCh chan []KarafunQueueEntry

	writeMtx          sync.Mutex
	kfConn            *websocket.Conn
	kfCommandSequence int
}

func allocSession(channel string) *karaokeSession {
	ret := &karaokeSession{channel: channel}
	ret.cond = contextcond.NewCond(&ret.mtx)
	ret.queueSyncCh = make(chan []KarafunQueueEntry, 1)
	ret.mtx.Lock()
	return ret
}

func handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	messageType, channel, err := conn.ReadMessage()
	if err != nil {
		log.Println(err)
		return
	}
	if messageType != websocket.TextMessage || len(channel) != 6 {
		log.Printf("Unexpected %s in hello", messageType)
		return
	}
	s, exists := sessions.LoadOrStore(string(channel), allocSession(string(channel)))
	if !exists {
		s.init()
	}
	s.mtx.Lock()
	if s.SessionError != "" {
		e, _ := json.Marshal(struct{ Error string }{s.SessionError})
		s.mtx.Unlock()
		conn.WriteMessage(websocket.TextMessage, e)
		return
	}
	s.mtx.Unlock()
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go s.websockWriter(ctx, conn)

	for {
		messageType, p, err := conn.ReadMessage()
		if err != nil {
			log.Println(err)
			return
		}
		switch messageType {
		case websocket.BinaryMessage:
			log.Printf("received binary %v", p)
		case websocket.CloseMessage:
			log.Printf("received close %v", p)
		case websocket.PingMessage:
			log.Printf("received ping %v", p)
		case websocket.PongMessage:
			log.Printf("received pong %v", p)

		case websocket.TextMessage:
			var cmd Command
			if err := json.Unmarshal(p, &cmd); err != nil {
				log.Printf("Bad json: %v", err)
				return
			}
			log.Printf("cmd: %s", cmd.Command)
			switch cmd.Command {
			case "enqueue":
				s.enqueue(cmd.SongID, cmd.Username, cmd.MinSingers)
			case "upvote":
				s.upvote(cmd.MyQueueID, cmd.Username)
			case "remove":
				s.remove(cmd.MyQueueID)
			case "play":
				s.play()
			case "pause":
				s.pause()
			case "next":
				s.next()
			case "set-volume":
				s.setVolume(cmd.TrackID, cmd.Number)
			case "change-key":
				s.changeKey(cmd.Number)
			case "change-tempo":
				s.changeTempo(cmd.Number)
			case "move-up", "move-down":
				s.moveUpDown(cmd.MyQueueID, cmd.Command == "move-up")
			default:
				log.Printf("Ignoring unknown command %q", cmd.Command)
			}
		default:
			log.Printf("Ignoring unknown message type %v", messageType)
		}
	}
}

func (s *karaokeSession) websockWriter(ctx context.Context, conn *websocket.Conn) {
	conn.EnableWriteCompression(true)
	statusVersion := 0
	queueVersion := 0
	permissionsVersion := 0
	s.mtx.Lock()
	defer s.mtx.Unlock()
	for {
		for statusVersion == s.StatusVersion && queueVersion == s.QueueVersion && permissionsVersion == s.PermissionsVersion {
			if err := s.cond.WaitContext(ctx); err != nil {
				return
			}
		}
		u := Update{}
		if statusVersion != s.StatusVersion {
			c := s.Status
			u.Status = &c
			statusVersion = s.StatusVersion
		}
		if queueVersion != s.QueueVersion {
			if s.Queue != nil {
				u.Queue = s.Queue
			} else {
				u.Queue = []QueueSong{}
			}
			queueVersion = s.QueueVersion
		}
		if permissionsVersion != s.PermissionsVersion {
			p := s.Permissions
			u.Permissions = &p
			permissionsVersion = s.PermissionsVersion
		}
		j, err := json.Marshal(u)
		if err != nil {
			panic(err)
		}
		s.mtx.Unlock()
		conn.SetWriteDeadline(time.Now().Add(time.Minute))
		err = conn.WriteMessage(websocket.TextMessage, j)
		s.mtx.Lock()
		if err != nil {
			log.Printf("Write error: %v", err)
			return
		}
	}
}

func (s *karaokeSession) init() {
	defer s.mtx.Unlock()
	// 1. Fetch the new webkcs url from their remote page
	h := http.Header{}
	h.Set("X-Karafun-Channel", s.channel)
	conn, _, err := dialer.Dial("ws://localhost:8067/", h)
	if err != nil {
		s.SessionError = err.Error()
		return
	}
	s.kfConn = conn
	go s.listen()
	go s.queueSyncer()
}

func (s *karaokeSession) listen() {
	for {
		messageType, p, err := s.kfConn.ReadMessage()
		if err != nil {
			log.Println(err)
			return
		}
		if messageType != websocket.TextMessage {
			log.Printf("Ignoring %v message from karafun: %q", messageType, string(p))
			continue
		}
		log.Printf("[karafun] %s", p)
		var km KarafunMessage
		if err := json.Unmarshal(p, &km); err != nil {
			panic(err)
		}
		switch km.Type {
		case "remote.QueueEvent":
			select {
			case s.queueSyncCh <- km.Payload.Queue.Items:
			case <-s.queueSyncCh:
				// We dropped the previous entry in the queue, now write the new one.
				s.queueSyncCh <- km.Payload.Queue.Items
			}

		case "remote.PermissionsUpdateEvent":
			s.mtx.Lock()
			s.Permissions = km.Payload.Permissions
			s.PermissionsVersion++
			s.mtx.Unlock()
			s.cond.Broadcast()

		case "remote.StatusEvent":
			var tracks []Track
			for _, t := range km.Payload.Status.Tracks {
				if t.Track.Caption == "" {
					t.Track.Caption = "Audio"
				}
				tracks = append(tracks, Track{
					TrackID: t.Track.Type,
					Caption: t.Track.Caption,
					Volume:  t.Volume,
					Color:   fmt.Sprintf("%02x%02x%02x", t.Track.Color.Red, t.Track.Color.Green, t.Track.Color.Blue),
				})
			}
			s.mtx.Lock()
			s.Status = Status{
				Playing: km.Payload.Status.State == 4,
				Loading: km.Payload.Status.State < 4,
				Tempo:   km.Payload.Status.Tempo,
				Pitch:   km.Payload.Status.Pitch,
				Tracks:  tracks,
			}
			s.StatusVersion++
			s.mtx.Unlock()
			s.cond.Broadcast()
		}
	}
}

func (s *karaokeSession) sendCommand(name string, payload map[string]any) {
	if payload == nil {
		payload = map[string]any{}
	}
	s.writeMtx.Lock()
	defer s.writeMtx.Unlock()
	s.kfCommandSequence++
	id := s.kfCommandSequence
	msg := map[string]any{
		"id":      id,
		"type":    name,
		"payload": payload,
	}
	if err := s.kfConn.WriteJSON(msg); err != nil {
		log.Printf("Writing to Karafun failed: %v", err)
	}
}

func (s *karaokeSession) enqueue(songID int, singer string, minSingers int) {
	s.mtx.Lock()
	s.Queue = append(s.Queue, QueueSong{
		Singers:    []string{singer},
		MinSingers: minSingers,
		SongID:     songID,
		MyQueueID:  int(rand.Int63()),
	})
	s.QueueVersion++
	s.mtx.Unlock()
	s.cond.Broadcast()
}

func (s *karaokeSession) upvote(myQueueID int, singer string) {
	s.mtx.Lock()
	for i, qe := range s.Queue {
		if qe.MyQueueID == myQueueID && !slices.Contains(qe.Singers, singer) {
			s.Queue[i].Singers = append(qe.Singers, singer)
		}
	}
	s.QueueVersion++
	s.mtx.Unlock()
	s.cond.Broadcast()
}

func (s *karaokeSession) remove(myQueueID int) {
	s.mtx.Lock()
	for i, qe := range s.Queue {
		if qe.MyQueueID == myQueueID {
			copy(s.Queue[i:], s.Queue[i+1:])
			s.Queue = s.Queue[:len(s.Queue)-1]
			break
		}
	}
	s.QueueVersion++
	s.mtx.Unlock()
	s.cond.Broadcast()
}

func (s *karaokeSession) play() {
	s.sendCommand("remote.PlayRequest", nil)
}

func (s *karaokeSession) pause() {
	s.sendCommand("remote.PauseRequest", nil)
}

func (s *karaokeSession) next() {
	s.sendCommand("remote.NextRequest", nil)
}

func (s *karaokeSession) setVolume(trackID, vol int) {
	s.sendCommand("remote.TrackVolumeRequest", map[string]any{
		"type":   trackID,
		"volume": vol,
	})
}

func (s *karaokeSession) changeKey(val int) {
	s.sendCommand("remote.PitchRequest", map[string]any{
		"pitch": val,
	})
}

func (s *karaokeSession) changeTempo(val int) {
	s.sendCommand("remote.TempoRequest", map[string]any{
		"pitch": val,
	})
}

func (s *karaokeSession) moveUpDown(myQueueID int, up bool) {
	/*
		s.sendCommand("remote.MoveInQueueResponse", map[string]any{
			"queueItemId": XXX,
			"to": XXX,
		})
	*/
}

func (s *karaokeSession) queueSyncer() {
	queueChangedCh := make(chan struct{}, 1)
	go func() {
		queueVersion := 0
		s.mtx.Lock()
		for {
			for queueVersion == s.QueueVersion {
				s.cond.Wait()
			}
			queueVersion = s.QueueVersion
			select {
			case queueChangedCh <- struct{}{}:
			default:
			}
		}
		s.mtx.Unlock()
	}()
	kfQueue := <-s.queueSyncCh
	s.mtx.Lock()
	if s.Queue == nil {
		for _, qe := range kfQueue {
			s.Queue = append(s.Queue, QueueSong{
				Artist:    qe.Song.Artist,
				Song:      qe.Song.Title,
				Singers:   []string{"adopted"},
				KfQueueID: qe.ID,
				MyQueueID: int(rand.Int63()),
			})
		}
	}
	s.QueueVersion++
	s.mtx.Unlock()
	s.cond.Broadcast()
	for {
		if method, payload, ok := s.reconcile(kfQueue); ok {
			s.sendCommand(method, payload)
			time.Sleep(300 * time.Millisecond)
			time.Sleep(700 * time.Millisecond) // XXX debugging
			kfQueue = <-s.queueSyncCh
			continue
		}
		select {
		case kfQueue = <-s.queueSyncCh:
		case <-queueChangedCh:
		}
	}
}

func (s *karaokeSession) reconcile(kfQueue []KarafunQueueEntry) (string, map[string]any, bool) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	for _, kfqe := range kfQueue {
		if strings.HasPrefix(kfqe.Singer, "sentinel-") {
			log.Printf("Found %s: %#v", kfqe.Singer, kfqe)
			for i, mqe := range s.Queue {
				if kfqe.Singer == fmt.Sprintf("sentinel-%d", mqe.MyQueueID) {
					s.Queue[i].Artist = kfqe.Song.Artist
					s.Queue[i].Song = kfqe.Song.Title
					log.Printf("Matched to song %#v", s.Queue[i])
					s.QueueVersion++
				}
			}
			return "remote.RemoveFromQueueRequest", map[string]any{
				"queueItemId": fmt.Sprint(kfqe.ID),
			}, true
		}
	}
	queued := map[string]struct{}{}
	for _, kfqe := range kfQueue {
		k := kfqe.Song.Artist + "\x00" + kfqe.Song.Title
		queued[k] = struct{}{}
	}
	wantedOrder := map[string][]int{}
	for i, mqe := range s.Queue {
		k := mqe.Artist + "\x00" + mqe.Song
		if _, ok := queued[k]; !ok {
			payload := map[string]any{
				"identifier": map[string]any{
					"type": 1,
					"id":   mqe.SongID,
				},
				"position": 99999,
			}
			if mqe.Artist == "" && mqe.Song == "" {
				payload["singer"] = fmt.Sprintf("sentinel-%d", mqe.MyQueueID)
			}
			return "remote.AddToQueueResponse", payload, true
		}
		wantedOrder[k] = append(wantedOrder[k], i)
	}
	for i := 0; genericz.Min(len(kfQueue), len(s.Queue)) > i; i++ {
		log.Printf("queuecmp: %d: %s <> %s", i, s.Queue[i].Song, kfQueue[i].Song.Title)
	}
	if len(kfQueue) > len(s.Queue) {
		for _, kfqe := range kfQueue[len(s.Queue):] {
			log.Printf("queuecmp: n/a <> %s", kfqe.Song.Title)
		}
	}
	if len(kfQueue) < len(s.Queue) {
		for _, mqe := range s.Queue[len(kfQueue):] {
			log.Printf("queuecmp: %s <> n/a", mqe.Song)
		}
	}
	for i, kfqe := range kfQueue {
		k := kfqe.Song.Artist + "\x00" + kfqe.Song.Title
		kfIds, ok := wantedOrder[k]
		if !ok {
			// Song in the Karafun queue that we don't know.
			return "remote.RemoveFromQueueRequest", map[string]any{
				"queueItemId": fmt.Sprint(kfqe.ID),
			}, true
		}
		if i != kfIds[0] {
			return "remote.MoveInQueueRequest", map[string]any{
				"queueItemId": fmt.Sprint(kfqe.ID),
				"to":          kfIds[0],
			}, true
		}
		if len(kfIds) == 1 {
			delete(wantedOrder, k)
		} else {
			wantedOrder[k] = kfIds[1:]
		}
	}
	s.cond.Broadcast()
	return "", nil, false
}

type Command struct {
	Command    string `json:"command"`
	Channel    string `json:"channel,omitempty"`
	Username   string `json:"username,omitempty"`
	SongID     int    `json:"song_id,omitempty"`
	TrackID    int    `json:"track_id,omitempty"`
	MyQueueID  int    `json:"my_queue_id,omitempty"`
	Number     int    `json:"number,omitempty"`
	Artist     string
	Song       string
	MinSingers int
}

type Update struct {
	Status      *Status      `json:"status"`
	Queue       []QueueSong  `json:"queue"`
	Permissions *Permissions `json:"permissions"`
}

type QueueSong struct {
	Artist     string   `json:"artist"`
	Song       string   `json:"song"`
	Singers    []string `json:"singers"`
	MinSingers int      `json:"min_singers"`
	SongID     int      `json:"song_id"`
	KfQueueID  string   `json:"kf_queue_id"`
	MyQueueID  int      `json:"my_queue_id"`
}

type Permissions struct {
	ManageQueue    bool `json:"manageQueue"`
	ViewQueue      bool `json:"viewQueue"`
	AddToQueue     bool `json:"addToQueue"`
	ManagePlayback bool `json:"managePlayback"`
	ManageVolumes  bool `json:"manageVolumes"`
}

type Status struct {
	Playing bool    `json:"playing"`
	Loading bool    `json:"loading"`
	Tempo   int     `json:"tempo"`
	Pitch   int     `json:"pitch"`
	Tracks  []Track `json:"tracks"`
}

type Track struct {
	TrackID int     `json:"track_id"`
	Volume  float32 `json:"volume"`
	Caption string  `json:"caption"`
	Color   string  `json:"color"`
}

type KarafunMessage struct {
	Type    string `json:"type"`
	Payload struct {
		Status struct {
			State  int `json:"state"`
			Tempo  int `json:"tempo"`
			Pitch  int `json:"pitch"`
			Tracks []struct {
				Track struct {
					Type    int    `json:"type"`
					Caption string `json:"caption"`
					Color   struct {
						Red   int `json:"red"`
						Green int `json:"green"`
						Blue  int `json:"blue"`
					} `json:"color"`
				} `json:"track,omitempty"`
				Volume float32 `json:"volume"`
			} `json:"tracks"`
		} `json:"status"`
		Queue struct {
			Items []KarafunQueueEntry `json:"items"`
		} `json:"queue"`
		Permissions Permissions `json:"permissions"`
		Preferences struct {
			AskOptions bool `json:"askOptions"`
		} `json:"preferences"`
		Configuration struct {
			PitchStep int `json:"pitchStep"`
			TempoStep int `json:"tempoStep"`
		} `json:"configuration"`
		Username string `json:"username"`
	} `json:"payload,omitempty"`
}

type KarafunQueueEntry struct {
	ID     string `json:"id"`
	Singer string `json:"singer"`
	Song   struct {
		ID struct {
			Type int `json:"type"`
			ID   int `json:"id"`
		} `json:"id"`
		Title      string        `json:"title"`
		Artist     string        `json:"artist"`
		SongTracks []interface{} `json:"songTracks"`
	} `json:"song"`
}
