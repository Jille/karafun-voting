import { WebSocketServer } from 'ws';
import io from "socket.io-client";

const wss = new WebSocketServer({ port: 8067 });

wss.on('connection', function connection(ws, request) {
	const channel = request.headers['x-karafun-channel'];
	let authed = false;
	let audioTracks = {};

	const kfsock = io("wss://www.karafun.co.uk/", {
		reconnectionDelayMax: 10000,
		transports: ["websocket"],
		query: {
			"remote": "kf" + channel,
		}
	});

	kfsock.on('connect', function() {
		console.log('kfsock connect event');
	});

	kfsock.on('permissions', function(perms) {
		if(!authed) {
			ws.send(JSON.stringify({
				type: 'core.AuthenticatedEvent',
				payload: {},
			}));
			authed = true;
		}
		ws.send(JSON.stringify({
			type: 'remote.PermissionsUpdateEvent',
			payload: {
				permissions: {
					"manageQueue": perms.includes('manageQueue'),
					"viewQueue": perms.includes('viewQueue'),
					"addToQueue": perms.includes('addToQueue'),
					"managePlayback": perms.includes('manageKaraoke'),
					"manageVolumes": perms.includes('managePlayer'),
					"sendPhotos": perms.includes('uploadPicture'),
				},
			},
		}));
	});
	kfsock.on('preferences', function(prefs) {
		ws.send(JSON.stringify({
			type: 'remote.PreferencesUpdateEvent',
			payload: {
				preferences: {
					askOptions: prefs.askSingerName,
					// TODO: prefs.generalVolume (bool)
					// TODO: prefs.micVolume (bool)
				},
			},
		}));
	});
	kfsock.on('status', function(status) {
		const tracks = [];
		if('volumeBv' in status) {
			tracks.push({
				track: {type: 4},
				volume: status['volumeBv'],
			});
		}
		if('volumeLd' in status) {
			for(const name of Object.keys(audioTracks)) {
				if(!(name in status['volumeLd'])) {
					delete audioTracks[name];
				}
			}
			for(const [name, volume] of Object.entries(status['volumeLd'])) {
				let id = 5;
				if(name in audioTracks) {
					id = audioTracks[name];
				} else {
					for(const n of Object.values(audioTracks)) {
						if(id <= n) {
							id = n+1;
						}
					}
					audioTracks[name] = id;
				}
				tracks.push({
					track: {type: id, caption: name, color: { red: 0, green: 0, blue: 0}},
					volume: volume,
				});
			}
		}
		ws.send(JSON.stringify({
			type: 'remote.StatusEvent',
			payload: {
				status: {
					state: status['state'] == 'playing' ? 4 : status['state'] == 'paused' ? 5 : 3,
					tempo: status['tempo'],
					pitch: status['pitch'],
					tracks: tracks,
				},
			},
		}));
	});
	kfsock.on('queue', function(queue) {
		const items = [];
		for(const s of queue) {
			// TODO: s.status (empty string for MacOS host, "loading", "ready", "playing")
			items.push({
				id: s['queueId'],
				singer: s['singer'] ?? '',
				song: {
					id: {
						type: 1,
						id: s['song_id'] ?? 0, // Not exposed in the MacOS host. // TODO: songId ?
					},
					"title": s['title'],
					"artist": s['artist'],
					"songTracks":[]
				}
			});
		}
		ws.send(JSON.stringify({
			type: 'remote.QueueEvent',
			payload: {
				queue: {
					items: items,
				},
			},
		}));
	});

	kfsock.on('serverUnreacheable', function() {
		console.log('received serverUnreacheable from kf');
		ws.send(JSON.stringify({
			type: 'ServerUnreachable',
			payload: {},
		}));
		kfsock.close();
	});

	kfsock.on('logout', function() {
		console.log('received logout from kf');
		ws.send(JSON.stringify({
			type: 'Logout',
			payload: {},
		}));
		kfsock.close();
	});

	ws.on('message', function(raw) {
		console.log('received: %s', raw);
		const data = JSON.parse(raw);
		switch(data.type) {
			case 'remote.AddToQueueRequest':
				kfsock.emit("queueAdd", {"songId": +data.payload.identifier.id, "pos": +data.payload.position, "singer": data.payload.singer ?? ''});
				break;
			case 'remote.MoveInQueueRequest':
				kfsock.emit("queueMove", {"queueId": data.payload.queueItemId, "from": data.payload.from ? +data.payload.from : +data.payload.queueItemId, "to": +data.payload.to});
				break;
			case 'remote.RemoveFromQueueRequest':
				kfsock.emit("queueRemove", data.payload.queueItemId);
				break;
			case 'remote.PlayRequest':
				kfsock.emit("play", null);
				break;
			case 'remote.PauseRequest':
				kfsock.emit("pause", null);
				break;
			case 'remote.NextRequest':
				kfsock.emit("next", null);
				break;
			case 'remote.TrackVolumeRequest':
				if(data.payload.type == 4) {
					kfsock.emit("volumeBv", data.payload.volume);
				} else {
					for(const [name, id] of Object.entries(audioTracks)) {
						if(id == data.payload.type) {
							kfsock.emit("volumeLd", {filename: name, volume: data.payload.volume});
							return;
						}
					}
					console.log("Failed to change volume for unknown track ", data.payload.type);
				}
				break;
			case 'remote.PitchRequest':
				kfsock.emit("pitch", data.payload.pitch);
				break;
			case 'remote.TempoRequest':
				kfsock.emit("tempo", data.payload.tempo);
				break;
			default:
				console.log("Ignoring unknown command ", data.type)
		}
	});

	ws.on('close', function() {
		console.log('Client disconnected, closing connection to karafun');
		kfsock.disconnect()
	});

	kfsock.on('loginAlreadyTaken', function() {
		kfsock.emit("authenticate", {"login":"proxy " + Math.random(),"channel":channel,"role":"participant","app":"karafun","socket_id":null});
	});

	kfsock.emit("authenticate", {"login":"proxy","channel":channel,"role":"participant","app":"karafun","socket_id":null});
});
