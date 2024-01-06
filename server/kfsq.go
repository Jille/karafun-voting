package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Jille/contextcond"
	"github.com/Jille/genericz"
	"github.com/Jille/genericz/mapz"
	"github.com/Jille/genericz/slicez"
	"github.com/davecgh/go-spew/spew"
	"github.com/gorilla/websocket"
)

var (
	datadir = flag.String("datadir", "", "Path to session persistence")

	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	dialer = websocket.Dialer{}

	sessions mapz.SyncMap[string, *karaokeSession]
)

func main() {
	flag.Parse()
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
	singerRoundRobin   []string

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
			u.Status = &s.Status
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
			u.Permissions = &s.Permissions
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
	if *datadir != "" {
		b, err := ioutil.ReadFile(filepath.Join(*datadir, s.channel+".json"))
		if os.IsNotExist(err) {
			b = []byte("null")
			err = nil
		}
		if err != nil {
			log.Printf("Can't recover session %s: %v", s.channel, err)
		}
		if err := json.Unmarshal(b, &s.Queue); err != nil {
			panic(fmt.Errorf("failed to recover session %s: %v", s.channel, err))
		}
	}
	// TODO: Fetch the new webkcs url from their remote page
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
					switch t.Track.Type {
					case 1:
						t.Track.Caption = "General volume"
					case 4:
						t.Track.Caption = "Vocals"
					default:
						t.Track.Caption = "Audio"
					}
				}
				tracks = append(tracks, Track{
					TrackID: t.Track.Type,
					Caption: t.Track.Caption,
					Volume:  t.Volume,
					Color:   fmt.Sprintf("%02x%02x%02x", t.Track.Color.Red, t.Track.Color.Green, t.Track.Color.Blue),
				})
			}
			slices.SortFunc(tracks, func(a, b Track) int {
				if a.TrackID < b.TrackID {
					return -1
				}
				return 1
			})
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
		MyQueueID:  int(rand.Int63n(9007199254740991)),
	})
	s.reorder()
	s.persistToDisk()
	s.QueueVersion++
	s.mtx.Unlock()
	s.cond.Broadcast()
}

func (s *karaokeSession) upvote(myQueueID int, singer string) {
	s.mtx.Lock()
	for i, qe := range s.Queue {
		if qe.MyQueueID == myQueueID && !slices.Contains(qe.Singers, singer) {
			if j := slices.Index(qe.Singers, "adopted"); j != -1 {
				qe.Singers[j] = singer
			} else {
				s.Queue[i].Singers = append(qe.Singers, singer)
			}
		}
	}
	s.reorder()
	s.persistToDisk()
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
	s.reorder()
	s.persistToDisk()
	s.QueueVersion++
	s.mtx.Unlock()
	s.cond.Broadcast()
}

func (s *karaokeSession) moveUpDown(myQueueID int, up bool) {
	s.mtx.Lock()
	for i, qe := range s.Queue {
		if qe.MyQueueID != myQueueID {
			continue
		}
		if up {
			for j, qe2 := range s.Queue[:i] {
				if setEquals(qe.Singers, qe2.Singers) {
					s.Queue[i], s.Queue[j] = s.Queue[j], s.Queue[i]
					break
				}
			}
		} else {
			for j, qe2 := range s.Queue[i+1:] {
				if setEquals(qe.Singers, qe2.Singers) {
					s.Queue[i], s.Queue[i+1+j] = s.Queue[i+1+j], s.Queue[i]
					break
				}
			}
		}
	}
	s.reorder()
	s.persistToDisk()
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
				Artist:        qe.Song.Artist,
				Song:          qe.Song.Title,
				Singers:       []string{"adopted"},
				HasBeenQueued: true,
				MyQueueID:     int(rand.Int63n(9007199254740991)),
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
					s.cond.Broadcast()
				}
			}
			return "remote.RemoveFromQueueRequest", map[string]any{
				"queueItemId": kfqe.ID,
			}, true
		}
	}

	inKfQueue := map[string][]idAndIndex{}
	for i, kfqe := range kfQueue {
		inKfQueue[kfqe.ArtistSong()] = append(inKfQueue[kfqe.ArtistSong()], idAndIndex{i, kfqe.ID})
	}
	s.Queue = slicez.Filter(s.Queue, func(mqe QueueSong) bool {
		if _, ok := inKfQueue[mqe.ArtistSong()]; !ok {
			if mqe.HasBeenQueued {
				s.QueueVersion++
				return false
			}
		}
		return true
	})
	position := 0
	for i, mqe := range s.Queue {
		qpos, ok := inKfQueue[mqe.ArtistSong()]
		if !ok {
			if mqe.SongID == 0 {
				panic(fmt.Errorf("Trying to queue %q - %q, but I don't know the song_id", mqe.Artist, mqe.Song))
			}
			payload := map[string]any{
				"identifier": map[string]any{
					"type": 1,
					"id":   mqe.SongID,
				},
				"position": 99999,
			}
			if mqe.Artist == "" && mqe.Song == "" {
				payload["singer"] = fmt.Sprintf("sentinel-%d", mqe.MyQueueID)
			} else if mqe.MinSingers > len(mqe.Singers) {
				continue
			}
			return "remote.AddToQueueRequest", payload, true
		}
		if mqe.MinSingers > len(mqe.Singers) {
			continue
		}
		s.Queue[i].HasBeenQueued = true
		if qpos[0].Index != position {
			return "remote.MoveInQueueRequest", map[string]any{
				"queueItemId": qpos[0].ID,
				"from":        qpos[0].Index,
				"to":          position,
			}, true
		}
		if len(qpos) == 1 {
			delete(inKfQueue, mqe.ArtistSong())
		} else {
			inKfQueue[mqe.ArtistSong()] = qpos[1:]
		}
		position++
	}
	for _, qpos := range inKfQueue {
		return "remote.RemoveFromQueueRequest", map[string]any{
			"queueItemId": qpos[0].ID,
		}, true
	}

	s.cond.Broadcast()
	return "", nil, false
}

type idAndIndex struct {
	Index int
	ID    any
}

func (s *karaokeSession) reorder() {
	var singerRoundRobin []string
	for _, qe := range s.Queue {
		for _, name := range qe.Singers {
			if !slices.Contains(singerRoundRobin, name) {
				singerRoundRobin = append(singerRoundRobin, name)
			}
		}
	}

	happiness := make([]float64, len(singerRoundRobin))

	rrIdx := 0
	moved := make([]bool, len(s.Queue))
	newQueue := make([]QueueSong, 0, len(s.Queue))
	for len(newQueue) < len(s.Queue) {
		lowestHappiness := genericz.Min(happiness...)
		for happiness[rrIdx] > lowestHappiness {
			rrIdx = (rrIdx + 1) % len(singerRoundRobin)
		}
		nextUp := singerRoundRobin[rrIdx]
		foundSong := false
		for i, qe := range s.Queue {
			if !moved[i] && slices.Contains(qe.Singers, nextUp) {
				newQueue = append(newQueue, qe)
				moved[i] = true
				for _, name := range qe.Singers {
					happiness[slices.Index(singerRoundRobin, name)] += 1 / float64(len(qe.Singers))
				}
				foundSong = true
				break
			}
		}
		if !foundSong {
			happiness[rrIdx] += 1000
		}
		rrIdx = (rrIdx + 1) % len(singerRoundRobin)
	}
	s.Queue = newQueue
	s.determineMoveability()
	spew.Dump(s.Queue)
}

func (s *karaokeSession) determineMoveability() {
	for i, qe := range s.Queue {
		s.Queue[i].CanMoveUp = false
		s.Queue[i].CanMoveDown = false
		for _, qe2 := range s.Queue[:i] {
			if setEquals(qe.Singers, qe2.Singers) {
				s.Queue[i].CanMoveUp = true
				break
			}
		}
		for _, qe2 := range s.Queue[i+1:] {
			if setEquals(qe.Singers, qe2.Singers) {
				s.Queue[i].CanMoveDown = true
				break
			}
		}
	}
}

func setEquals(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for _, x := range a {
		found := false
		for _, y := range b {
			if x == y {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func pickNextSinger(happiness []float64) int {
	var winner int
	for i, s := range happiness {
		if s < happiness[winner] {
			winner = i
		}
	}
	return winner
}

func (s *karaokeSession) persistToDisk() {
	if *datadir == "" {
		return
	}
	b, err := json.Marshal(s.Queue)
	if err != nil {
		log.Printf("Failed to persist queue to disk: %v", err)
		return
	}
	if err := ioutil.WriteFile(filepath.Join(*datadir, s.channel+".json"), b, 0666); err != nil {
		log.Printf("Failed to persist queue to disk: %v", err)
		return
	}
}

type Command struct {
	Command    string `json:"command"`
	Channel    string `json:"channel,omitempty"`
	Username   string `json:"username,omitempty"`
	SongID     int    `json:"song_id,omitempty"`
	TrackID    int    `json:"track_id,omitempty"`
	MyQueueID  int    `json:"my_queue_id,omitempty"`
	Number     int    `json:"number,omitempty"`
	MinSingers int    `json:"min_singers,omitempty"`
}

type Update struct {
	Status      *Status      `json:"status"`
	Queue       []QueueSong  `json:"queue"`
	Permissions *Permissions `json:"permissions"`
}

type QueueSong struct {
	Artist        string   `json:"artist"`
	Song          string   `json:"song"`
	Singers       []string `json:"singers"`
	MinSingers    int      `json:"min_singers"`
	SongID        int      `json:"song_id"`
	MyQueueID     int      `json:"my_queue_id"`
	CanMoveUp     bool     `json:"can_move_up"`
	CanMoveDown   bool     `json:"can_move_down"`
	HasBeenQueued bool     `json:"has_been_queued"`
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
	ID     any    `json:"id"`
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

func (qe *KarafunQueueEntry) ArtistSong() string {
	if qe == nil {
		return "\x00\x00\x00"
	}
	return qe.Song.Artist + "\x00" + qe.Song.Title
}

func (qe *QueueSong) ArtistSong() string {
	if qe == nil {
		return "\x00\x00\x00"
	}
	return qe.Artist + "\x00" + qe.Song
}
